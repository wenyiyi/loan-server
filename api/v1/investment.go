package v1

import "LoanService/internal/model"

type InvestmentRequest struct {
	Amount int64  `json:"amount" binding:"required,gt=0"`
	Email  string `json:"-"`
	Key    string `json:"-"`
}

type InvestmentResult struct {
	ID              int64        `json:"id"`
	LoanNo          string       `json:"loan_no"`
	InvestedAmount  int64        `json:"invested_amount"`
	RemainingAmount int64        `json:"remaining_amount"`
	Status          model.Status `json:"status"`
}
