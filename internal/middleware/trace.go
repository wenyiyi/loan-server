package middleware

import (
	"LoanService/pkg/log"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TraceMiddleware attaches the HTTP span created by otelgin to request logs.
func TraceMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		sc := trace.SpanContextFromContext(ctx.Request.Context())
		if sc.IsValid() {
			traceID, spanID := sc.TraceID().String(), sc.SpanID().String()
			ctx.Set("trace_id", traceID)
			ctx.Set("span_id", spanID)
			ctx.Header("X-Trace-ID", traceID)
			requestCtx := log.WithValue(ctx.Request.Context(), logger,
				zap.String("trace_id", traceID), zap.String("span_id", spanID))
			ctx.Request = ctx.Request.WithContext(requestCtx)
		}
		ctx.Next()
	}
}
