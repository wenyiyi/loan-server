//go:build integration

package integration

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestGetLoanLifecycle(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	path := "/api/v1/loans/" + loan.LoanNo
	check := func(status model.Status, invested int64) {
		t.Helper()
		r := f.request(t, http.MethodGet, path, "staff-token", "", "", 200, 0)
		var result struct {
			Data model.Loan `json:"data"`
		}
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &result))
		got := result.Data
		t.Logf("[Get] status=%s, invested=%d, remaining=%d; want status=%s, invested=%d", got.Status, got.InvestedAmount, got.Remaining, status, invested)
		require.Equal(t, loan.LoanNo, got.LoanNo)
		require.Equal(t, "api-borrower", got.BorrowerID)
		require.EqualValues(t, 100, got.PrincipalAmount)
		require.Equal(t, invested, got.InvestedAmount)
		require.Equal(t, 100-invested, got.Remaining)
		require.Equal(t, status, got.Status)
		require.Equal(t, "10", got.Rate.String())
		require.Equal(t, "8", got.ROI.String())
		require.Equal(t, loan.AgreementURL, got.AgreementURL)
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &envelope))
		require.NotContains(t, envelope.Data, "id")
	}
	check(model.Proposed, 0)
	f.approve(t, loan.LoanNo)
	check(model.Approved, 0)
	f.investment(t, loan.LoanNo, "partial", 40, 200, 0)
	check(model.Approved, 40)
	f.investment(t, loan.LoanNo, "full", 60, 200, 0)
	check(model.Invested, 100)
	f.request(t, http.MethodPost, path+"/disbursement", "staff-token", "", `{"signed_agreement_url":"https://example.com/s.pdf"}`, 200, 0)
	check(model.Disbursed, 100)
	assertTableCount(t, f.db, "loans", 1)
	assertTableCount(t, f.db, "loan_status_history", 4)
}
func TestGetLoanAccessAndMissing(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	path := "/api/v1/loans/" + loan.LoanNo
	f.request(t, http.MethodGet, path, "", "", "", 401, v1.ErrUnauthorized.Code)
	f.request(t, http.MethodGet, path, "investor-token", "", "", 403, v1.ErrForbidden.Code)
	f.request(t, http.MethodGet, "/api/v1/loans/missing", "staff-token", "", "", 404, v1.ErrNotFound.Code)
	assertTableCount(t, f.db, "loans", 1)
	assertTableCount(t, f.db, "loan_status_history", 1)
}
func TestDocumentURLsRejectedBeforeWrites(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.request(t, http.MethodPost, "/api/v1/loans", "staff-token", "", `{"borrower_id":"b","principal_amount":100,"rate":10,"roi":8,"agreement_url":"https://example.com/a.jpg"}`, 400, v1.ErrRequestParams.Code)
	path := "/api/v1/loans/" + loan.LoanNo
	f.request(t, http.MethodPost, path+"/approval", "staff-token", "", `{"visit_proof_url":"https://example.com/a.pdf"}`, 400, v1.ErrRequestParams.Code)
	require.Equal(t, model.Proposed, storedLoan(t, f.db, loan.LoanNo).Status)
	f.approve(t, loan.LoanNo)
	f.investment(t, loan.LoanNo, "fund", 100, 200, 0)
	f.request(t, http.MethodPost, path+"/disbursement", "staff-token", "", `{"signed_agreement_url":"https://example.com/a.png"}`, 400, v1.ErrRequestParams.Code)
	require.Equal(t, model.Invested, storedLoan(t, f.db, loan.LoanNo).Status)
	assertTableCount(t, f.db, "loans", 1)
	assertTableCount(t, f.db, "approvals", 1)
	assertTableCount(t, f.db, "disbursements", 0)
	assertTableCount(t, f.db, "loan_status_history", 3)
}
