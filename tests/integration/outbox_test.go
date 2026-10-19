//go:build integration

package integration

import (
	"context"
	"testing"

	"LoanService/internal/model"
)

func TestRollbackWhenOutboxFails(t *testing.T) {
	s, p := setup(t)
	l := create(t, s, 100)
	approve(t, s, loanID(l.ID))
	c := context.Background()
	if e := p.WithContext(c).Exec("ALTER TABLE notification_outbox ADD CONSTRAINT reject_test CHECK (false)").Error; e != nil {
		t.Fatal(e)
	}
	if _, e := s.Invest(c, loanID(l.ID), inv("rollback", 100)); e == nil {
		t.Fatal("expected transaction failure")
	}
	got, e := s.Get(c, loanID(l.ID))
	if e != nil || got.InvestedAmount != 0 || got.Status != model.Approved {
		t.Fatal(got, e)
	}
	var count int
	if e = p.WithContext(c).Raw("SELECT count(*) FROM loan_investments").Row().Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
