package service

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"context"
	"encoding/json"
)

func (s Service) Invest(
	ctx context.Context,
	loanNo string,
	InvestorID string,
	req v1.InvestmentRequest,
) (*v1.InvestmentResult, error) {
	var result *v1.InvestmentResult

	err := s.transaction(ctx, func(tx Service) error {
		// Lock the loan to prevent concurrent over-investment
		loan, err := tx.loans.GetForUpdate(ctx, loanNo)
		if err != nil {
			return err
		}

		// Return the original result for duplicate requests
		investment, err := tx.checkIdempotency(ctx, loanNo, InvestorID, req)
		if err != nil {
			return err
		}

		if investment != nil {
			result = investment
			return nil
		}
		// Validate the investment and calculate the new balance
		result, err = calculateInvestment(loan, loanNo, req.Amount)
		if err != nil {
			return err
		}

		// Save the investment and update the loan atomically.
		err = tx.saveInvestment(ctx, loanNo, InvestorID, req, result)
		if err != nil {
			return err
		}

		// Record the state change and queue agreement notifications.
		if result.Status == model.Invested {
			return tx.handleFullyFunded(ctx, loan)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s Service) checkIdempotency(
	ctx context.Context,
	loanNo string,
	investorID string,
	req v1.InvestmentRequest,
) (*v1.InvestmentResult, error) {
	investment, err := s.investments.GetByKey(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	if investment == nil {
		return nil, nil
	}

	if investment.LoanNo != loanNo ||
		investment.InvestorID != investorID ||
		investment.Email != req.Email ||
		investment.Amount != req.Amount {
		return nil, v1.ErrIdempotency
	}

	var result v1.InvestmentResult
	if err := json.Unmarshal(investment.Response, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func calculateInvestment(loan model.Loan, loanNo string, amount int64) (*v1.InvestmentResult, error) {
	if loan.Status != model.Approved {
		return nil, v1.ErrState
	}
	if amount <= 0 {
		return nil, v1.ErrRequestParams
	}
	remaining := loan.PrincipalAmount - loan.InvestedAmount
	if amount > remaining {
		return nil, v1.ErrAmount
	}
	result := v1.InvestmentResult{LoanNo: loanNo, InvestedAmount: loan.InvestedAmount + amount, RemainingAmount: remaining - amount, Status: model.Approved}
	if result.RemainingAmount == 0 {
		result.Status = model.Invested
	}
	return &result, nil
}

func (s Service) saveInvestment(ctx context.Context, loanNo string, InvestorID string, req v1.InvestmentRequest, result *v1.InvestmentResult) error {
	investment, err := s.investments.Create(ctx, model.Investment{LoanNo: loanNo, InvestorID: InvestorID, Email: req.Email, Amount: req.Amount, IdempotencyKey: req.Key, Response: json.RawMessage(`{}`)})
	if err != nil {
		return err
	}
	result.ID = investment.ID
	err = s.loans.UpdateInvestedAmount(ctx, loanNo, result.InvestedAmount, result.Status)
	if err != nil {
		return err
	}
	response, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.investments.SaveResponse(ctx, investment.ID, response)
}

func (s Service) handleFullyFunded(ctx context.Context, loan model.Loan) error {
	if err := s.history.Create(ctx, model.LoanStatusHistory{LoanNo: loan.LoanNo, PreviousState: model.Approved, NewState: model.Invested, Action: model.ActionFullyFunded}); err != nil {
		return err
	}
	investments, err := s.investments.ListByLoanNo(ctx, loan.LoanNo)
	if err != nil {
		return err
	}
	recipients := make(map[string]model.Investment)
	for _, investment := range investments {
		recipients[investment.InvestorID] = investment
	}
	tasks := make([]model.NotificationOutbox, 0, len(recipients))
	for _, investment := range recipients {
		tasks = append(tasks, model.NotificationOutbox{LoanNo: loan.LoanNo, InvestorID: investment.InvestorID, Email: investment.Email, AgreementURL: loan.AgreementURL, NotificationType: "LOAN_FUNDED"})
	}
	if len(tasks) == 0 {
		return nil
	}
	return s.outbox.Create(ctx, tasks)
}
