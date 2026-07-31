// Package redis wraps a Redigo connection pool with tracing helpers.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("pkg/redis")

type Client struct {
	pool *redis.Pool
}

type Config struct {
	Host         string
	Port         int
	Password     string
	DB           int
	MaxIdle      int
	MaxActive    int
	IdleTimeout  int // seconds
	DialTimeout  int // seconds
	ReadTimeout  int // seconds
	WriteTimeout int // seconds
}

// durationOrDefault converts a seconds value to a Duration, falling back to def
// when seconds is non-positive so a timeout is always applied.
func durationOrDefault(seconds int, def time.Duration) time.Duration {
	if seconds <= 0 {
		return def
	}
	return time.Duration(seconds) * time.Second
}

func New(cfg Config) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Sensible non-zero fallbacks so a hung Redis can never block a caller
	// indefinitely, even if a config value is omitted.
	dialTimeout := durationOrDefault(cfg.DialTimeout, 5*time.Second)
	readTimeout := durationOrDefault(cfg.ReadTimeout, 3*time.Second)
	writeTimeout := durationOrDefault(cfg.WriteTimeout, 3*time.Second)

	pool := &redis.Pool{
		MaxIdle:     cfg.MaxIdle,
		MaxActive:   cfg.MaxActive,
		IdleTimeout: time.Duration(cfg.IdleTimeout) * time.Second,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", addr,
				redis.DialConnectTimeout(dialTimeout),
				redis.DialReadTimeout(readTimeout),
				redis.DialWriteTimeout(writeTimeout),
			)
			if err != nil {
				return nil, err
			}
			if cfg.Password != "" {
				if _, err := c.Do("AUTH", cfg.Password); err != nil {
					c.Close() //nolint:errcheck // best-effort cleanup on auth failure
					return nil, err
				}
			}
			if cfg.DB != 0 {
				if _, err := c.Do("SELECT", cfg.DB); err != nil {
					c.Close() //nolint:errcheck // best-effort cleanup on select failure
					return nil, err
				}
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}

	// Test connection
	conn := pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	if _, err := conn.Do("PING"); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{pool: pool}, nil
}

func (c *Client) Close() error {
	return c.pool.Close()
}

func (c *Client) Pool() *redis.Pool {
	return c.pool
}

func (c *Client) Conn() redis.Conn {
	return c.pool.Get()
}

// Set stores a value with optional expiration
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	_, span := tracer.Start(ctx, "redis.Set",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := json.Marshal(value)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	if expiration > 0 {
		_, err = conn.Do("SETEX", key, int(expiration.Seconds()), data)
	} else {
		_, err = conn.Do("SET", key, data)
	}

	if err != nil {
		span.RecordError(err)
	}
	return err
}

// Get retrieves a value and unmarshals it into dest
func (c *Client) Get(ctx context.Context, key string, dest interface{}) error {
	_, span := tracer.Start(ctx, "redis.Get",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			return ErrNil
		}
		span.RecordError(err)
		return err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

// GetString retrieves a string value
func (c *Client) GetString(ctx context.Context, key string) (string, error) {
	_, span := tracer.Start(ctx, "redis.GetString",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	val, err := redis.String(conn.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			return "", ErrNil
		}
		span.RecordError(err)
		return "", err
	}
	return val, nil
}

// Delete removes keys
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	_, span := tracer.Start(ctx, "redis.Delete",
		trace.WithAttributes(attribute.StringSlice("redis.keys", keys)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	args := make([]interface{}, len(keys))
	for i, key := range keys {
		args[i] = key
	}

	_, err := conn.Do("DEL", args...)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, span := tracer.Start(ctx, "redis.Exists",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	exists, err := redis.Bool(conn.Do("EXISTS", key))
	if err != nil {
		span.RecordError(err)
		return false, err
	}
	return exists, nil
}

// SetNX sets a value only if the key doesn't exist (for distributed locks)
func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	_, span := tracer.Start(ctx, "redis.SetNX",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := json.Marshal(value)
	if err != nil {
		span.RecordError(err)
		return false, fmt.Errorf("failed to marshal value: %w", err)
	}

	var reply interface{}
	if expiration > 0 {
		reply, err = conn.Do("SET", key, data, "NX", "EX", int(expiration.Seconds()))
	} else {
		reply, err = conn.Do("SETNX", key, data)
	}

	if err != nil {
		span.RecordError(err)
		return false, err
	}

	if reply == nil {
		return false, nil
	}

	if s, ok := reply.(string); ok {
		return s == "OK", nil
	}
	if i, ok := reply.(int64); ok {
		return i == 1, nil
	}
	return false, nil
}

// Incr increments an integer value
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	_, span := tracer.Start(ctx, "redis.Incr",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	val, err := redis.Int64(conn.Do("INCR", key))
	if err != nil {
		span.RecordError(err)
		return 0, err
	}
	return val, nil
}

// Expire sets expiration on a key
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	_, span := tracer.Start(ctx, "redis.Expire",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	_, err := conn.Do("EXPIRE", key, int(expiration.Seconds()))
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// HSet sets a hash field
func (c *Client) HSet(ctx context.Context, key, field string, value interface{}) error {
	_, span := tracer.Start(ctx, "redis.HSet",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.field", field),
		))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := json.Marshal(value)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = conn.Do("HSET", key, field, data)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// HGet gets a hash field
func (c *Client) HGet(ctx context.Context, key, field string, dest interface{}) error {
	_, span := tracer.Start(ctx, "redis.HGet",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.field", field),
		))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := redis.Bytes(conn.Do("HGET", key, field))
	if err != nil {
		if err == redis.ErrNil {
			return ErrNil
		}
		span.RecordError(err)
		return err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

// HGetAll gets all fields in a hash
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	_, span := tracer.Start(ctx, "redis.HGetAll",
		trace.WithAttributes(attribute.String("redis.key", key)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	result, err := redis.StringMap(conn.Do("HGETALL", key))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return result, nil
}

// Publish publishes a message to a channel
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	_, span := tracer.Start(ctx, "redis.Publish",
		trace.WithAttributes(attribute.String("redis.channel", channel)))
	defer span.End()

	conn := c.pool.Get()
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := json.Marshal(message)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	_, err = conn.Do("PUBLISH", channel, data)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// ErrNil is returned when a key doesn't exist
var ErrNil = redis.ErrNil

// IsErrNil checks if error is ErrNil
func IsErrNil(err error) bool {
	return err == redis.ErrNil
}
