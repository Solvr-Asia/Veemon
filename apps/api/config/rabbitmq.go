package config

import (
	"veemon-common/infra/rabbitmq"

	"go.uber.org/zap"
)

func NewRabbitMQ(cfg *Config, log *zap.Logger) (*rabbitmq.Client, error) {
	return rabbitmq.New(rabbitmq.Config{
		Host:     cfg.RabbitMQHost,
		Port:     cfg.RabbitMQPort,
		User:     cfg.RabbitMQUser,
		Password: cfg.RabbitMQPassword,
		VHost:    cfg.RabbitMQVHost,
	}, log)
}
