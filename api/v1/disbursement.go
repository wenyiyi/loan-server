package v1

type DisbursementRequest struct {
	EmployeeId   string `json:"employee_id"`
	AgreementURL string `json:"signed_agreement_url" binding:"required"`
	At           int64  `json:"disbursed_at" binding:"required"`
}
