package log

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLog constructs the shared structured logger from application configuration.
func NewLog(logLevel string) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		Error(context.Background(), "parse log level failed", err)
		return nil, fmt.Errorf("invalid log.level: %w", err)
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	return cfg.Build()
}
