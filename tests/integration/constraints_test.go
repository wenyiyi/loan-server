//go:build integration

package integration

import (
	"LoanService/internal/model"
	"context"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestLoanDatabaseChecks(t *testing.T) {
	svc, db := setup(t)
	loan := create(t, svc, 100)
	for _, tc := range []struct{ name, sql, constraint string }{
		{"zero principal", "principal_amount=0", "chk_loan_amounts"},
		{"negative principal", "principal_amount=-1", "chk_loan_amounts"},
		{"negative total", "invested_amount=-1", "chk_loan_amounts"},
		{"over total", "invested_amount=101", "chk_loan_amounts"},
		{"negative rate", "rate=-1", "chk_loan_rates"},
		{"high rate", "rate=101", "chk_loan_rates"},
		{"negative ROI", "roi=-1", "chk_loan_rates"},
		{"ROI exceeds rate", "roi=11", "chk_loan_rates"},
		{"invalid status", "status='UNKNOWN'", "chk_loan_status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := db.Exec("UPDATE loans SET "+tc.sql+" WHERE loan_no = ?", loan.LoanNo).Error
			t.Logf("[DB check] change=%s, error=%v; want constraint=%s", tc.sql, err, tc.constraint)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, "23514", pgErr.Code)
			require.Equal(t, tc.constraint, pgErr.ConstraintName)
		})
	}
	got := storedLoan(t, db, loan.LoanNo)
	require.EqualValues(t, 100, got.PrincipalAmount)
	require.Zero(t, got.InvestedAmount)
	require.Equal(t, model.Proposed, got.Status)
	for _, rates := range []string{"rate=0,roi=0", "rate=100,roi=100"} {
		require.NoError(t, db.Exec("UPDATE loans SET "+rates+" WHERE loan_no = ?", loan.LoanNo).Error)
	}
}
func TestInvestmentAmountAndApprovalUniqueChecks(t *testing.T) {
	svc, db := setup(t)
	loan := create(t, svc, 100)
	approve(t, svc, loan.LoanNo)
	for _, amount := range []int64{0, -1} {
		value := model.Investment{LoanNo: loan.LoanNo, InvestorID: "i", Email: "i@example.com", Amount: amount, IdempotencyKey: "invalid", Response: []byte(`{}`)}
		err := db.Create(&value).Error
		t.Logf("[DB check] amount=%d, error=%v; want chk_investment_amount", amount, err)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		require.Equal(t, "23514", pgErr.Code)
		require.Equal(t, "chk_investment_amount", pgErr.ConstraintName)
	}
	err := db.Create(&model.Approval{LoanNo: loan.LoanNo, EmployeeID: "other"}).Error
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23505", pgErr.Code)
	require.Equal(t, "idx_approvals_loan_no", pgErr.ConstraintName)
	assertTableCount(t, db, "investments", 0)
	assertTableCount(t, db, "approvals", 1)
}
func TestConstraintsMigrationRoundTrip(t *testing.T) {
	svc, db := setup(t)
	loan := create(t, svc, 100)
	for _, direction := range []string{"down", "up"} {
		sql, err := os.ReadFile("../../migrations/002_data_constraints." + direction + ".sql")
		require.NoError(t, err)
		require.NoError(t, db.Exec(string(sql)).Error)
		t.Logf("[Migration] 002 %s complete; check data is kept", direction)
		require.EqualValues(t, 100, storedLoan(t, db, loan.LoanNo).PrincipalAmount)
	}
	err := db.Exec("UPDATE loans SET principal_amount=-1 WHERE loan_no=?", loan.LoanNo).Error
	require.Error(t, err)
	assertTableCount(t, db, "loans", 1)
	assertTableCount(t, db, "loan_status_history", 1)
}
func TestConstraintsMigrationRejectsInvalidOldData(t *testing.T) {
	svc, db := setup(t)
	loan := create(t, svc, 100)
	down, err := os.ReadFile("../../migrations/002_data_constraints.down.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(down)).Error)
	require.NoError(t, db.Exec("UPDATE loans SET principal_amount=-1 WHERE loan_no=?", loan.LoanNo).Error)
	up, err := os.ReadFile("../../migrations/002_data_constraints.up.sql")
	require.NoError(t, err)
	// Keep the failed explicit transaction on one connection so ROLLBACK releases it.
	require.NoError(t, db.Connection(func(conn *gorm.DB) error {
		err := conn.WithContext(context.Background()).Exec(string(up)).Error
		require.Error(t, err)
		require.NoError(t, conn.Exec("ROLLBACK").Error)
		return nil
	}))
	require.EqualValues(t, -1, storedLoan(t, db, loan.LoanNo).PrincipalAmount)
	assertTableCount(t, db, "loans", 1)
}
