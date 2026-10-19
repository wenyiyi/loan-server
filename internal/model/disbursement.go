package model

type Disbursement struct {
	LoanNo             string `gorm:"column:loan_no;size:22;not null;index" json:"loan_no"`
	OfficerEmployeeID  string `json:"officer_employee_id"`
	SignedAgreementURL string `json:"signed_agreement_url"`
	DisbursedAt        int64  `json:"disbursed_at"`
}

func (Disbursement) TableName() string { return "loan_disbursements" }
