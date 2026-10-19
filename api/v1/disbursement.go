package v1

type DisbursementRequest struct {
	AgreementURL string `json:"signed_agreement_url" binding:"required"`
	DisbursedAt  int64  `json:"disbursed_at" binding:"required"`
}
