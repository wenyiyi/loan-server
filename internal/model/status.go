package model

type Status string
type Action string

const (
	Proposed  Status = "PROPOSED"
	Approved  Status = "APPROVED"
	Invested  Status = "INVESTED"
	Disbursed Status = "DISBURSED"
)

const (
	ActionCreate      Action = "CREATE"
	ActionApprove     Action = "APPROVE"
	ActionFullyFunded Action = "FULLY_FUNDED"
	ActionDisburse    Action = "DISBURSE"
)
