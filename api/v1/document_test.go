package v1

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDocumentURLValidation(t *testing.T) {
	for _, tc := range []struct {
		name, url                 string
		create, approve, disburse bool
	}{
		{"pdf", "https://example.com/a.pdf", true, false, true},
		{"jpg", "https://example.com/a.jpg", false, true, true},
		{"jpeg", "https://example.com/a.jpeg", false, true, true},
		{"png", "https://example.com/a.png", false, true, false},
		{"signed uppercase PDF", "https://example.com/A.PDF?signature=test", true, false, true},
		{"plain HTTP", "http://example.com/a.pdf", false, false, false},
		{"credentials", "https://user:pass@example.com/a.pdf", false, false, false},
		{"no suffix", "https://example.com/file", false, false, false},
		{"query suffix only", "https://example.com/file?name=a.pdf", false, false, false},
		{"wrong suffix", "https://example.com/a.exe", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := []struct {
				name  string
				valid bool
				check func() error
			}{
				{"create", tc.create, (LoanCreateRequest{Rate: dec("10"), ROI: dec("8"), AgreementURL: tc.url}).Validate},
				{"approve", tc.approve, (ApprovalRequest{VisitProofURL: tc.url}).Validate},
				{"disburse", tc.disburse, (DisbursementRequest{AgreementURL: tc.url}).Validate},
			}
			for _, request := range requests {
				t.Run(request.name, func(t *testing.T) {
					err := request.check()
					t.Logf("[URL check] url=%s, want valid=%t, error=%v", tc.url, request.valid, err)
					if request.valid {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, ErrRequestParams)
					}
				})
			}
		})
	}
}
