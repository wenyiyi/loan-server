package repository

import (
	applog "LoanService/pkg/log"
	"context"
	"fmt"
	"time"

	"LoanService/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// DB carries cancellation and deadlines from the current request into GORM.
func (r *Repository) DB(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

func NewDB(ctx context.Context, conf *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(conf.DatabaseURL), &gorm.Config{
		DisableAutomaticPing: true,
		// SQL logging is disabled to avoid exposing borrower and investor data.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		applog.Error(ctx, "repository.NewDB failed", err)
		return nil, fmt.Errorf("database initialization failed")
	}
	sqlDB, err := db.DB()
	if err != nil {
		applog.Error(ctx, "repository.NewDB failed", err)
		return nil, fmt.Errorf("database pool initialization failed")
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(time.Hour)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		applog.Error(ctx, "repository.NewDB failed", err)
		_ = sqlDB.Close()
		return nil, fmt.Errorf("database connection failed")
	}
	return db, nil
}
