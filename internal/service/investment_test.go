package service

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"github.com/stretchr/testify/require"
	"testing"
)

// Pure calculation tests require no database; transaction tests live in tests/integration.
func TestCalculateInvestment(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status           model.Status
		invested, amount int64
		err              error
		resultStatus     model.Status
		remaining        int64
	}{
		{"partial", model.Approved, 20, 30, nil, model.Approved, 50},
		{"full", model.Approved, 20, 80, nil, model.Invested, 0},
		{"zero", model.Approved, 0, 0, v1.ErrRequestParams, "", 0},
		{"negative", model.Approved, 0, -1, v1.ErrRequestParams, "", 0},
		{"excess", model.Approved, 20, 81, v1.ErrAmount, "", 0},
		{"proposed", model.Proposed, 0, 1, v1.ErrState, "", 0},
		{"invested", model.Invested, 100, 1, v1.ErrState, "", 0},
		{"disbursed", model.Disbursed, 100, 1, v1.ErrState, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("[Input] status=%s, principal=100, invested=%d, amount=%d; want error=%v, status=%s, remaining=%d", tc.status, tc.invested, tc.amount, tc.err, tc.resultStatus, tc.remaining)
			got, err := calculateInvestment(model.Loan{Status: tc.status, PrincipalAmount: 100, InvestedAmount: tc.invested}, "loan", tc.amount)
			t.Logf("[Actual] result=%+v, error=%v", got, err)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				require.Nil(t, got)
				t.Log("[Pass] Expected error, no result")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.resultStatus, got.Status)
			require.Equal(t, tc.remaining, got.RemainingAmount)
			require.Equal(t, tc.invested+tc.amount, got.InvestedAmount)
			require.Equal(t, "loan", got.LoanNo)
			t.Log("[Pass] Status, total, remaining amount, and loan number match")
		})
	}
}
