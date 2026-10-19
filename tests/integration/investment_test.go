//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"
	"LoanService/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func floatPointer(v float64) *decimal.Decimal { d := decimal.NewFromFloat(v); return &d }
func create(t *testing.T, s service.Service, principal int64) model.Loan {
	t.Helper()
	t.Logf("[Setup] Create loan: principal=%d, rate=10, ROI=8; want status=PROPOSED", principal)
	r, err := s.Create(context.Background(), v1.LoanCreateRequest{BorrowerID: "borrower", PrincipalAmount: principal, Rate: floatPointer(10), ROI: floatPointer(8), AgreementURL: "https://example.com/a.pdf"})
	require.NoError(t, err)
	require.NotEmpty(t, r.LoanNo)
	t.Logf("[Result] Loan created: loan_no=%s", r.LoanNo)
	return model.Loan{LoanNo: r.LoanNo}
}
func approve(t *testing.T, s service.Service, no string) {
	t.Helper()
	t.Logf("[Run] Approve loan=%s, staff=staff; want PROPOSED -> APPROVED", no)
	require.NoError(t, s.Approve(context.Background(), no, "staff", v1.ApprovalRequest{VisitProofURL: "https://example.com/v.jpg"}))
	t.Log("[Pass] Approval succeeded")
}
func storedLoan(t *testing.T, db *gorm.DB, no string) model.Loan {
	t.Helper()
	var l model.Loan
	require.NoError(t, db.Where("loan_no = ?", no).Take(&l).Error)
	t.Logf("[DB] loan=%s, status=%s, principal=%d, invested=%d, remaining=%d", l.LoanNo, l.Status, l.PrincipalAmount, l.InvestedAmount, l.PrincipalAmount-l.InvestedAmount)
	return l
}

