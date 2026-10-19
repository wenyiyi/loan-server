//go:build wireinject

package wire

import (
	"context"

	"LoanService/config"
	"LoanService/internal/handler"
	"LoanService/internal/repository"
	"LoanService/internal/server"
	"LoanService/internal/service"

	"github.com/google/wire"
	"go.uber.org/zap"
)

var repositorySet = wire.NewSet(repository.NewDBWithCleanup, repository.NewRepository)
var serviceSet = wire.NewSet(service.NewService)
var handlerSet = wire.NewSet(handler.NewHandler, handler.NewLoanHandler, handler.NewInvestmentHandler)
var serverSet = wire.NewSet(server.NewHTTPServer, server.NewApp)

func NewWire(ctx context.Context, conf *config.Config, logger *zap.Logger) (*server.App, func(), error) {
	panic(wire.Build(repositorySet, serviceSet, handlerSet, serverSet))
}
