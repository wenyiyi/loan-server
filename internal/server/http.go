package server

import (
	applog "LoanService/pkg/log"
	"context"
	"net/http"
	"time"

	v1 "LoanService/api/v1"
	"LoanService/config"
	"LoanService/internal/handler"
	"LoanService/internal/middleware"
	"LoanService/pkg/tracer"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewHTTPServer(
	ctx context.Context,
	conf *config.Config,
	logger *zap.Logger,
	loanHandler *handler.LoanHandler,
	investmentHandler *handler.InvestmentHandler,
	db *gorm.DB,
) (*http.Server, func(), error) {
	shutdownTracing, err := tracer.InitTracer(ctx, conf.Trace.Endpoint, conf.Trace.Name)
	if err != nil {
		applog.Error(ctx, "initialize HTTP tracing failed", err)
		return nil, nil, err
	}
	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			applog.Error(shutdownCtx, "flush HTTP tracing failed", err)
		}
	}
	ready := func(ctx context.Context) error {
		pool, err := db.DB()
		if err != nil {
			applog.Error(ctx, "get readiness database pool failed", err)
			return err
		}
		return pool.PingContext(ctx)
	}
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil)
	engine.Use(
		otelgin.Middleware(conf.Trace.Name),
		middleware.TraceMiddleware(logger),
		middleware.RequestLogMiddleware(logger),
		gin.CustomRecovery(func(ctx *gin.Context, _ any) { v1.HandleInternalError(ctx) }),
	)
	engine.GET("/healthz", health)
	engine.GET("/readyz", func(ctx *gin.Context) {
		if err := ready(ctx.Request.Context()); err != nil {
			applog.Error(ctx.Request.Context(), "database readiness check failed", err)
			v1.HandleError(ctx, v1.ErrNotReady)
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	api := engine.Group("/api/v1", middleware.Auth(conf.Auth))

	loans := api.Group("/loans")
	{
		// Staff only
		loans.POST("", middleware.RequireStaff(), loanHandler.Create)
		loans.GET("/:loan_no", middleware.RequireStaff(), loanHandler.Get)
		loans.POST("/:loan_no/approval", middleware.RequireStaff(), loanHandler.Approve)
		loans.POST("/:loan_no/disbursement", middleware.RequireStaff(), loanHandler.Disburse)

		// Investors only
		loans.POST("/:loan_no/investments", middleware.RequireInvestor(), investmentHandler.Invest)
	}
	engine.NoRoute(func(ctx *gin.Context) { v1.HandleError(ctx, v1.ErrNotFound) })
	return &http.Server{
		Addr:              conf.HTTPAddr,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}, cleanup, nil
}

func health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}
