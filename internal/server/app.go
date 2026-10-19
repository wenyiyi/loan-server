package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	applog "LoanService/pkg/log"
	"go.uber.org/zap"
)

// App owns the HTTP server lifecycle; Wire owns dependency cleanup.
type App struct {
	server *http.Server
	logger *zap.Logger
}

func NewApp(server *http.Server, logger *zap.Logger) *App {
	return &App{server: server, logger: logger}
}

func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	if ctx.Err() == nil {
		go func() {
			a.logger.Info("server listening", zap.String("request_id", applog.ProcessID()), zap.String("address", a.server.Addr))
			done <- a.server.ListenAndServe()
		}()
	}
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			applog.Error(ctx, "HTTP server failed", err)
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		applog.Error(shutdownCtx, "HTTP server shutdown failed", err)
		// Stop active connections before Wire closes their database pool.
		if closeErr := a.server.Close(); closeErr != nil {
			applog.Error(shutdownCtx, "HTTP server close failed", closeErr)
		}
		return err
	}
	return nil
}
