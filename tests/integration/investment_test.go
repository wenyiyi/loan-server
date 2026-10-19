//go:build integration

package integration

import (
	"LoanService/api/v1"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"LoanService/config"
	"LoanService/internal/handler"
	"LoanService/internal/middleware"
	"LoanService/internal/model"
	"LoanService/internal/notification"
	"LoanService/internal/server"
	"LoanService/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func create(t *testing.T, s service.Service, principal int64) model.Loan {
	t.Helper()
	l, e := s.Create(context.Background(), v1.LoanCreateRequest{BorrowerID: "borrower", PrincipalAmount: principal, Rate: floatPointer(10), ROI: floatPointer(8), AgreementURL: "https://example.com/a.pdf"})
	if e != nil {
		t.Fatal(e)
	}
	if l.ID == 0 {
		t.Fatal("missing created ID")
	}
	loan, err := s.Get(context.Background(), loanID(l.ID))
	if err != nil {
		t.Fatal(err)
	}
	if loan.Remaining != principal {
		t.Fatalf("invalid remaining amount: %d", loan.Remaining)
	}
	return loan
}
func approve(t *testing.T, s service.Service, id string) {
	t.Helper()
	if e := s.Approve(context.Background(), id, "staff", v1.ApprovalRequest{VisitProofURL: "https://example.com/a.jpg", ApprovedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
}
func inv(key string, amount int64) v1.InvestmentRequest {
	return v1.InvestmentRequest{InvestorID: "investor", Email: "i@example.com", Key: key, Amount: amount}
}
func TestLifecycleAndIdempotency(t *testing.T) {
	s, p := setup(t)
	c := context.Background()
	l := create(t, s, 100)
	if _, e := s.Invest(c, loanID(l.ID), inv("early", 1)); !errors.Is(e, v1.ErrState) {
		t.Fatal(e)
	}
	if e := s.Disburse(c, loanID(l.ID), v1.DisbursementRequest{EmployeeID: "staff", AgreementURL: "https://example.com/s.pdf", At: time.Now()}); !errors.Is(e, v1.ErrState) {
		t.Fatal(e)
	}
	approve(t, s, loanID(l.ID))
	if e := s.Approve(c, loanID(l.ID), "staff", v1.ApprovalRequest{VisitProofURL: "https://example.com/a.jpg", ApprovedAt: time.Now()}); !errors.Is(e, v1.ErrState) {
		t.Fatal(e)
	}
	a, e := s.Invest(c, loanID(l.ID), inv("first", 40))
	if e != nil {
		t.Fatal(e)
	}
	if a.Total != 40 || a.Status != model.Approved {
		t.Fatal(a)
	}
	if _, e = s.Invest(c, loanID(l.ID), inv("too-much", 61)); !errors.Is(e, v1.ErrAmount) {
		t.Fatal(e)
	}
	b, e := s.Invest(c, loanID(l.ID), inv("second", 60))
	if e != nil || b.Status != model.Invested {
		t.Fatal(b, e)
	}
	retry, e := s.Invest(c, loanID(l.ID), inv("first", 40))
	if e != nil || retry != a {
		t.Fatal(retry, e)
	}
	if _, e = s.Invest(c, loanID(l.ID), inv("first", 41)); !errors.Is(e, v1.ErrIdempotency) {
		t.Fatal(e)
	}
	if e = s.Disburse(c, loanID(l.ID), v1.DisbursementRequest{EmployeeID: "staff", AgreementURL: "https://example.com/s.pdf", At: time.Now()}); e != nil {
		t.Fatal(e)
	}
	if e = s.Disburse(c, loanID(l.ID), v1.DisbursementRequest{EmployeeID: "staff", AgreementURL: "https://example.com/s.pdf", At: time.Now()}); !errors.Is(e, v1.ErrState) {
		t.Fatal(e)
	}
	var history, outbox int
	if e = p.WithContext(c).Raw("SELECT count(*) FROM loan_status_history WHERE loan_id=?", loanID(l.ID)).Row().Scan(&history); e != nil {
		t.Fatal(e)
	}
	if e = p.WithContext(c).Raw("SELECT count(*) FROM notification_outbox WHERE loan_id=?", loanID(l.ID)).Row().Scan(&outbox); e != nil {
		t.Fatal(e)
	}
	if history != 4 || outbox != 1 {
		t.Fatal(history, outbox)
	}
	sender := &fakeSender{fail: true}
	w := notification.Worker{DB: p, Sender: sender}
	if e = w.Once(c); e == nil {
		t.Fatal("expected mail failure")
	}
	var delivered bool
	if e = p.WithContext(c).Raw("SELECT delivered_at IS NOT NULL FROM notification_outbox").Row().Scan(&delivered); e != nil || delivered {
		t.Fatal(delivered, e)
	}
	if e = p.WithContext(c).Exec("UPDATE notification_outbox SET available_at=now()").Error; e != nil {
		t.Fatal(e)
	}
	sender.fail = false
	if e = w.Once(c); e != nil {
		t.Fatal(e)
	}
	if e = w.Once(c); e != nil {
		t.Fatal(e)
	}
	if sender.calls != 2 {
		t.Fatal(sender.calls)
	}
}

type fakeSender struct {
	fail  bool
	calls int
}

func (s *fakeSender) Send(context.Context, string, string, string) error {
	s.calls++
	if s.fail {
		return errors.New("mail down")
	}
	return nil
}
func TestConcurrentFunding(t *testing.T) {
	s, p := setup(t)
	l := create(t, s, 100)
	approve(t, s, loanID(l.ID))
	ctx := context.Background()
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Invest(ctx, loanID(l.ID), inv(fmt.Sprintf("concurrent-%d", i), 10))
			if e == nil {
				successes.Add(1)
			} else if !errors.Is(e, v1.ErrState) {
				t.Errorf("unexpected error: %v", e)
			}
		}(i)
	}
	wg.Wait()
	got, e := s.Get(ctx, loanID(l.ID))
	if e != nil || successes.Load() != 10 || got.InvestedAmount != 100 || got.Status != model.Invested {
		t.Fatal(got, e, successes.Load())
	}
	var total int64
	if e = p.WithContext(ctx).Raw("SELECT sum(amount) FROM loan_investments WHERE loan_id=?", loanID(l.ID)).Row().Scan(&total); e != nil || total != got.InvestedAmount {
		t.Fatal(total, e)
	}
}
func TestConcurrentDuplicateAndCrossLoanKey(t *testing.T) {
	s, _ := setup(t)
	l := create(t, s, 100)
	approve(t, s, loanID(l.ID))
	c := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.Invest(c, loanID(l.ID), inv("same-key", 10))
			if e != nil || v.Total != 10 {
				t.Errorf("duplicate result: %+v %v", v, e)
			}
		}()
	}
	wg.Wait()
	got, e := s.Get(c, loanID(l.ID))
	if e != nil || got.InvestedAmount != 10 {
		t.Fatal(got, e)
	}
	other := create(t, s, 100)
	approve(t, s, loanID(other.ID))
	if _, e = s.Invest(c, loanID(other.ID), inv("same-key", 10)); !errors.Is(e, v1.ErrIdempotency) {
		t.Fatal(e)
	}
}
func TestHTTPLifecycle(t *testing.T) {
	s, p := setup(t)
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	baseHandler := handler.NewHandler(logger)
	router := server.NewHTTPServer(&config.Config{HTTPAddr: ":8080"}, logger,
		handler.NewLoanHandler(baseHandler, s), handler.NewInvestmentHandler(baseHandler, s),
		middleware.Auth(map[string]model.Identity{"staff": {ID: "staff", Role: "staff"}, "investor": {ID: "investor", Role: "investor", Email: "i@example.com"}}), func(ctx context.Context) error {
			db, err := p.DB()
			if err != nil {
				return err
			}
			return db.PingContext(ctx)
		}).Handler
	call := func(path, token, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "http-investment")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	data := call("/api/v1/loans", "staff", `{"borrower_id":"borrower","principal_amount":100,"rate":10,"roi":8,"agreement_url":"https://example.com/loan.pdf"}`, 200)
	var response struct {
		Data model.Loan `json:"data"`
	}
	if e := json.Unmarshal(data, &response); e != nil {
		t.Fatal(e)
	}
	loan := response.Data
	base := fmt.Sprintf("/api/v1/loans/%s", loanID(loan.ID))
	call(base+"/approval", "staff", fmt.Sprintf(`{"validator_employee_id":"staff","visit_proof_url":"https://example.com/a.jpg","approved_at":%q}`, time.Now().Format(time.RFC3339Nano)), 200)
	call(base+"/investments", "investor", `{"investor_id":"investor","amount":100}`, 201)
	call(base+"/disbursement", "staff", fmt.Sprintf(`{"officer_employee_id":"staff","signed_agreement_url":"https://example.com/a.pdf","disbursed_at":%q}`, time.Now().Format(time.RFC3339Nano)), 200)
	call(base+"/investments", "investor", `{"investor_id":"investor","amount":100}`, 201)
	got, e := s.Get(context.Background(), loanID(loan.ID))
	if e != nil || got.Status != model.Disbursed {
		t.Fatal(got, e)
	}
}
func TestConcurrentOverInvestment(t *testing.T) {
	s, _ := setup(t)
	l := create(t, s, 100)
	approve(t, s, loanID(l.ID))
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Invest(context.Background(), loanID(l.ID), inv(fmt.Sprintf("over-%d", i), 80))
			if e == nil {
				successes.Add(1)
			} else if !errors.Is(e, v1.ErrAmount) {
				t.Errorf("unexpected error %v", e)
			}
		}(i)
	}
	wg.Wait()
	got, e := s.Get(context.Background(), loanID(l.ID))
	if e != nil || got.InvestedAmount != 80 || successes.Load() != 1 {
		t.Fatal(got, e, successes.Load())
	}
}

func floatPointer(value float64) *decimal.Decimal { d := decimal.NewFromFloat(value); return &d }

func TestConcurrentCrossLoanIdempotencyKey(t *testing.T) {
	svc, db := setup(t)
	loans := []model.Loan{create(t, svc, 100), create(t, svc, 100)}
	for _, loan := range loans {
		approve(t, svc, loanID(loan.ID))
	}
	start := make(chan struct{})
	results := make(chan error, len(loans))
	for _, loan := range loans {
		go func(id string) {
			<-start
			_, err := svc.Invest(context.Background(), id, inv("cross-loan-race", 10))
			results <- err
		}(loanID(loan.ID))
	}
	close(start)
	successes, conflicts := 0, 0
	for range loans {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, v1.ErrIdempotency):
			conflicts++
		default:
			t.Fatalf("unexpected key-race error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("got %d successes, %d conflicts", successes, conflicts)
	}
	var count int64
	if err := db.Table("loan_investments").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count %d, error %v", count, err)
	}
}
