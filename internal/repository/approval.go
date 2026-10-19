package repository

import (
	"LoanService/internal/model"
	"context"
)

type ApprovalRepository interface {
	Create(context.Context, model.Approval) (model.Approval, error)
}
type approvalRepository struct{ *Repository }

func NewApprovalRepository(r *Repository) ApprovalRepository { return &approvalRepository{r} }

func (r *approvalRepository) Create(ctx context.Context, value model.Approval) (model.Approval, error) {
	err := r.DB(ctx).Create(&value).Error
	return value, err
}
