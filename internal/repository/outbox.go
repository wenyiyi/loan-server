package repository

import (
	"LoanService/internal/model"
	applog "LoanService/pkg/log"
	"context"

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

func (r *notificationOutboxRepository) Create(ctx context.Context, values []model.NotificationOutbox) error {
	if len(values) == 0 {
		return nil
	}
	return r.DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&values).Error
}

func (r *notificationOutboxRepository) Claim(ctx context.Context, lease string) (*model.NotificationOutbox, error) {
	var task model.NotificationOutbox
	result := r.DB(ctx).Raw(`UPDATE notification_outbox
 SET available_at=(extract(epoch from now())*1000)::bigint+60000, attempts=attempts+1, lease_token=?
 WHERE id=(SELECT id FROM notification_outbox WHERE delivered_at IS NULL AND available_at<=(extract(epoch from now())*1000)::bigint
 ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,email,agreement_url`, lease).Scan(&task)
	if result.Error != nil {
		applog.Error(ctx, "outbox.Claim failed", result.Error)
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &task, nil
}

func (r *notificationOutboxRepository) Retry(ctx context.Context, id int64, lease string) error {
	return r.DB(ctx).Exec(`UPDATE notification_outbox SET available_at=(extract(epoch from now())*1000)::bigint+LEAST(attempts,60)*10000,lease_token=NULL WHERE id=? AND lease_token=?`, id, lease).Error
}

func (r *notificationOutboxRepository) Complete(ctx context.Context, id int64, lease string) error {
	return r.DB(ctx).Exec("UPDATE notification_outbox SET delivered_at=(extract(epoch from now())*1000)::bigint,lease_token=NULL WHERE id=? AND lease_token=?", id, lease).Error
}
