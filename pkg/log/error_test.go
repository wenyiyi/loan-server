package log

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorKeepsRequestCorrelation(t *testing.T) {
	core, entries := observer.New(zap.ErrorLevel)
	ctx := WithValue(context.Background(), zap.New(core), zap.String("request_id", "request-123"), zap.String("trace_id", "trace-456"))
	Error(ctx, "database query failed", errors.New("connection unavailable"))
	require.Len(t, entries.All(), 1)
	fields := entries.All()[0].ContextMap()
	require.Equal(t, "request-123", fields["request_id"])
	require.Equal(t, "trace-456", fields["trace_id"])
	require.Equal(t, "connection unavailable", fields["error"])
}

func TestErrorBeforeRequestHasCorrelation(t *testing.T) {
	core, entries := observer.New(zap.ErrorLevel)
	original := fallbackLogger
	fallbackLogger = zap.New(core).With(zap.String("request_id", ProcessID()))
	t.Cleanup(func() { fallbackLogger = original })
	Error(context.Background(), "configuration failed", errors.New("missing value"))
	require.Len(t, entries.All(), 1)
	require.NotEmpty(t, entries.All()[0].ContextMap()["request_id"])
}
