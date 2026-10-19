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

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
)

func NewHTTPServer(
	conf *config.Config,
	logger *zap.Logger,
	loanHandler *handler.LoanHandler,
	investmentHandler *handler.InvestmentHandler,
	authMiddleware gin.HandlerFunc,
	ready func(context.Context) error,
) *http.Server {
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil)
	engine.Use(
		otelgin.Middleware(conf.Trace.Name),
		middleware.TraceMiddleware(logger),
		middleware.RequestLogMiddleware(logger),
		gin.CustomRecovery(func(ctx *gin.Context, _ any) { v1.HandleInternalError(ctx) }),
	)
	engine.GET("/health", health)
	engine.GET("/healthz", health)
	engine.GET("/readyz", func(ctx *gin.Context) {
		if err := ready(ctx.Request.Context()); err != nil {
			applog.Error(context.Background(), "http.NewHTTPServer failed", err)
			v1.HandleError(ctx, v1.ErrNotReady)
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	api := engine.Group("/api/v1", authMiddleware)

	loans := api.Group("/loans")
	{
		// Staff only
		loans.POST("", middleware.RequireStaff(), loanHandler.Create)
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
	}
}

func health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}
