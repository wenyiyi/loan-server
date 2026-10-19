package log

import (
	"context"

	"go.uber.org/zap"
)

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
