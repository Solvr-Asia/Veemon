package config

import (
	"context"

	"veemon-common/infra/rabbitmq"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// provideOptionalRabbitMQ connects to RabbitMQ but never fails application
// startup: cmd/server treats a broker outage as "messaging disabled" (the
// original main.go logged a warning and kept running with a nil client).
func provideOptionalRabbitMQ(lc fx.Lifecycle, cfg *Config, log *zap.Logger) *rabbitmq.Client {
	client, err := NewRabbitMQ(cfg, log)
	if err != nil {
		log.Warn("Failed to connect to RabbitMQ, messaging disabled", zap.Error(err))
		return nil
	}
	log.Info("RabbitMQ connection established",
		zap.String("host", cfg.RabbitMQHost),
		zap.Int("port", cfg.RabbitMQPort),
	)
	registerRabbitMQClose(lc, client, log)
	return client
}

// provideRequiredRabbitMQ connects to RabbitMQ and fails startup on error:
// cmd/worker's entire job is consuming from it, so a broker it can't reach is
// fatal (the original main.go's log.Fatal). Returning the error here lets fx
// abort Start (and roll back anything already started) instead.
func provideRequiredRabbitMQ(lc fx.Lifecycle, cfg *Config, log *zap.Logger) (*rabbitmq.Client, error) {
	client, err := NewRabbitMQ(cfg, log)
	if err != nil {
		return nil, err
	}
	log.Info("RabbitMQ connection established",
		zap.String("host", cfg.RabbitMQHost),
		zap.Int("port", cfg.RabbitMQPort),
	)
	registerRabbitMQClose(lc, client, log)
	return client, nil
}

func registerRabbitMQClose(lc fx.Lifecycle, client *rabbitmq.Client, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			if err := client.Close(); err != nil {
				log.Warn("failed to close rabbitmq client", zap.Error(err))
			}
			return nil
		},
	})
}
