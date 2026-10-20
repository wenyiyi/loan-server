package service

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"LoanService/internal/repository"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type loanReadStub struct {
	repository.LoanRepository
	loan   model.Loan
	err    error
	ctx    context.Context
	number string
}

func (r *loanReadStub) GetByLoanNo(ctx context.Context, number string) (model.Loan, error) {
	r.ctx = ctx
	r.number = number
	return r.loan, r.err
}

func TestGetLoan(t *testing.T) {
	for _, tc := range []struct {
		name     string
		invested int64
		err      error
	}{{"new", 0, nil}, {"partial", 40, nil}, {"full", 100, nil}, {"not found", 0, v1.ErrNotFound}, {"database error", 0, errors.New("database unavailable")}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &loanReadStub{loan: model.Loan{LoanNo: "loan", PrincipalAmount: 100, InvestedAmount: tc.invested}, err: tc.err}
			svc := Service{loans: repo}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := svc.Get(ctx, "loan")
			t.Logf("[Get] invested=%d, remaining=%d, error=%v; want error=%v", tc.invested, got.Remaining, err, tc.err)
			require.Equal(t, ctx, repo.ctx)
			require.Equal(t, "loan", repo.number)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(100)-tc.invested, got.Remaining)
			require.Equal(t, "loan", got.LoanNo)
		})
	}
}
