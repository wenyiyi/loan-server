package model

type Disbursement struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	LoanNo             string `gorm:"column:loan_no;size:22;not null;uniqueIndex" json:"loan_no"`
	EmployeeID         string `gorm:"not null" json:"employee_id"`
	SignedAgreementURL string `gorm:"not null" json:"signed_agreement_url"`
	DisbursedAt        int64  `gorm:"not null" json:"disbursed_at"`

	CreatedAt int64 `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

func (Disbursement) TableName() string {
	return "disbursements"
}
