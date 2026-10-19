//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type apiFixture struct {
	db     *gorm.DB
	router http.Handler
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	svc, db := setup(t)
	return &apiFixture{db: db, router: setupRouter(t, svc, db)}
}

// sendAPI makes a complete request through the production router, including authentication.
// It does not use testing assertions, so concurrent tests can safely call it in goroutines.
func sendAPI(router http.Handler, method, path, token, key, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func (f *apiFixture) request(t *testing.T, method, path, token, key, body string, status, code int) *httptest.ResponseRecorder {
	t.Helper()
	t.Logf("[HTTP request] %s %s, key=%q, body=%s; want HTTP=%d, code=%d", method, path, key, body, status, code)
	response := sendAPI(f.router, method, path, token, key, body)
	t.Logf("[HTTP response] HTTP=%d, body=%s", response.Code, response.Body.String())
	require.Equal(t, status, response.Code, response.Body.String())
	var envelope v1.Response
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, code, envelope.Code, response.Body.String())
	return response
}

func (f *apiFixture) create(t *testing.T, principal int64) model.Loan {
	t.Helper()
	body := fmt.Sprintf(`{"borrower_id":"api-borrower","principal_amount":%d,"rate":10,"roi":8,"agreement_url":"https://example.com/a.pdf"}`, principal)
	// Create currently returns HTTP 200 through HandleSuccess; do not change the production API for a test.
	response := f.request(t, http.MethodPost, "/api/v1/loans", "staff-token", "", body, http.StatusOK, 0)
	var envelope struct {
		Data model.Loan `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.NotEmpty(t, envelope.Data.LoanNo)
	require.NoError(t, f.db.Where("loan_no = ?", envelope.Data.LoanNo).Take(&envelope.Data).Error)
	return envelope.Data
}

func (f *apiFixture) approve(t *testing.T, id string) {
	t.Helper()
	body := fmt.Sprintf(`{"validator_employee_id":"staff","visit_proof_url":"https://example.com/visit.jpg","approved_at":%q}`, time.Now().UTC().Format(time.RFC3339Nano))
	f.request(t, http.MethodPost, "/api/v1/loans/"+id+"/approval", "staff-token", "", body, http.StatusOK, 0)
}

func (f *apiFixture) investment(t *testing.T, id, key string, amount int64, status, code int) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"investor_id":"investor","amount":%d}`, amount)
	return f.request(t, http.MethodPost, "/api/v1/loans/"+id+"/investments", "investor-token", key, body, status, code)
}

func assertTableCount(t *testing.T, db *gorm.DB, table string, count int64) {
	t.Helper()
	var actual int64
	require.NoError(t, db.Table(table).Count(&actual).Error)
	t.Logf("[DB check] table=%s, want rows=%d, actual=%d, match=%t", table, count, actual, count == actual)
	assert.Equal(t, count, actual, table)
}

func TestCreateLoan(t *testing.T) {
	f := newAPIFixture(t)
	created := f.create(t, 100000)
	var stored model.Loan
	require.NoError(t, f.db.Where("loan_no = ?", created.LoanNo).Take(&stored).Error)
	t.Logf("[DB row] stored=%+v", stored)
	assert.Equal(t, "api-borrower", stored.BorrowerID)
	assert.Equal(t, int64(100000), stored.PrincipalAmount)
	assert.Zero(t, stored.InvestedAmount)
	assert.Equal(t, int64(100000), stored.PrincipalAmount-stored.InvestedAmount)
	assert.Equal(t, model.Proposed, stored.Status)
	assert.Equal(t, "10", stored.Rate.String())
	assert.Equal(t, "8", stored.ROI.String())
	assert.NotZero(t, stored.CreatedAt)
	assert.NotZero(t, stored.UpdatedAt)
	var history model.LoanStatusHistory
	require.NoError(t, f.db.Where("loan_no = ?", stored.LoanNo).Take(&history).Error)
	t.Logf("[DB row] history=%+v", history)
	assert.Equal(t, model.ActionCreate, history.Action)
	assert.Empty(t, history.PreviousState)
	assert.Equal(t, model.Proposed, history.NewState)
	assertTableCount(t, f.db, "loans", 1)
	assertTableCount(t, f.db, "loan_status_history", 1)
}

