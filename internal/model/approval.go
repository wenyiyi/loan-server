package model

type Approval struct {
	ID                  int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	LoanNo              string `gorm:"column:loan_no;size:22;not null;index" json:"loan_no"`
	ValidatorEmployeeID string `gorm:"not null" json:"validator_employee_id"`
	VisitProofURL       string `gorm:"not null" json:"visit_proof_url"`
	ApprovedAt          int64  `gorm:"not null" json:"approved_at"`
	CreatedAt           int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt           int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

func (Approval) TableName() string {
	return "approvals"
}
