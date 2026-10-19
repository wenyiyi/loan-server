package model

import (
	"github.com/shopspring/decimal"
)

type Loan struct {
	ID              int64  `gorm:"primaryKey;autoIncrement" json:"-"`
	LoanNo          string `gorm:"column:loan_no;size:22;not null;uniqueIndex" json:"loan_no"`
	BorrowerID      string `gorm:"not null" json:"borrower_id"`
	PrincipalAmount int64  `gorm:"not null" json:"principal_amount"`
	InvestedAmount  int64  `gorm:"not null;default:0" json:"invested_amount"`
	Remaining       int64  `gorm:"-" json:"remaining_amount"`

	Rate decimal.Decimal `gorm:"type:numeric(10,6);not null" json:"rate"`
	ROI  decimal.Decimal `gorm:"type:numeric(10,6);not null" json:"roi"`

	AgreementURL string `gorm:"not null" json:"agreement_url"`
	Status       Status `gorm:"type:varchar(32);not null;default:PROPOSED" json:"status"`

	CreatedAt int64 `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

func (Loan) TableName() string {
	return "loans"
}
