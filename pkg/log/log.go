package log

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

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

type contextKey struct{}

// WithContext returns the request logger, falling back to the supplied logger.
func WithContext(ctx context.Context, fallback *zap.Logger) *zap.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*zap.Logger); ok {
		return logger
	}
	return fallback
}

// WithValue adds fields to the context logger for downstream layers.
func WithValue(ctx context.Context, logger *zap.Logger, fields ...zap.Field) context.Context {
	return context.WithValue(ctx, contextKey{}, WithContext(ctx, logger).With(fields...))
}

// The fallback correlates logs emitted before configuration or outside a request.
var processID = strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 16)
var fallbackLogger = zap.New(zapcore.NewCore(
	zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
	zapcore.AddSync(os.Stderr), zap.InfoLevel,
)).With(zap.String("request_id", processID))

func ProcessID() string { return processID }

// Error preserves request tracing and also works during configuration loading.
func Error(ctx context.Context, operation string, err error) {
	WithContext(ctx, fallbackLogger).Error(operation, zap.Error(err))
}
