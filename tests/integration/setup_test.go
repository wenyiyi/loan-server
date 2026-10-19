//go:build integration

package integration

import (
	"LoanService/internal/repository"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"LoanService/config"
	"LoanService/internal/handler"
	"LoanService/internal/model"
	"LoanService/internal/server"
	"LoanService/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func setup(t *testing.T) (service.Service, *gorm.DB) {
	t.Helper()
	t.Logf("[Start] %s", t.Name())
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("[End] Failed. Check results and assertions above")
		} else {
			t.Log("[End] All checks passed. Results match")
		}
	})
	conf, err := config.NewConfig("../../config/local.yml")
	require.NoError(t, err)
	dsn := conf.DatabaseURL
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, parsed.Scheme, "database.url must be a PostgreSQL URL")
	require.NotEmpty(t, strings.TrimPrefix(parsed.Path, "/"), "database.url must include a database name")

	admin, err := repository.NewDB(context.Background(), &config.Config{DatabaseURL: dsn})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminDB.Close()) })
	// Confirm the connection targets the configured database before creating a test schema.
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	require.Equal(t, strings.TrimPrefix(parsed.Path, "/"), databaseName, "unexpected database")

	var suffix [12]byte
	_, err = rand.Read(suffix[:])
	require.NoError(t, err)
	schema := "loan_test_" + hex.EncodeToString(suffix[:])
	t.Logf("[Setup] Connected to database=%s, test schema=%s", databaseName, schema)
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		t.Logf("[Cleanup] Removed test schema=%s", schema)
	})
	params := parsed.Query()
	params.Set("search_path", schema)
	parsed.RawQuery = params.Encode()
	db, err := repository.NewDB(context.Background(), &config.Config{DatabaseURL: parsed.String()})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrations)
	sort.Strings(migrations)
	for _, migration := range migrations {
		data, err := os.ReadFile(migration)
		require.NoError(t, err)
		t.Logf("[Setup] Run migration=%s", filepath.Base(migration))
		require.NoError(t, db.Exec(string(data)).Error, fmt.Sprintf("migration %s", filepath.Base(migration)))
	}
	return service.NewService(repository.NewRepository(db), zap.NewNop()), db
}

func setupRouter(t *testing.T, svc service.Service, db *gorm.DB) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	base := handler.NewHandler(logger)
	tokens := map[string]model.Identity{
		"staff-token":    {ID: "staff", Role: "staff"},
		"investor-token": {ID: "investor", Role: "investor", Email: "i@example.com"},
	}
	httpServer, cleanup, err := server.NewHTTPServer(context.Background(), &config.Config{HTTPAddr: ":8080", Trace: config.TraceConfig{Name: "loan-integration"}, Auth: tokens},
		logger, handler.NewLoanHandler(base, svc), handler.NewInvestmentHandler(base, svc), db)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return httpServer.Handler
}
