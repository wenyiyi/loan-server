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
	DB     *gorm.DB
	Sender Sender
}

func NewWorker(db *gorm.DB, sender Sender) *Worker {
	return &Worker{DB: db, Sender: sender}
}

// Run leases one task at a time. No database transaction stays open during SMTP.
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

func (w Worker) Once(ctx context.Context) error {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	lease := hex.EncodeToString(b)
	outbox := repository.NewNotificationOutboxRepository(repository.NewRepository(w.DB))
	task, err := outbox.Claim(ctx, lease)
	if err != nil {
		applog.Error(ctx, "worker.Once failed", err)
		return err
	}
	if task == nil {
		return nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = w.Sender.Send(sendCtx, task.Email, task.AgreementURL, fmt.Sprintf("notification-%d", task.ID))
	if err != nil {
		applog.Error(ctx, "worker.Once failed", err)
		ack := outbox.Retry(ctx, task.ID, lease)
		if ack != nil {
			applog.Error(ctx, "worker.Once failed", ack)
			return ack
		}
		return err
	}
	return outbox.Complete(ctx, task.ID, lease)
}
