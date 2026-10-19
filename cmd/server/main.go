package main

import (
	"LoanService/internal/repository"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"LoanService/config"
	"LoanService/internal/handler"
	"LoanService/internal/middleware"
	"LoanService/internal/server"
	"LoanService/internal/service"
	applog "LoanService/pkg/log"
	"LoanService/pkg/tracer"

	"go.uber.org/zap"
)

func main() {
	path := flag.String("conf", "", "configuration YAML path (APP_CONF takes precedence)")
	flag.Parse()
	if err := run(*path); err != nil {
		applog.Error(context.Background(), "application startup failed", err)
		os.Exit(1)
	}
}

func run(path string) error {
	if err := config.LoadLocalEnv(); err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return err
	}
	conf, err := config.NewConfig(path)
	if err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return err
	}
	if err := conf.ValidateAuth(); err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return err
	}
	logger, err := applog.NewLog(conf.LogLevel)
	if err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return err
	}
	defer func() { _ = logger.Sync() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownTracing, err := tracer.InitTracer(ctx, conf.Trace.Endpoint, conf.Trace.Name)
	if err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return fmt.Errorf("initialize tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			applog.Error(shutdownCtx, "flush tracing failed", err)
		}
	}()
	db, err := repository.NewDB(ctx, conf)
	if err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		applog.Error(context.Background(), "main.run failed", err)
		return err
	}
	defer sqlDB.Close()
	repo := repository.NewRepository(db)
	svc := service.NewService(repo, logger)
	baseHandler := handler.NewHandler(logger)
	loanHandler := handler.NewLoanHandler(baseHandler, svc)
	investmentHandler := handler.NewInvestmentHandler(baseHandler, svc)
	httpServer := server.NewHTTPServer(conf, logger, loanHandler, investmentHandler, middleware.Auth(conf.Auth), sqlDB.PingContext)
	done := make(chan error, 1)
	go func() {
		logger.Info("server listening", zap.String("request_id", applog.ProcessID()), zap.String("address", conf.HTTPAddr))
		done <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
