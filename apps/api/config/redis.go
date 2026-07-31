package config

import (
	"veemon-common/infra/redis"
)

func NewRedis(cfg *Config) (*redis.Client, error) {
	return redis.New(redis.Config{
		Host:         cfg.RedisHost,
		Port:         cfg.RedisPort,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		MaxIdle:      cfg.RedisMaxIdle,
		MaxActive:    cfg.RedisMaxActive,
		IdleTimeout:  cfg.RedisIdleTimeout,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	})
}
