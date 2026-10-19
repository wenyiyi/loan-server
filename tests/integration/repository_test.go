//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"LoanService/internal/model"
	"LoanService/internal/repository"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestRepositoriesOnlyWriteTheirOwnTable(t *testing.T) {
	_, db := setup(t)
	ctx := context.Background()
	base := repository.NewRepository(db)
	loans := repository.NewLoanRepository(base)
	approvals := repository.NewApprovalRepository(base)
	t.Log("[Run] Create loan through repository; want 1 loan and no history")
	loan, err := loans.Create(ctx, model.Loan{LoanNo: "repository-test", BorrowerID: "borrower", PrincipalAmount: 100, Rate: decimal.NewFromInt(10), ROI: decimal.NewFromInt(8), AgreementURL: "https://example.com/a.pdf", Status: model.Proposed})
	require.NoError(t, err)
	assertTableCount(t, db, "loans", 1)
	assertTableCount(t, db, "loan_status_history", 0)
	t.Log("[Run] Create approval through repository; want 1 approval, no status change, no history")
	_, err = approvals.Create(ctx, model.Approval{LoanNo: loan.LoanNo, EmployeeID: "staff", VisitProofURL: "https://example.com/a.jpg", ApprovedAt: time.Now().UnixMilli()})
	require.NoError(t, err)
	stored, err := loans.GetByLoanNo(ctx, loan.LoanNo)
	require.NoError(t, err)
	t.Logf("[Check] Loan status=%s, want=PROPOSED", stored.Status)
	require.Equal(t, model.Proposed, stored.Status)
	assertTableCount(t, db, "approvals", 1)
	assertTableCount(t, db, "loan_status_history", 0)
}
