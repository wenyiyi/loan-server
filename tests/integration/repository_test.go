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
	loan, err := loans.Create(ctx, model.Loan{BorrowerID: "borrower", PrincipalAmount: 100, Rate: decimal.NewFromInt(10), ROI: decimal.NewFromInt(8), AgreementURL: "https://example.com/a.pdf", Status: model.Proposed})
	require.NoError(t, err)
	assertTableCount(t, db, "loans", 1)
	assertTableCount(t, db, "loan_status_history", 0)
	_, err = approvals.Create(ctx, model.Approval{LoanID: loanID(loan.ID), ValidatorEmployeeID: "staff", VisitProofURL: "https://example.com/a.jpg", ApprovedAt: time.Now()})
	require.NoError(t, err)
	stored, err := loans.GetById(ctx, loanID(loan.ID))
	require.NoError(t, err)
	require.Equal(t, model.Proposed, stored.Status)
	assertTableCount(t, db, "loan_approvals", 1)
	assertTableCount(t, db, "loan_status_history", 0)
}
