package main

import (
	"context"
	"flag"

	"LoanService/cmd/server/wire"
	"LoanService/config"
	applog "LoanService/pkg/log"
)

func main() {
	path := flag.String("conf", config.DefaultPath, "configuration YAML path")
	flag.Parse()

	conf, err := config.NewConfig(*path)
	if err != nil {
		panic(err)
	}

	if err := conf.ValidateAuth(); err != nil {
		panic(err)
	}

	logger, err := applog.NewLog(conf.LogLevel)
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	app, cleanup, err := wire.NewWire(context.Background(), conf, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(context.Background()); err != nil {
		panic(err)
	}
}
