package v1

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestLoanCreateValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rate, roi *decimal.Decimal
		valid     bool
	}{
		{"valid", dec("10"), dec("8"), true}, {"zero", dec("0"), dec("0"), true}, {"upper boundary", dec("100"), dec("100"), true},
		{"missing rate", nil, dec("8"), false}, {"missing ROI", dec("10"), nil, false},
		{"negative rate", dec("-1"), dec("0"), false}, {"negative ROI", dec("10"), dec("-1"), false},
		{"rate too high", dec("100.000001"), dec("8"), false}, {"ROI too high", dec("100"), dec("100.000001"), false}, {"ROI exceeds rate", dec("10"), dec("11"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("[Input] rate=%v, ROI=%v; want valid=%t", tc.rate, tc.roi, tc.valid)
			err := (LoanCreateRequest{Rate: tc.rate, ROI: tc.roi}).Validate()
			t.Logf("[Actual] error=%v, valid=%t", err, err == nil)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrRequestParams)
			}
			t.Log("[Pass] Rate and ROI check matches")
		})
	}
}
func dec(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
