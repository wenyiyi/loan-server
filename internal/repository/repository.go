package repository

import (
	v1 "LoanService/api/v1"
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

// NewDBWithCleanup lets Wire close the pool on initialization failure or exit.
func NewDBWithCleanup(ctx context.Context, conf *config.Config) (*gorm.DB, func(), error) {
	db, err := NewDB(ctx, conf)
	if err != nil {
		applog.Error(ctx, "initialize database provider failed", err)
		return nil, nil, err
	}
	pool, err := db.DB()
	if err != nil {
		applog.Error(ctx, "get database pool failed", err)
		return nil, nil, err
	}
	return db, func() {
		if err := pool.Close(); err != nil {
			applog.Error(ctx, "close database pool failed", err)
		}
	}, nil
}

// checkUpdateResult checks whether the update succeeded.
func checkUpdateResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return v1.ErrNotFound
	}

	return nil
}
