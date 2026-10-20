package notification

import (
	"LoanService/internal/repository"
	applog "LoanService/pkg/log"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Worker struct {
	outbox repository.NotificationOutboxRepository
	Sender Sender
}

func NewWorker(db *gorm.DB, sender Sender) *Worker {
	return &Worker{outbox: repository.NewNotificationOutboxRepository(repository.NewRepository(db)), Sender: sender}
}

// Run polls the outbox every second and processes one task at a time.
// No database transaction is held open while sending emails.
func (w Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Once(ctx); err != nil && ctx.Err() == nil {
				applog.Error(ctx, "notification attempt failed", err)
			}
		}
	}
}

// Once claims and processes a single notification task.
func (w Worker) Once(ctx context.Context) error {
	// Generate a unique lease token for this attempt.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	lease := hex.EncodeToString(b)

	// Atomically claim an available task.
	task, err := w.outbox.Claim(ctx, lease)
	if err != nil {
		return err
	}

	// No pending tasks.
	if task == nil {
		return nil
	}

	// Limit email sending to 10 seconds, shorter than the 60-second lease.
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = w.Sender.Send(
		sendCtx,
		task.Email,
		task.AgreementURL,
		fmt.Sprintf("notification-%d", task.ID),
	)

	// Use a separate timeout to update the task status.
	ackCtx, ackCancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		5*time.Second,
	)
	defer ackCancel()

	if err != nil {
		// Release the lease and schedule a retry.
		if retryErr := w.outbox.Retry(ackCtx, task.ID, lease); retryErr != nil {
			return fmt.Errorf(
				"send failed: %v; retry update failed: %w",
				err, retryErr,
			)
		}
		return err
	}

	// Mark the task as delivered.
	return w.outbox.Complete(ackCtx, task.ID, lease)
}
