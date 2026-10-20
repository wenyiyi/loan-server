package repository

import (
	"LoanService/internal/model"
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

type InvestmentRepository interface {
	Create(context.Context, model.Investment) (model.Investment, error)
	GetByKey(context.Context, string) (*model.Investment, error)
	SaveResponse(context.Context, int64, json.RawMessage) error
	ListByLoanNo(context.Context, string) ([]model.Investment, error)
}
type investmentRepository struct{ *Repository }

func NewInvestmentRepository(r *Repository) InvestmentRepository { return &investmentRepository{r} }

func (r *investmentRepository) Create(ctx context.Context, value model.Investment) (model.Investment, error) {
	err := r.DB(ctx).Create(&value).Error
	return value, err
}

func (r *investmentRepository) GetByKey(ctx context.Context, key string) (*model.Investment, error) {
	var value model.Investment
	err := r.DB(ctx).Where("idempotency_key = ?", key).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *investmentRepository) SaveResponse(ctx context.Context, id int64, response json.RawMessage) error {
	return r.DB(ctx).Model(&model.Investment{}).Where("id = ?", id).Update("response", response).Error
}

func (r *investmentRepository) ListByLoanNo(ctx context.Context, loanNo string) ([]model.Investment, error) {
	values := make([]model.Investment, 0)
	err := r.DB(ctx).Where("loan_no = ?", loanNo).Order("id").Find(&values).Error
	return values, err
}
