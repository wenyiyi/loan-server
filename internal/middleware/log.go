package middleware

import (
	"LoanService/pkg/log"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLogMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		start := time.Now()

		requestID := ctx.GetHeader("X-Request-ID")
		if requestID == "" {
			var id [16]byte
			_, _ = rand.Read(id[:])
			requestID = hex.EncodeToString(id[:])
		}

		ctx.Set("request_id", requestID)
		ctx.Header("X-Request-ID", requestID)

		requestCtx := log.WithValue(ctx.Request.Context(), logger, zap.String("request_id", requestID))
		ctx.Request = ctx.Request.WithContext(requestCtx)

		ctx.Next()

		log.WithContext(ctx.Request.Context(), logger).Info(
			"http request",
			zap.String("method", ctx.Request.Method),
			zap.String("path", ctx.Request.URL.Path),
			zap.Int("status", ctx.Writer.Status()),
			zap.Duration("duration", time.Since(start)),
			zap.Int("error_code", ctx.GetInt("error_code")),
		)
	}
}
