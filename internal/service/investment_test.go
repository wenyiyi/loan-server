package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"LoanService/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCalculateInvestment(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    model.Status
		amount    int64
		err       error
		remaining int64
	}{
		{"partial", model.Approved, 40, nil, 60}, {"fully funded", model.Approved, 100, nil, 0},
		{"wrong state", model.Proposed, 10, v1.ErrState, 0}, {"zero", model.Approved, 0, v1.ErrRequestParams, 0},
		{"negative", model.Approved, -1, v1.ErrRequestParams, 0}, {"over principal", model.Approved, 101, v1.ErrAmount, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := calculateInvestment(model.Loan{PrincipalAmount: 100, Status: test.status}, "loan", test.amount)
			require.ErrorIs(t, err, test.err)
			if test.err == nil {
				require.Equal(t, test.remaining, result.RemainingAmount)
			}
		})
	}
	result, err := calculateInvestment(model.Loan{PrincipalAmount: math.MaxInt64, InvestedAmount: math.MaxInt64 - 1, Status: model.Approved}, "loan", 1)
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), result.InvestedAmount)
}

type investmentStub struct {
	repository.InvestmentRepository
	record *model.Investment
	err    error
	calls  int
}

func (s *investmentStub) GetByKey(context.Context, string) (*model.Investment, error) {
	s.calls++
	return s.record, s.err
}
func TestIdempotencyIncludesInvestor(t *testing.T) {
	req := v1.InvestmentRequest{Amount: 10, Email: "i@example.com", Key: "key"}
	response, _ := json.Marshal(v1.InvestmentResult{ID: 1, LoanNo: "loan", InvestedAmount: 10, RemainingAmount: 90, Status: model.Approved})
	base := model.Investment{LoanNo: "loan", InvestorID: "investor", Email: req.Email, Amount: 10, Response: response}
	for _, change := range []func(*model.Investment){func(v *model.Investment) { v.LoanNo = "other" }, func(v *model.Investment) { v.InvestorID = "other" }, func(v *model.Investment) { v.Email = "other@example.com" }, func(v *model.Investment) { v.Amount = 11 }} {
		value := base
		change(&value)
		svc := Service{investments: &investmentStub{record: &value}}
		_, err := svc.checkIdempotency(context.Background(), "loan", "investor", req)
		require.ErrorIs(t, err, v1.ErrIdempotency)
	}
	svc := Service{investments: &investmentStub{record: &base}}
	result, err := svc.checkIdempotency(context.Background(), "loan", "investor", req)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.ID)
}
func TestUniqueRecoveryIsConstraintSpecific(t *testing.T) {
	for _, constraint := range []string{"unrelated_unique", "uq_investments_idempotency_key"} {
		stub := &investmentStub{}
		svc := Service{investments: stub, logger: zap.NewNop()}
		databaseErr := &pgconn.PgError{Code: "23505", ConstraintName: constraint}
		_, err := svc.resolveInvestmentError(context.Background(), "loan", "investor", v1.InvestmentRequest{}, databaseErr)
		require.True(t, errors.Is(err, databaseErr))
		if constraint == "unrelated_unique" {
			require.Zero(t, stub.calls)
		} else {
			require.Equal(t, 1, stub.calls)
		}
	}
}
