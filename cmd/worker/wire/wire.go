//go:build wireinject

package wire

import (
	"context"

	"LoanService/config"
	"LoanService/internal/notification"
	"LoanService/internal/repository"

	"github.com/google/wire"
)

var repositorySet = wire.NewSet(repository.NewDBWithCleanup)
var notificationSet = wire.NewSet(wire.FieldsOf(new(*config.Config), "SMTP"), notification.NewSMTP, wire.Bind(new(notification.Sender), new(notification.SMTP)), notification.NewWorker)

func NewWire(ctx context.Context, conf *config.Config) (*notification.Worker, func(), error) {
	panic(wire.Build(repositorySet, notificationSet))
}
