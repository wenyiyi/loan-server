package log

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
