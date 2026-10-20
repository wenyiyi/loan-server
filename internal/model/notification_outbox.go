package model

type NotificationOutbox struct {
	ID               int64  `gorm:"primaryKey;autoIncrement"`
	LoanNo           string `gorm:"column:loan_no;size:22;not null;uniqueIndex:uq_outbox_notification"`
	InvestorID       string `gorm:"not null;uniqueIndex:uq_outbox_notification"`
	Email            string `gorm:"not null"`
	AgreementURL     string `gorm:"not null"`
	NotificationType string `gorm:"not null;default:LOAN_FUNDED;uniqueIndex:uq_outbox_notification"`

	LeaseToken  *string `gorm:"column:lease_token"`
	Attempts    int     `gorm:"not null;default:0"`
	AvailableAt int64   `gorm:"not null"`
	DeliveredAt *int64  `gorm:"column:delivered_at"`
	CreatedAt   int64   `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt   int64   `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

func (NotificationOutbox) TableName() string {
	return "notification_outbox"
}
