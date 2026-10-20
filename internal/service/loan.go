package service

import (
	"context"
	"time"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"LoanService/pkg/log"
	"LoanService/pkg/sid"

	"go.uber.org/zap"
)

// Create creates a loan and records its initial status.
func (s Service) Create(ctx context.Context, req v1.LoanCreateRequest) (*v1.LoanCreateResponse, error) {
	loanNo := sid.GenUUID()
	loan := model.Loan{
		LoanNo:          loanNo,
		BorrowerID:      req.BorrowerID,
		PrincipalAmount: req.PrincipalAmount,
		Rate:            *req.Rate,
		ROI:             *req.ROI,
		AgreementURL:    req.AgreementURL,
		Status:          model.Proposed,
	}

	err := s.transaction(ctx, func(tx Service) error {
		_, err := tx.loans.Create(ctx, loan)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("insert loan failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		return tx.recordHistory(ctx, loanNo, "", model.Proposed, model.ActionCreate)
	})
	if err != nil {
		log.WithContext(ctx, s.logger).Error("create loan failed", zap.String("loan_no", loanNo), zap.Error(err))
		return nil, err
	}

	return &v1.LoanCreateResponse{
		LoanNo: loanNo,
	}, nil
}

// Approve approves a proposed loan.
func (s Service) Approve(ctx context.Context, loanNo string, EmployeeId string, req v1.ApprovalRequest) error {
	return s.transaction(ctx, func(tx Service) error {
		// add lock
		err := tx.loans.UpdateStatusWithConditions(ctx, loanNo, model.Proposed, model.Approved)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("update loan approval status failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}
		// create approval
		_, err = tx.approvals.Create(ctx, model.Approval{
			LoanNo:        loanNo,
			EmployeeID:    EmployeeId,
			VisitProofURL: req.VisitProofURL,
			ApprovedAt:    time.Now().UnixMilli(),
		})
		if err != nil {
			log.WithContext(ctx, s.logger).Error("create loan approval failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}
		// submit tx, release lock
		return tx.recordHistory(ctx, loanNo, model.Proposed, model.Approved, model.ActionApprove)
	})
}

// recordHistory records a loan status transition.
func (s Service) recordHistory(ctx context.Context, loanNo string, from, to model.Status, action model.Action) error {
	err := s.history.Create(ctx, model.LoanStatusHistory{
		LoanNo:        loanNo,
		PreviousState: from,
		NewState:      to,
		Action:        action,
	})

	if err != nil {
		log.WithContext(ctx, s.logger).Error("create loan status history failed", zap.String("loan_no", loanNo), zap.Error(err))
	}

	return err
}

// Disburse disburses a fully funded loan.
func (s Service) Disburse(ctx context.Context, loanNo string, employeeID string, req v1.DisbursementRequest) error {
	return s.transaction(ctx, func(tx Service) error {
		// add lock
		loan, err := tx.loans.GetForUpdate(ctx, loanNo)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("lock loan for disbursement failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		if loan.Status != model.Invested {
			return v1.ErrState
		}

		_, err = tx.disbursements.Create(ctx, model.Disbursement{
			LoanNo:             loanNo,
			EmployeeID:         employeeID,
			SignedAgreementURL: req.AgreementURL,
			DisbursedAt:        time.Now().UnixMilli(),
		})
		if err != nil {
			log.WithContext(ctx, s.logger).Error("create loan disbursement failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		err = tx.loans.UpdateStatusWithConditions(
			ctx, loanNo, model.Invested, model.Disbursed,
		)
		if err != nil {
			log.WithContext(ctx, s.logger).Error("update loan disbursement status failed", zap.String("loan_no", loanNo), zap.Error(err))
			return err
		}

		// submit tx, release lock
		return tx.recordHistory(ctx, loanNo, model.Invested, model.Disbursed, model.ActionDisburse)
	})
}

// Get returns a loan with its current remaining amount.
func (s Service) Get(ctx context.Context, loanNo string) (model.Loan, error) {
	loan, err := s.loans.GetByLoanNo(ctx, loanNo)
	if err != nil {
		return model.Loan{}, err
	}
	loan.Remaining = loan.PrincipalAmount - loan.InvestedAmount
	return loan, nil
}