func TestApproveLoan(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loan.LoanNo)
	var stored model.Loan
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&stored).Error)
	t.Logf("[DB row] stored=%+v", stored)
	assert.Equal(t, model.Approved, stored.Status)
	var approval model.Approval
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&approval).Error)
	t.Logf("[DB row] approval=%+v", approval)
	assert.Equal(t, "staff", approval.EmployeeID)
	assert.Equal(t, "https://example.com/visit.jpg", approval.VisitProofURL)
	assert.GreaterOrEqual(t, approval.ApprovedAt, stored.CreatedAt)
	var history model.LoanStatusHistory
	require.NoError(t, f.db.Where("loan_no = ? AND new_state = ?", loan.LoanNo, model.Approved).Take(&history).Error)
	t.Logf("[DB row] history=%+v", history)
	assert.Equal(t, model.Proposed, history.PreviousState)
	assertTableCount(t, f.db, "loan_status_history", 2)
}

func TestInvestLoan(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loan.LoanNo)
	f.investment(t, loan.LoanNo, "first-investment", 40, http.StatusOK, 0)
	var stored model.Loan
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&stored).Error)
	t.Logf("[DB row] stored=%+v", stored)
	assert.Equal(t, int64(40), stored.InvestedAmount)
	assert.Equal(t, int64(60), stored.PrincipalAmount-stored.InvestedAmount)
	assert.Equal(t, model.Approved, stored.Status)
	var investment model.Investment
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&investment).Error)
	t.Logf("[DB row] investment=%+v", investment)
	assert.Equal(t, int64(40), investment.Amount)
	assert.Equal(t, "investor", investment.InvestorID)
	assert.Equal(t, "i@example.com", investment.Email)
	assert.Equal(t, "first-investment", investment.IdempotencyKey)
	f.investment(t, loan.LoanNo, "second-investment", 60, http.StatusOK, 0)
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&stored).Error)
	t.Logf("[DB row] stored=%+v", stored)
	assert.Equal(t, int64(100), stored.InvestedAmount)
	assert.Zero(t, stored.PrincipalAmount-stored.InvestedAmount)
	assert.Equal(t, model.Invested, stored.Status)
	assertTableCount(t, f.db, "investments", 2)
	assertTableCount(t, f.db, "loan_status_history", 3)
	assertTableCount(t, f.db, "notification_outbox", 1)
}

func TestOverInvestLoan(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loan.LoanNo)
	f.investment(t, loan.LoanNo, "accepted", 40, http.StatusOK, 0)
	// Over-investment must remain a public business conflict.
	response := f.investment(t, loan.LoanNo, "over-investment", 61, http.StatusConflict, v1.ErrAmount.Code)
	assert.NotContains(t, response.Body.String(), "SQL")
	var stored model.Loan
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&stored).Error)
	t.Logf("[DB row] stored=%+v", stored)
	assert.Equal(t, int64(40), stored.InvestedAmount)
	assert.Equal(t, int64(60), stored.PrincipalAmount-stored.InvestedAmount)
	assert.Equal(t, model.Approved, stored.Status)
	assertTableCount(t, f.db, "investments", 1)
	assertTableCount(t, f.db, "loan_status_history", 2)
	assertTableCount(t, f.db, "notification_outbox", 0)
}

func TestConcurrentInvest(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loan.LoanNo)
	const requests = 32
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- sendAPI(f.router, http.MethodPost, "/api/v1/loans/"+loan.LoanNo+"/investments", "investor-token",
				fmt.Sprintf("concurrent-api-%d", index), `{"investor_id":"investor","amount":10}`)
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for response := range results {
		t.Logf("[HTTP result] HTTP=%d, body=%s", response.Code, response.Body.String())
		var envelope v1.Response
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		if response.Code == http.StatusOK {
			successes++
			assert.Zero(t, envelope.Code)
		} else {
			assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
			assert.Contains(t, []int{v1.ErrState.Code, v1.ErrAmount.Code}, envelope.Code, response.Body.String())
		}
	}
	t.Logf("[Summary] Successes=%d, want=10; failures=%d, want=22", successes, requests-successes)
	assert.Equal(t, 10, successes)
	var stored model.Loan
	require.NoError(t, f.db.Where("loan_no = ?", loan.LoanNo).Take(&stored).Error)
	t.Logf("[DB row] stored=%+v", stored)
	assert.Equal(t, int64(100), stored.InvestedAmount)
	assert.Equal(t, model.Invested, stored.Status)
	var total int64
	require.NoError(t, f.db.Model(&model.Investment{}).Where("loan_no = ?", loan.LoanNo).Select("COALESCE(SUM(amount),0)").Scan(&total).Error)
	assert.Equal(t, stored.InvestedAmount, total)
	assertTableCount(t, f.db, "investments", 10)
	assertTableCount(t, f.db, "loan_status_history", 3)
	assertTableCount(t, f.db, "notification_outbox", 1)
}

