package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"LoanService/internal/repository"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type investmentLookupStub struct {
	repository.InvestmentRepository
	value *model.Investment
	err   error
}

func (s investmentLookupStub) GetByKey(context.Context, string) (*model.Investment, error) {
	return s.value, s.err
}
func TestCheckIdempotency(t *testing.T) {
	expected := v1.InvestmentResult{ID: 7, LoanNo: "loan", InvestedAmount: 10, RemainingAmount: 90, Status: model.Approved}
	response, err := json.Marshal(expected)
	require.NoError(t, err)
	existing := model.Investment{LoanNo: "loan", InvestorID: "investor", Email: "i@example.com", Amount: 10, Response: response}
	lookupErr := errors.New("lookup failed")
	for _, tc := range []struct {
		name        string
		value       *model.Investment
		lookupError error
		investor    string
		email       string
		req         v1.InvestmentRequest
		wantError   error
		malformed   bool
	}{
		{name: "new", investor: "investor", email: "i@example.com", req: v1.InvestmentRequest{Amount: 10}},
		{name: "replay", value: &existing, investor: "investor", email: "i@example.com", req: v1.InvestmentRequest{Amount: 10}},
		{name: "lookup error", lookupError: lookupErr, wantError: lookupErr},
		{name: "changed amount", value: &existing, investor: "investor", email: "i@example.com", req: v1.InvestmentRequest{Amount: 11}, wantError: v1.ErrIdempotency},
		{name: "changed investor", value: &existing, investor: "other", email: "i@example.com", req: v1.InvestmentRequest{Amount: 10}, wantError: v1.ErrIdempotency},
		{name: "changed email", value: &existing, investor: "investor", email: "other@example.com", req: v1.InvestmentRequest{Amount: 10}, wantError: v1.ErrIdempotency},
		{name: "invalid stored response", value: &model.Investment{LoanNo: "loan", InvestorID: "investor", Email: "i@example.com", Amount: 10, Response: json.RawMessage(`{`)}, investor: "investor", email: "i@example.com", req: v1.InvestmentRequest{Amount: 10}, malformed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("[Input] case=%s, saved=%+v, lookup error=%v, investor=%s, request=%+v; want error=%v, bad response=%t", tc.name, tc.value, tc.lookupError, tc.investor, tc.req, tc.wantError, tc.malformed)
			svc := Service{logger: zap.NewNop(), investments: investmentLookupStub{value: tc.value, err: tc.lookupError}}
			result, err := svc.checkIdempotency(context.Background(), "loan", tc.investor, "test-key", tc.email, tc.req.Amount)
			t.Logf("[Actual] result=%+v, error=%v", result, err)
			if tc.malformed {
				require.Error(t, err)
				require.Nil(t, result)
			} else if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tc.value == nil {
					require.Nil(t, result)
				} else {
					require.Equal(t, &expected, result)
				}
			}
			t.Log("[Pass] Key lookup result and error match")
		})
	}
}
