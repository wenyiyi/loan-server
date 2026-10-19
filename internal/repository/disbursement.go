package repository

import (
	"LoanService/internal/model"
	"context"
)

type DisbursementRepository interface {
	Create(context.Context, model.Disbursement) (model.Disbursement, error)
}

type disbursementRepository struct{ *Repository }

func NewDisbursementRepository(r *Repository) DisbursementRepository {
	return &disbursementRepository{r}
}

func (r *disbursementRepository) Create(ctx context.Context, value model.Disbursement) (model.Disbursement, error) {
	err := r.DB(ctx).Create(&value).Error
	return value, err
}