func TestTransactionRollback(t *testing.T) {
	f := newAPIFixture(t)
	// Fail after the loan INSERT, while writing its CREATE audit record.
	require.NoError(t, f.db.Exec("ALTER TABLE loan_status_history ADD CONSTRAINT reject_create CHECK (action <> 'CREATE')").Error)
	body := `{"borrower_id":"rollback-borrower","principal_amount":100,"rate":10,"roi":8,"agreement_url":"https://example.com/a.pdf"}`
	response := f.request(t, http.MethodPost, "/api/v1/loans", "staff-token", "", body, http.StatusInternalServerError, v1.ErrInternalServerError.Code)
	assert.NotContains(t, response.Body.String(), "reject_create")
	assertTableCount(t, f.db, "loans", 0)
	assertTableCount(t, f.db, "loan_status_history", 0)
}

func TestAuthenticatedIdentityAndServerTimestamps(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	before := time.Now().UnixMilli()
	f.request(t, http.MethodPost, "/api/v1/loans/"+loan.LoanNo+"/approval", "staff-token", "", `{"employee_id":"forged","validator_employee_id":"forged","approved_at":1,"visit_proof_url":"https://example.com/v.jpg"}`, http.StatusOK, 0)
	var approval model.Approval
	require.NoError(t, f.db.Take(&approval).Error)
	t.Logf("[DB row] approval=%+v", approval)
	require.Equal(t, "staff", approval.EmployeeID)
	require.GreaterOrEqual(t, approval.ApprovedAt, before)
	require.LessOrEqual(t, approval.ApprovedAt, time.Now().UnixMilli())
	f.request(t, http.MethodPost, "/api/v1/loans/"+loan.LoanNo+"/investments", "investor-token", "identity", `{"investor_id":"forged","email":"forged@example.com","amount":100}`, http.StatusOK, 0)
	var investment model.Investment
	require.NoError(t, f.db.Take(&investment).Error)
	t.Logf("[DB row] investment=%+v", investment)
	require.Equal(t, "investor", investment.InvestorID)
	require.Equal(t, "i@example.com", investment.Email)
	before = time.Now().UnixMilli()
	f.request(t, http.MethodPost, "/api/v1/loans/"+loan.LoanNo+"/disbursement", "staff-token", "", `{"officer_employee_id":"forged","disbursed_at":1,"signed_agreement_url":"https://example.com/s.pdf"}`, http.StatusOK, 0)
	var d model.Disbursement
	require.NoError(t, f.db.Take(&d).Error)
	t.Logf("[DB row] d=%+v", d)
	require.Equal(t, "staff", d.OfficerEmployeeID)
	require.GreaterOrEqual(t, d.DisbursedAt, before)
	require.LessOrEqual(t, d.DisbursedAt, time.Now().UnixMilli())
	require.Equal(t, model.Disbursed, storedLoan(t, f.db, loan.LoanNo).Status)
}
func TestCreateInvalidParameters(t *testing.T) {
	f := newAPIFixture(t)
	for _, tc := range []struct {
		name      string
		principal int64
		rate, roi string
	}{{"zero principal", 0, "10", "8"}, {"negative principal", -1, "10", "8"}, {"negative rate", 100, "-1", "0"}, {"high rate", 100, "101", "8"}, {"negative ROI", 100, "10", "-1"}, {"high ROI", 100, "100", "101"}, {"ROI exceeds rate", 100, "10", "11"}, {"missing rate", 100, "null", "8"}, {"missing ROI", 100, "10", "null"}} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"borrower_id":"borrower","principal_amount":%d,"rate":%s,"roi":%s,"agreement_url":"https://example.com/a.pdf"}`, tc.principal, tc.rate, tc.roi)
			f.request(t, http.MethodPost, "/api/v1/loans", "staff-token", "", body, v1.ErrRequestParams.HTTPStatus, v1.ErrRequestParams.Code)
		})
	}
	assertTableCount(t, f.db, "loans", 0)
	assertTableCount(t, f.db, "loan_status_history", 0)
}
