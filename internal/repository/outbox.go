package repository

import (
	"LoanService/internal/model"
	applog "LoanService/pkg/log"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationOutboxRepository interface {
	Create(context.Context, []model.NotificationOutbox) error
	Claim(context.Context, string) (*model.NotificationOutbox, error)
	Retry(context.Context, int64, string) error
	Complete(context.Context, int64, string) error
}

type notificationOutboxRepository struct{ *Repository }

func NewNotificationOutboxRepository(r *Repository) NotificationOutboxRepository {
	return &notificationOutboxRepository{r}
}

// Create inserts notification tasks and ignores duplicate records.
func (r *notificationOutboxRepository) Create(ctx context.Context, values []model.NotificationOutbox) error {
	if len(values) == 0 {
		return nil
	}

	return r.DB(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&values).Error
}

// Claim atomically claims one available task with a 60-second lease.
// SKIP LOCKED prevents multiple workers from claiming the same task concurrently.
func (r *notificationOutboxRepository) Claim(
	ctx context.Context,
	lease string,
) (*model.NotificationOutbox, error) {
	var task model.NotificationOutbox

	result := r.DB(ctx).Raw(`
        WITH next_task AS (
            SELECT id
            FROM notification_outbox
            WHERE delivered_at IS NULL
              AND available_at <= (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
            ORDER BY id
            FOR UPDATE SKIP LOCKED
            LIMIT 1
        )
        UPDATE notification_outbox AS o
        SET available_at = (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint + 60000,
            attempts = o.attempts + 1,
            lease_token = ?
        FROM next_task
        WHERE o.id = next_task.id
        RETURNING o.id, o.email, o.agreement_url
    `, lease).Scan(&task)

	if result.Error != nil {
		applog.Error(ctx, "outbox.Claim failed", result.Error)
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &task, nil
}

// Retry releases the lease and schedules the next attempt using linear backoff.
// The lease token ensures only the current worker can update the task.
func (r *notificationOutboxRepository) Retry(
	ctx context.Context,
	id int64,
	lease string,
) error {
	result := r.DB(ctx).Exec(`
        UPDATE notification_outbox
        SET available_at = (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
                           + LEAST(attempts, 60) * 10000,
            lease_token = NULL
        WHERE id = ?
          AND lease_token = ?
          AND delivered_at IS NULL
    `, id, lease)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// Complete marks a task as delivered and releases its lease.
// The lease token prevents an expired worker from completing the task.
func (r *notificationOutboxRepository) Complete(
	ctx context.Context,
	id int64,
	lease string,
) error {
	result := r.DB(ctx).Exec(`
        UPDATE notification_outbox
        SET delivered_at = (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint,
            lease_token = NULL
        WHERE id = ?
          AND lease_token = ?
          AND delivered_at IS NULL
    `, id, lease)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}
