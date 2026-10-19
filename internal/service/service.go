package service

import (
	"LoanService/internal/repository"
	"context"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	logger        *zap.Logger
	db            *repository.Repository
	loans         repository.LoanRepository
	approvals     repository.ApprovalRepository
	disbursements repository.DisbursementRepository
	investments   repository.InvestmentRepository
	history       repository.LoanStatusHistoryRepository
	outbox        repository.NotificationOutboxRepository
}

func NewService(db *repository.Repository, loggers ...*zap.Logger) Service {
	logger := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return Service{logger: logger, db: db, loans: repository.NewLoanRepository(db), approvals: repository.NewApprovalRepository(db), disbursements: repository.NewDisbursementRepository(db), investments: repository.NewInvestmentRepository(db), history: repository.NewLoanStatusHistoryRepository(db), outbox: repository.NewNotificationOutboxRepository(db)}
}

// Every table repository in the callback shares this transaction.
func (s Service) transaction(ctx context.Context, fn func(Service) error) error {
	return s.db.DB(ctx).Transaction(func(tx *gorm.DB) error { return fn(NewService(repository.NewRepository(tx), s.logger)) })
}
