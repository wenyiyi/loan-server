package v1

type DisbursementRequest struct {
	AgreementURL string `json:"signed_agreement_url" binding:"required,url"`
}
