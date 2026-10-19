package v1

import (
	"github.com/shopspring/decimal"
)

type LoanCreateRequest struct {
	BorrowerID      string           `json:"borrower_id" binding:"required,max=128"`
	PrincipalAmount int64            `json:"principal_amount" binding:"required,gt=0"`
	Rate            *decimal.Decimal `json:"rate" binding:"required"`
	ROI             *decimal.Decimal `json:"roi" binding:"required"`
	AgreementURL    string           `json:"agreement_url" binding:"required,url"`
}

func (r LoanCreateRequest) Validate() error {
	if r.Rate == nil || r.ROI == nil {
		return ErrRequestParams
	}

	max := decimal.NewFromInt(100)

	// ROI (investor return) should not exceed Rate (borrower interest rate)
	// The difference represents the platform's interest spread
	if r.Rate.IsNegative() ||
		r.Rate.GreaterThan(max) ||
		r.ROI.IsNegative() ||
		r.ROI.GreaterThan(max) ||
		r.ROI.GreaterThan(*r.Rate) {
		return ErrRequestParams
	}

	return nil
}

type LoanCreateResponse struct {
	LoanNo string `json:"loan_no"`
}
