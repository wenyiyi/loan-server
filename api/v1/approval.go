package v1

type ApprovalRequest struct {
	EmployeeId    string `json:"employee_id"`
	VisitProofURL string `json:"visit_proof_url" binding:"required,url"`
	ApprovedAt    int64  `json:"approved_at" binding:"required"`
}
