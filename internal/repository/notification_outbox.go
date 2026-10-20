package repository

import (
	"context"

	"LoanService/internal/model"
	applog "LoanService/pkg/log"

	"gorm.io/gorm/clause"
)

type NotificationOutboxRepository interface {
	Create(context.Context, []model.NotificationOutbox) error
	Claim(context.Context, string) (*model.NotificationOutbox, error)
	Retry(context.Context, int64, string) error
	Complete(context.Context, int64, string) error
}

type notificationOutboxRepository struct {
	*Repository
}

func NewNotificationOutboxRepository(
	r *Repository,
) NotificationOutboxRepository {
	return &notificationOutboxRepository{
		Repository: r,
	}
}

// Create inserts notification tasks and ignores duplicate records.
// GORM automatically populates CreatedAt and UpdatedAt.
func (r *notificationOutboxRepository) Create(
	ctx context.Context,
	values []model.NotificationOutbox,
) error {
	if len(values) == 0 {
		return nil
	}
	// batch insert
	return r.DB(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&values).Error
}

// Claim atomically claims one available task with a 60-second lease.
// SKIP LOCKED prevents multiple workers from claiming the same task.
// Tasks are ordered by their next available time.
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
			  AND available_at <=
				  (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
			ORDER BY available_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE notification_outbox AS o
		SET
			available_at =
				(EXTRACT(EPOCH FROM NOW()) * 1000)::bigint + 60000,
			attempts = o.attempts + 1,
			lease_token = ?,
			updated_at =
				(EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
		FROM next_task
		WHERE o.id = next_task.id
		RETURNING
			o.id,
			o.email,
			o.agreement_url
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

// Retry releases the lease and schedules the next attempt.
// Linear backoff: 10 seconds per attempt, capped at 10 minutes.
// The lease token ensures only the current worker can update the task.
func (r *notificationOutboxRepository) Retry(
	ctx context.Context,
	id int64,
	lease string,
) error {
	result := r.DB(ctx).Exec(`
		UPDATE notification_outbox
		SET
			available_at =
				(EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
				+ LEAST(attempts, 60) * 10000,
			lease_token = NULL,
			updated_at =
				(EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
		WHERE id = ?
		  AND lease_token = ?
		  AND delivered_at IS NULL
	`, id, lease)

	return checkUpdateResult(result)
}

// Complete marks a task as delivered and releases its lease.
// The lease token prevents another worker from completing the task.
func (r *notificationOutboxRepository) Complete(
	ctx context.Context,
	id int64,
	lease string,
) error {
	result := r.DB(ctx).Exec(`
		UPDATE notification_outbox
		SET
			delivered_at =
				(EXTRACT(EPOCH FROM NOW()) * 1000)::bigint,
			lease_token = NULL,
			updated_at =
				(EXTRACT(EPOCH FROM NOW()) * 1000)::bigint
		WHERE id = ?
		  AND lease_token = ?
		  AND delivered_at IS NULL
	`, id, lease)

	return checkUpdateResult(result)
}
