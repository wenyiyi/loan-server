package main

import (
	"LoanService/internal/repository"
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"LoanService/config"
	"LoanService/internal/notification"
	applog "LoanService/pkg/log"
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
	if err := conf.ValidateSMTP(); err != nil {
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
	sender := notification.NewSMTP(conf.SMTP)
	worker := notification.NewWorker(db, sender, logger)
	worker.Run(ctx)
	return nil
}
