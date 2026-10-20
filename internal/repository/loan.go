package repository

import (
	"context"
	"errors"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LoanRepository interface {
	Create(ctx context.Context, loan model.Loan) (model.Loan, error)
	GetByLoanNo(ctx context.Context, loanNo string) (model.Loan, error)
	GetForUpdate(ctx context.Context, loanNo string) (model.Loan, error)
	UpdateInvestedAmount(ctx context.Context, loanNo string, total int64, newStatus model.Status) error
	UpdateStatusWithConditions(ctx context.Context, loanNo string, from, to model.Status) error
}

type loanRepository struct {
	*Repository
}

func NewLoanRepository(r *Repository) LoanRepository {
	return &loanRepository{Repository: r}
}

// Create creates a new loan.
func (r *loanRepository) Create(
	ctx context.Context,
	loan model.Loan,
) (model.Loan, error) {
	err := r.DB(ctx).Create(&loan).Error
	return loan, err
}

// GetByLoanNo retrieves a loan by its business identifier.
func (r *loanRepository) GetByLoanNo(
	ctx context.Context,
	loanNo string,
) (model.Loan, error) {
	return findLoan(r.DB(ctx), loanNo)
}

// GetForUpdate locks the loan row within a transaction.
func (r *loanRepository) GetForUpdate(
	ctx context.Context,
	loanNo string,
) (model.Loan, error) {
	db := r.DB(ctx).Clauses(clause.Locking{
		Strength: "UPDATE",
	})

	return findLoan(db, loanNo)
}

// findLoan centralizes loan lookup and error handling.
func findLoan(db *gorm.DB, loanNo string) (model.Loan, error) {
	var loan model.Loan

	err := db.Where("loan_no = ?", loanNo).
		Take(&loan).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Loan{}, v1.ErrNotFound
	}

	return loan, err
}

// UpdateInvestedAmount updates the invested amount and status
// only when the loan is in the APPROVED state.
func (r *loanRepository) UpdateInvestedAmount(
	ctx context.Context,
	loanNo string,
	totalInvestedAmount int64,
	newStatus model.Status,
) error {
	result := r.DB(ctx).
		Model(&model.Loan{}).
		Where(
			`loan_no = ?
			 AND status = ?
			 AND ? BETWEEN 0 AND principal_amount
			 AND (
				 (? = principal_amount AND ? = ?)
				 OR
				 (? < principal_amount AND ? = ?)
			 )`,
			loanNo,
			model.Approved,
			totalInvestedAmount,
			totalInvestedAmount, newStatus, model.Invested,
			totalInvestedAmount, newStatus, model.Approved,
		).
		Updates(map[string]any{
			"invested_amount": totalInvestedAmount,
			"status":          newStatus,
		})

	return checkUpdateResult(result)
}

// UpdateStatusWithConditions performs a conditional status transition.
func (r *loanRepository) UpdateStatusWithConditions(ctx context.Context, loanNo string, from, to model.Status) error {
	result := r.DB(ctx).
		Model(&model.Loan{}).
		Where("loan_no = ? AND status = ?", loanNo, from).
		Update("status", to)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return v1.ErrState
	}

	return nil
}