// Each callback uses the production pool and opens its own service transaction.
func concurrently(n int, fn func(int) error) []error {
	start := make(chan struct{})
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; results[i] = fn(i) }(i)
	}
	close(start)
	wg.Wait()
	return results
}
func TestInvestmentLifecycleAndIdempotency(t *testing.T) {
	s, db := setup(t)
	c := context.Background()
	l := create(t, s, 100)
	t.Log("[Run] Invest 1 before approval; want ErrState")
	_, err := s.Invest(c, l.LoanNo, "investor", "i@example.com", "early", v1.InvestmentRequest{Amount: 1})
	t.Logf("[Result] error=%v", err)
	require.ErrorIs(t, err, v1.ErrState)
	approve(t, s, l.LoanNo)
	for _, amount := range []int64{0, -1} {
		t.Logf("[Run] Invest amount=%d; want ErrRequestParams", amount)
		_, err = s.Invest(c, l.LoanNo, "investor", "i@example.com", fmt.Sprint(amount), v1.InvestmentRequest{Amount: amount})
		require.ErrorIs(t, err, v1.ErrRequestParams)
	}
	first, err := s.Invest(c, l.LoanNo, "investor", "i@example.com", "first", v1.InvestmentRequest{Amount: 40})
	t.Logf("[Result] Invest 40; want APPROVED, invested=40, remaining=60; actual=%+v, error=%v", first, err)
	require.NoError(t, err)
	require.Equal(t, model.Approved, first.Status)
	require.EqualValues(t, 60, first.RemainingAmount)
	_, err = s.Invest(c, l.LoanNo, "investor", "i@example.com", "over", v1.InvestmentRequest{Amount: 61})
	require.ErrorIs(t, err, v1.ErrAmount)
	last, err := s.Invest(c, l.LoanNo, "investor", "i@example.com", "last", v1.InvestmentRequest{Amount: 60})
	t.Logf("[Result] Invest 60; want INVESTED, invested=100, remaining=0; actual=%+v, error=%v", last, err)
	require.NoError(t, err)
	require.Equal(t, model.Invested, last.Status)
	retry, err := s.Invest(c, l.LoanNo, "investor", "i@example.com", "first", v1.InvestmentRequest{Amount: 40})
	t.Logf("[Result] Retry first key; want original result, no new row; actual=%+v, error=%v", retry, err)
	require.NoError(t, err)
	require.Equal(t, first, retry)
	for _, tc := range []struct {
		investor string
		email    string
		req      v1.InvestmentRequest
	}{{"investor", "i@example.com", v1.InvestmentRequest{Amount: 41}}, {"other", "i@example.com", v1.InvestmentRequest{Amount: 40}}, {"investor", "other@example.com", v1.InvestmentRequest{Amount: 40}}} {
		t.Logf("[Run] Reuse first key with changed data: investor=%s, amount=%d, email=%s; want ErrIdempotency", tc.investor, tc.req.Amount, tc.email)
		_, err = s.Invest(c, l.LoanNo, tc.investor, tc.email, "first", tc.req)
		t.Logf("[Result] error=%v", err)
		require.ErrorIs(t, err, v1.ErrIdempotency)
	}
	other := create(t, s, 100)
	approve(t, s, other.LoanNo)
	_, err = s.Invest(c, other.LoanNo, "investor", "i@example.com", "first", v1.InvestmentRequest{Amount: 40})
	require.ErrorIs(t, err, v1.ErrIdempotency)
	require.EqualValues(t, 100, storedLoan(t, db, l.LoanNo).InvestedAmount)
	assertTableCount(t, db, "investments", 2)
	assertTableCount(t, db, "notification_outbox", 1)
	var h int64
	require.NoError(t, db.Model(&model.LoanStatusHistory{}).Where("loan_no = ? AND action = ?", l.LoanNo, model.ActionFullyFunded).Count(&h).Error)
	t.Logf("[Check] Funding history count=%d, want=1", h)
	require.EqualValues(t, 1, h)
}
func TestConcurrentFunding(t *testing.T) {
	s, db := setup(t)
	l := create(t, s, 100)
	approve(t, s, l.LoanNo)
	t.Log("[Run] Start 32 investors together, amount=10, principal=100; want 10 successes and 22 ErrState errors")
	results := concurrently(32, func(i int) error {
		_, err := s.Invest(context.Background(), l.LoanNo, fmt.Sprintf("investor-%d", i), "i@example.com", fmt.Sprintf("key-%d", i), v1.InvestmentRequest{Amount: 10})
		return err
	})
	successes := 0
	for i, err := range results {
		t.Logf("[Result] Request[%d] error=%v; want success or ErrState", i, err)
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, v1.ErrState)
		}
	}
	require.Equal(t, 10, successes)
	got := storedLoan(t, db, l.LoanNo)
	require.Equal(t, model.Invested, got.Status)
	require.EqualValues(t, 100, got.InvestedAmount)
	var total int64
	require.NoError(t, db.Model(&model.Investment{}).Select("COALESCE(SUM(amount),0)").Scan(&total).Error)
	require.Equal(t, got.InvestedAmount, total)
	assertTableCount(t, db, "investments", 10)
	assertTableCount(t, db, "loan_status_history", 3)
	assertTableCount(t, db, "notification_outbox", 10)
}
func TestConcurrentDuplicateInvestment(t *testing.T) {
	s, db := setup(t)
	l := create(t, s, 100)
	approve(t, s, l.LoanNo)
	values := make([]*v1.InvestmentResult, 16)
	t.Log("[Run] Start 16 requests with the same key, amount=100; want equal results, 1 investment, and 1 notice")
	errs := concurrently(16, func(i int) error {
		var err error
		values[i], err = s.Invest(context.Background(), l.LoanNo, "investor", "i@example.com", "same", v1.InvestmentRequest{Amount: 100})
		return err
	})
	for i, err := range errs {
		t.Logf("[Retry] Request[%d] error=%v, result=%+v; want same result as first request", i, err, values[i])
		require.NoError(t, err)
		require.Equal(t, values[0], values[i])
	}
	require.EqualValues(t, 100, storedLoan(t, db, l.LoanNo).InvestedAmount)
	assertTableCount(t, db, "investments", 1)
	assertTableCount(t, db, "notification_outbox", 1)
	assertTableCount(t, db, "loan_status_history", 3)
}
func TestConcurrentOverInvestment(t *testing.T) {
	t.Log("[Case] Two requests invest 80 each, principal=100; want 1 success, 1 ErrAmount, invested=80")
	s, db := setup(t)
	l := create(t, s, 100)
	approve(t, s, l.LoanNo)
	errs := concurrently(2, func(i int) error {
		_, err := s.Invest(context.Background(), l.LoanNo, "investor", "i@example.com", fmt.Sprint(i), v1.InvestmentRequest{Amount: 80})
		return err
	})
	n := 0
	for i, err := range errs {
		t.Logf("[Result] Request[%d] error=%v", i, err)
		if err == nil {
			n++
		} else {
			require.ErrorIs(t, err, v1.ErrAmount)
		}
	}
	require.Equal(t, 1, n)
	got := storedLoan(t, db, l.LoanNo)
	require.EqualValues(t, 80, got.InvestedAmount)
	require.Equal(t, model.Approved, got.Status)
	assertTableCount(t, db, "investments", 1)
	assertTableCount(t, db, "notification_outbox", 0)
}
func TestConcurrentCrossLoanKey(t *testing.T) {
	t.Log("[Case] Two loans use global-key for amount=10; wait before INSERT; want 1 success and 1 ErrIdempotency")
	s, db := setup(t)
	// Force both transactions past the key lookup before either INSERT executes.
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:key_race", func(tx *gorm.DB) {
		if tx.Statement.Table == "investments" {
			arrived <- struct{}{}
			<-release
		}
	}))
	defer func() { require.NoError(t, db.Callback().Create().Remove("test:key_race")) }()
	go func() { <-arrived; <-arrived; close(release) }()

	loans := []model.Loan{create(t, s, 100), create(t, s, 100)}
	for _, l := range loans {
		approve(t, s, l.LoanNo)
	}
	errs := concurrently(2, func(i int) error {
		_, err := s.Invest(context.Background(), loans[i].LoanNo, "investor", "i@example.com", "global-key", v1.InvestmentRequest{Amount: 10})
		return err
	})
	n := 0
	for i, err := range errs {
		t.Logf("[Result] Request[%d] error=%v", i, err)
		if err == nil {
			n++
		} else {
			require.ErrorIs(t, err, v1.ErrIdempotency)
		}
	}
	require.Equal(t, 1, n)
	assertTableCount(t, db, "investments", 1)
	require.EqualValues(t, 10, storedLoan(t, db, loans[0].LoanNo).InvestedAmount+storedLoan(t, db, loans[1].LoanNo).InvestedAmount)
}
func TestTransitionsConcurrentAndTimestamps(t *testing.T) {
	for _, transition := range []string{"approve", "disburse"} {
		t.Run(transition, func(t *testing.T) {
			s, db := setup(t)
			l := create(t, s, 100)
			c := context.Background()
			if transition == "disburse" {
				approve(t, s, l.LoanNo)
				_, err := s.Invest(c, l.LoanNo, "investor", "i@example.com", "fund", v1.InvestmentRequest{Amount: 100})
				require.NoError(t, err)
			}
			t.Logf("[Run] Start 2 %s requests for one loan; want 1 success, 1 ErrState, 1 record, and 1 history row", transition)
			before := time.Now().UnixMilli()
			errs := concurrently(2, func(int) error {
				if transition == "approve" {
					return s.Approve(c, l.LoanNo, "staff", v1.ApprovalRequest{VisitProofURL: "https://example.com/a.jpg"})
				}
				return s.Disburse(c, l.LoanNo, "staff", v1.DisbursementRequest{AgreementURL: "https://example.com/a.pdf"})
			})
			after := time.Now().UnixMilli()
			n := 0
			for i, err := range errs {
				t.Logf("[Result] Request[%d] error=%v", i, err)
				if err == nil {
					n++
				} else {
					require.ErrorIs(t, err, v1.ErrState)
				}
			}
			require.Equal(t, 1, n)
			var timestamp int64
			var action model.Action
			if transition == "approve" {
				var a model.Approval
				require.NoError(t, db.Take(&a).Error)
				timestamp = a.ApprovedAt
				require.Equal(t, "staff", a.EmployeeID)
				assertTableCount(t, db, "approvals", 1)
				require.Equal(t, model.Approved, storedLoan(t, db, l.LoanNo).Status)
				action = model.ActionApprove
			} else {
				var d model.Disbursement
				require.NoError(t, db.Take(&d).Error)
				timestamp = d.DisbursedAt
				require.Equal(t, "staff", d.OfficerEmployeeID)
				assertTableCount(t, db, "disbursements", 1)
				require.Equal(t, model.Disbursed, storedLoan(t, db, l.LoanNo).Status)
				action = model.ActionDisburse
			}
			t.Logf("[Check] Server time=%d; want UnixMilli in [%d, %d]", timestamp, before, after)
			require.GreaterOrEqual(t, timestamp, before)
			require.LessOrEqual(t, timestamp, after)
			var count int64
			require.NoError(t, db.Model(&model.LoanStatusHistory{}).Where("action = ?", action).Count(&count).Error)
			t.Logf("[Check] action=%s, count=%d, want=1", action, count)
			require.EqualValues(t, 1, count)
		})
	}
}
func TestInvalidTransitionStates(t *testing.T) {
	for _, status := range []model.Status{model.Proposed, model.Approved, model.Invested, model.Disbursed} {
		t.Run(string(status), func(t *testing.T) {
			t.Logf("[Case] status=%s; approve requires PROPOSED, disburse requires INVESTED; want ErrState otherwise and no DB change", status)
			s, db := setup(t)
			l := create(t, s, 100)
			require.NoError(t, db.Model(&model.Loan{}).Where("loan_no = ?", l.LoanNo).Update("status", status).Error)
			if status != model.Proposed {
				require.ErrorIs(t, s.Approve(context.Background(), l.LoanNo, "staff", v1.ApprovalRequest{}), v1.ErrState)
			}
			if status != model.Invested {
				require.ErrorIs(t, s.Disburse(context.Background(), l.LoanNo, "staff", v1.DisbursementRequest{}), v1.ErrState)
			}
			require.Equal(t, status, storedLoan(t, db, l.LoanNo).Status)
			assertTableCount(t, db, "approvals", 0)
			assertTableCount(t, db, "disbursements", 0)
			assertTableCount(t, db, "loan_status_history", 1)
		})
	}
}
func TestCreateHistoryFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "create", "loan_status_history")
}

func TestApproveRecordFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "approve", "approvals")
}

func TestApproveHistoryFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "approve", "loan_status_history")
}

func TestInvestRecordFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "invest", "investments")
}

func TestInvestHistoryFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "invest", "loan_status_history")
}

func TestInvestOutboxFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "invest", "notification_outbox")
}

func TestDisburseRecordFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "disburse", "disbursements")
}

func TestDisburseHistoryFailureRollsBack(t *testing.T) {
	assertTransactionFailureRollback(t, "disburse", "loan_status_history")
}

func assertTransactionFailureRollback(t *testing.T, op, tableName string) {
	t.Helper()
	t.Logf("[1/5 Setup] action=%s, failing table=%s", op, tableName)
	s, db := setup(t)
	c := context.Background()
	l := create(t, s, 100)
	if op == "invest" || op == "disburse" {
		approve(t, s, l.LoanNo)
	}
	if op == "disburse" {
		_, err := s.Invest(c, l.LoanNo, "investor", "i@example.com", "fund", v1.InvestmentRequest{Amount: 100})
		require.NoError(t, err)
	}
	before := storedLoan(t, db, l.LoanNo)
	tables := []string{"loans", "approvals", "investments", "disbursements", "loan_status_history", "notification_outbox"}
	counts := make([]int64, len(tables))
	for i, table := range tables {
		require.NoError(t, db.Table(table).Count(&counts[i]).Error)
		t.Logf("[2/5 Before] table=%s, row count=%d", table, counts[i])
	}
	t.Logf("[3/5 Fail] Add CHECK(false) NOT VALID to test table %s; keep old rows, reject new writes", tableName)
	// NOT VALID leaves existing rows intact, but rejects every subsequent INSERT.
	require.NoError(t, db.Exec("ALTER TABLE "+tableName+" ADD CONSTRAINT reject_test CHECK (false) NOT VALID").Error)
	var err error
	switch op {
	case "create":
		_, err = s.Create(c, v1.LoanCreateRequest{BorrowerID: "rollback", PrincipalAmount: 100, Rate: floatPointer(10), ROI: floatPointer(8)})
	case "approve":
		err = s.Approve(c, l.LoanNo, "staff", v1.ApprovalRequest{})
	case "invest":
		_, err = s.Invest(c, l.LoanNo, "investor", "i@example.com", "rollback", v1.InvestmentRequest{Amount: 100})
	case "disburse":
		err = s.Disburse(c, l.LoanNo, "staff", v1.DisbursementRequest{})
	}
	t.Logf("[4/5 Result] %s error=%v; want failure and full rollback", op, err)
	require.Error(t, err)
	t.Logf("[5/5 Check] Want loan status=%s, invested=%d, and all row counts unchanged", before.Status, before.InvestedAmount)
	require.Equal(t, before, storedLoan(t, db, l.LoanNo))
	for i, table := range tables {
		assertTableCount(t, db, table, counts[i])
	}
}
