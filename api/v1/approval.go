package v1

type ApprovalRequest struct {
	VisitProofURL string `json:"visit_proof_url" binding:"required,url"`
}
