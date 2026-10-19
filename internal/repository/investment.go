package repository

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	applog "LoanService/pkg/log"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
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
	// Different loans can race on the global key despite holding their own row locks.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_investments_idempotency_key" {
		return value, v1.ErrIdempotency
	}
	return value, err
}

func (r *investmentRepository) GetByKey(ctx context.Context, key string) (*model.Investment, error) {
	var value model.Investment
	err := r.DB(ctx).Where("idempotency_key = ?", key).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		applog.Error(ctx, "get investment by idempotency key failed", err)
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
