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
	response := sendAPI(f.router, method, path, token, key, body)
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
	require.NotEmpty(t, envelope.Data.ID)
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
	assert.Equal(t, count, actual, table)
}

func TestCreateLoan(t *testing.T) {
	f := newAPIFixture(t)
	created := f.create(t, 100000)
	var stored model.Loan
	require.NoError(t, f.db.Where("id = ?", loanID(created.ID)).Take(&stored).Error)
	assert.Equal(t, "api-borrower", stored.BorrowerID)
	assert.Equal(t, int64(100000), stored.PrincipalAmount)
	assert.Zero(t, stored.InvestedAmount)
	assert.Equal(t, int64(100000), stored.Remaining)
	assert.Equal(t, model.Proposed, stored.Status)
	assert.Equal(t, "10", stored.Rate.String())
	assert.Equal(t, "8", stored.ROI.String())
	assert.NotZero(t, stored.CreatedAt)
	assert.NotZero(t, stored.UpdatedAt)
	var history model.LoanStatusHistory
	require.NoError(t, f.db.Where("loan_id = ?", loanID(stored.ID)).Take(&history).Error)
	assert.Equal(t, model.ActionCreate, history.Action)
	assert.Empty(t, history.PreviousState)
	assert.Equal(t, model.Proposed, history.NewState)
	assertTableCount(t, f.db, "loans", 1)
	assertTableCount(t, f.db, "loan_status_history", 1)
}

func TestApproveLoan(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loanID(loan.ID))
	var stored model.Loan
	require.NoError(t, f.db.Where("id = ?", loanID(loan.ID)).Take(&stored).Error)
	assert.Equal(t, model.Approved, stored.Status)
	var approval model.Approval
	require.NoError(t, f.db.Where("loan_id = ?", loanID(loan.ID)).Take(&approval).Error)
	assert.Equal(t, "staff", approval.ValidatorEmployeeID)
	assert.Equal(t, "https://example.com/visit.jpg", approval.VisitProofURL)
	assert.False(t, approval.ApprovedAt.Before(stored.CreatedAt))
	var history model.LoanStatusHistory
	require.NoError(t, f.db.Where("loan_id = ? AND new_state = ?", loanID(loan.ID), model.Approved).Take(&history).Error)
	assert.Equal(t, model.Proposed, history.PreviousState)
	assertTableCount(t, f.db, "loan_status_history", 2)
}

func TestInvestLoan(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loanID(loan.ID))
	f.investment(t, loanID(loan.ID), "first-investment", 40, http.StatusCreated, 0)
	var stored model.Loan
	require.NoError(t, f.db.Where("id = ?", loanID(loan.ID)).Take(&stored).Error)
	assert.Equal(t, int64(40), stored.InvestedAmount)
	assert.Equal(t, int64(60), stored.Remaining)
	assert.Equal(t, model.Approved, stored.Status)
	var investment model.Investment
	require.NoError(t, f.db.Where("loan_id = ?", loanID(loan.ID)).Take(&investment).Error)
	assert.Equal(t, int64(40), investment.Amount)
	assert.Equal(t, "investor", investment.InvestorID)
	assert.Equal(t, "i@example.com", investment.Email)
	assert.Equal(t, "first-investment", investment.IdempotencyKey)
	f.investment(t, loanID(loan.ID), "second-investment", 60, http.StatusCreated, 0)
	require.NoError(t, f.db.Where("id = ?", loanID(loan.ID)).Take(&stored).Error)
	assert.Equal(t, int64(100), stored.InvestedAmount)
	assert.Zero(t, stored.Remaining)
	assert.Equal(t, model.Invested, stored.Status)
	assertTableCount(t, f.db, "loan_investments", 2)
	assertTableCount(t, f.db, "loan_status_history", 3)
	assertTableCount(t, f.db, "notification_outbox", 1)
}

func TestOverInvestLoan(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loanID(loan.ID))
	f.investment(t, loanID(loan.ID), "accepted", 40, http.StatusCreated, 0)
	// The current handler returns HTTP 500 with numeric business code 1002 for a repository error.
	response := f.investment(t, loanID(loan.ID), "over-investment", 61, http.StatusConflict, v1.ErrAmount.Code)
	assert.NotContains(t, response.Body.String(), "SQL")
	var stored model.Loan
	require.NoError(t, f.db.Where("id = ?", loanID(loan.ID)).Take(&stored).Error)
	assert.Equal(t, int64(40), stored.InvestedAmount)
	assert.Equal(t, int64(60), stored.Remaining)
	assert.Equal(t, model.Approved, stored.Status)
	assertTableCount(t, f.db, "loan_investments", 1)
	assertTableCount(t, f.db, "loan_status_history", 2)
	assertTableCount(t, f.db, "notification_outbox", 0)
}

func TestConcurrentInvest(t *testing.T) {
	f := newAPIFixture(t)
	loan := f.create(t, 100)
	f.approve(t, loanID(loan.ID))
	const requests = 32
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- sendAPI(f.router, http.MethodPost, "/api/v1/loans/"+loanID(loan.ID)+"/investments", "investor-token",
				fmt.Sprintf("concurrent-api-%d", index), `{"investor_id":"investor","amount":10}`)
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for response := range results {
		var envelope v1.Response
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		if response.Code == http.StatusCreated {
			successes++
			assert.Zero(t, envelope.Code)
		} else {
			assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
			assert.Contains(t, []int{v1.ErrState.Code, v1.ErrAmount.Code}, envelope.Code, response.Body.String())
		}
	}
	assert.Equal(t, 10, successes)
	var stored model.Loan
	require.NoError(t, f.db.Where("id = ?", loanID(loan.ID)).Take(&stored).Error)
	assert.Equal(t, int64(100), stored.InvestedAmount)
	assert.Equal(t, model.Invested, stored.Status)
	var total int64
	require.NoError(t, f.db.Model(&model.Investment{}).Where("loan_id = ?", loanID(loan.ID)).Select("COALESCE(SUM(amount),0)").Scan(&total).Error)
	assert.Equal(t, stored.InvestedAmount, total)
	assertTableCount(t, f.db, "loan_investments", 10)
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
