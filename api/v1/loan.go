package v1

import (
	"github.com/shopspring/decimal"
)

type LoanCreateRequest struct {
	EmployeeId      string           `json:"employee_id"`
	BorrowerID      string           `json:"borrower_id" binding:"required,max=128"`
	PrincipalAmount int64            `json:"principal_amount" binding:"required,gt=0"`
	Rate            *decimal.Decimal `json:"rate" binding:"required"`
	ROI             *decimal.Decimal `json:"roi" binding:"required"`
	AgreementURL    string           `json:"agreement_url" binding:"required"`
}

type LoanCreateResponse struct {
	LoanNo string `json:"loan_no"`
}
