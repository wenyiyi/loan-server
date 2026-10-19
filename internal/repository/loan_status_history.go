package repository

import (
	"LoanService/internal/model"
	"context"
)

type LoanStatusHistoryRepository interface {
	Create(context.Context, model.LoanStatusHistory) error
}

type loanStatusHistoryRepository struct{ *Repository }

func NewLoanStatusHistoryRepository(r *Repository) LoanStatusHistoryRepository {
	return &loanStatusHistoryRepository{r}
}

func (r *loanStatusHistoryRepository) Create(ctx context.Context, value model.LoanStatusHistory) error {
	return r.DB(ctx).Create(&value).Error
}
