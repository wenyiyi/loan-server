package service

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"LoanService/pkg/log"
	"context"
	"encoding/json"

	"go.uber.org/zap"
)

func (s Service) Invest(
	ctx context.Context,
	loanNo string,
	InvestorID string,
	email string,
	key string,
	req v1.InvestmentRequest,
) (*v1.InvestmentResult, error) {
	var result *v1.InvestmentResult

	err := s.transaction(ctx, func(tx Service) error {
		// add lock
		loan, err := tx.loans.GetForUpdate(ctx, loanNo)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("lock loan for investment failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		// Return the original result for duplicate requests
		investment, err := tx.checkIdempotency(ctx, loanNo, InvestorID, key, email, req.Amount)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("check investment idempotency failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		if investment != nil {
			result = investment
			return nil
		}
		// Validate the investment and calculate the new balance
		result, err = calculateInvestment(loan, loanNo, req.Amount)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("calculate investment failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		// Save the investment and update the loan atomically.
		err = tx.saveInvestment(ctx, loanNo, InvestorID, email, key, req.Amount, result)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("save investment failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		// Record the state change and queue agreement notifications.
		if result.Status == model.Invested {
			// submit tx, release lock
			return tx.handleFullyFunded(ctx, loan)
		}

		return nil
	})

	if err != nil {
		log.WithContext(ctx, s.logger).Error("investment transaction failed", zap.String("loan_no", loanNo), zap.Error(err))
		return nil, err
	}

	return result, nil
}

// checkIdempotency checks whether an investment request has already been processed.
// If the key exists, it validates the request parameters and returns the original response.
// If the key does not exist, it returns nil so the investment can proceed.
func (s Service) checkIdempotency(ctx context.Context, loanNo string, investorID string, key string, email string, amount int64) (*v1.InvestmentResult, error) {
	investment, err := s.investments.GetByKey(ctx, key)
	if err != nil {
		log.WithContext(ctx, s.logger).Error("get existing investment failed", zap.String("loan_no", loanNo), zap.Error(err))
		return nil, err
	}

	// No existing request was found. Proceed with the investment.
	if investment == nil {
		return nil, nil
	}

	// Reject the request if the same key is reused with different parameters.
	if investment.LoanNo != loanNo ||
		investment.InvestorID != investorID ||
		investment.Email != email ||
		investment.Amount != amount {
		return nil, v1.ErrIdempotency
	}

	// Return the original response without processing the investment again.
	var result v1.InvestmentResult
	if err := json.Unmarshal(investment.Response, &result); err != nil {
		log.WithContext(ctx, s.logger).Error("decode investment response failed", zap.String("loan_no", loanNo), zap.Error(err))
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
	// Partial funding: APPROVED -> APPROVED
	// Fully funded: APPROVED -> INVESTED
	result := v1.InvestmentResult{LoanNo: loanNo, InvestedAmount: loan.InvestedAmount + amount, RemainingAmount: remaining - amount, Status: model.Approved}
	if result.RemainingAmount == 0 {
		result.Status = model.Invested
	}
	return &result, nil
}

func (s Service) saveInvestment(ctx context.Context, loanNo string, InvestorID string, email string, key string, amount int64, result *v1.InvestmentResult) error {
	investment, err := s.investments.Create(ctx, model.Investment{LoanNo: loanNo, InvestorID: InvestorID, Email: email, Amount: amount, IdempotencyKey: key, Response: json.RawMessage(`{}`)})
	if err != nil {
		log.WithContext(ctx, s.logger).Error("create investment failed", zap.String("loan_no", loanNo), zap.Error(err))
		return err
	}
	result.ID = investment.ID
	err = s.loans.UpdateInvestedAmount(ctx, loanNo, result.InvestedAmount, result.Status)
	if err != nil {
		log.WithContext(ctx, s.logger).Error("update invested amount failed", zap.String("loan_no", loanNo), zap.Error(err))
		return err
	}
	response, err := json.Marshal(result)
	if err != nil {
		log.WithContext(ctx, s.logger).Error("encode investment response failed", zap.String("loan_no", loanNo), zap.Error(err))
		return err
	}
	return s.investments.SaveResponse(ctx, investment.ID, response)
}

func (s Service) handleFullyFunded(ctx context.Context, loan model.Loan) error {
	if err := s.history.Create(ctx, model.LoanStatusHistory{LoanNo: loan.LoanNo, PreviousState: model.Approved, NewState: model.Invested, Action: model.ActionFullyFunded}); err != nil {
		log.WithContext(ctx, s.logger).Error("create fully funded history failed", zap.String("loan_no", loan.LoanNo), zap.Error(err))
		return err
	}
	investments, err := s.investments.ListByLoanNo(ctx, loan.LoanNo)
	if err != nil {
		log.WithContext(ctx, s.logger).Error("list funded loan investments failed", zap.String("loan_no", loan.LoanNo), zap.Error(err))
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
