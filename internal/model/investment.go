package model

import (
	"encoding/json"
)

type Investment struct {
	ID             int64           `gorm:"primaryKey;autoIncrement" json:"investment_id"`
	LoanNo         string          `gorm:"column:loan_no;size:22;not null;index" json:"loan_no"`
	InvestorID     string          `gorm:"not null" json:"investor_id"`
	Email          string          `gorm:"not null" json:"-"`
	Amount         int64           `gorm:"not null" json:"amount"`
	IdempotencyKey string          `gorm:"not null;uniqueIndex" json:"-"`
	Response       json.RawMessage `gorm:"type:jsonb;not null" json:"-"`
	CreatedAt      int64           `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt      int64           `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

func (Investment) TableName() string {
	return "investments"
}
