// Package rabbitmq provides an auto-reconnecting AMQP client.
package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("pkg/rabbitmq")

// ErrNotConnected is returned by operations attempted while the client has no
// live connection (e.g. during a reconnect window).
var ErrNotConnected = errors.New("rabbitmq: not connected")

const (
	reconnectMinBackoff = 1 * time.Second
	reconnectMaxBackoff = 30 * time.Second
)

// Client is an auto-reconnecting RabbitMQ client. A dropped connection is
// re-established in the background; publishers see a transient error and
// consumers automatically re-attach once connectivity returns.
type Client struct {
	config Config
	logger *zap.Logger
	url    string

	mu      sync.RWMutex
	conn    *amqp.Connection
	channel *amqp.Channel // dedicated to publishing

	consumerWG sync.WaitGroup
	done       chan struct{}
	closeOnce  sync.Once
}

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	VHost    string
}

type PublishOptions struct {
	Exchange    string
	RoutingKey  string
	Mandatory   bool
	Immediate   bool
	ContentType string
	Headers     map[string]interface{}
}

type ConsumeOptions struct {
	Queue       string
	ConsumerTag string
	AutoAck     bool
	Exclusive   bool
	NoLocal     bool
	NoWait      bool
	Args        amqp.Table
	// PrefetchCount sets QoS on the consumer's dedicated channel (0 = unlimited).
	PrefetchCount int
}

type Message struct {
	Body    []byte
	Headers map[string]interface{}
}

func New(cfg Config, logger *zap.Logger) (*Client, error) {
	url := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.VHost)

	client := &Client{
		config: cfg,
		logger: logger,
		url:    url,
		done:   make(chan struct{}),
	}

	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Supervise the connection and reconnect on unexpected close.
	go client.supervise()

	return client, nil
}

// connect (re)establishes the connection and the publish channel.
func (c *Client) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.channel = ch
	c.mu.Unlock()
	return nil
}

// supervise blocks on the current connection's close notification and, unless
// the client was intentionally closed, reconnects with capped exponential
// backoff.
func (c *Client) supervise() {
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}

		closeErr := <-conn.NotifyClose(make(chan *amqp.Error, 1))

		select {
		case <-c.done:
			return // intentional shutdown
		default:
		}

		c.logger.Warn("rabbitmq connection lost; reconnecting", zap.Error(closeErr))

		backoff := reconnectMinBackoff
		for {
			select {
			case <-c.done:
				return
			case <-time.After(backoff):
			}
			if err := c.connect(); err != nil {
				c.logger.Error("rabbitmq reconnect failed; will retry", zap.Error(err), zap.Duration("backoff", backoff))
				backoff = nextBackoff(backoff)
				continue
			}
			c.logger.Info("rabbitmq reconnected")
			break
		}
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > reconnectMaxBackoff {
		return reconnectMaxBackoff
	}
	return d
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() { close(c.done) })

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// currentChannel returns the live publish channel, or an error if disconnected.
func (c *Client) currentChannel() (*amqp.Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.channel == nil || c.conn == nil || c.conn.IsClosed() {
		return nil, ErrNotConnected
	}
	return c.channel, nil
}

// Channel returns the current publish channel (may be nil during a reconnect).
func (c *Client) Channel() *amqp.Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channel
}

// Ping reports whether the client currently has a live connection and channel.
// It returns ErrNotConnected during a reconnect window, making it suitable for
// readiness checks.
func (c *Client) Ping() error {
	_, err := c.currentChannel()
	return err
}

// DeclareExchange declares an exchange on the publish channel.
func (c *Client) DeclareExchange(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	ch, err := c.currentChannel()
	if err != nil {
		return err
	}
	return ch.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
}

// DeclareQueue declares a queue on the publish channel.
func (c *Client) DeclareQueue(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	ch, err := c.currentChannel()
	if err != nil {
		return amqp.Queue{}, err
	}
	return ch.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

// BindQueue binds a queue to an exchange on the publish channel.
func (c *Client) BindQueue(queueName, routingKey, exchangeName string, noWait bool, args amqp.Table) error {
	ch, err := c.currentChannel()
	if err != nil {
		return err
	}
	return ch.QueueBind(queueName, routingKey, exchangeName, noWait, args)
}

// SetQoS sets QoS on the publish channel. Consumers set their own QoS via
// ConsumeOptions.PrefetchCount on their dedicated channels.
func (c *Client) SetQoS(prefetchCount, prefetchSize int, global bool) error {
	ch, err := c.currentChannel()
	if err != nil {
		return err
	}
	return ch.Qos(prefetchCount, prefetchSize, global)
}

// Publish publishes a message with tracing. On a transient channel error it
// retries once after a short delay to ride out a reconnect.
func (c *Client) Publish(ctx context.Context, opts PublishOptions, message interface{}) error {
	ctx, span := tracer.Start(ctx, "rabbitmq.Publish",
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", opts.Exchange),
			attribute.String("messaging.rabbitmq.routing_key", opts.RoutingKey),
		))
	defer span.End()

	body, err := json.Marshal(message)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	headers := make(amqp.Table)
	for k, v := range opts.Headers {
		headers[k] = v
	}

	// Inject trace context into headers
	carrier := make(propagation.MapCarrier)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		headers[k] = v
	}

	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/json"
	}

	publishing := amqp.Publishing{
		ContentType:  contentType,
		Body:         body,
		Headers:      headers,
		Timestamp:    time.Now(),
		DeliveryMode: amqp.Persistent,
	}

	err = c.publishOnce(ctx, opts, publishing)
	if err != nil {
		// Give the supervisor a moment to reconnect, then retry once.
		time.Sleep(reconnectMinBackoff)
		if retryErr := c.publishOnce(ctx, opts, publishing); retryErr != nil {
			span.RecordError(retryErr)
			return retryErr
		}
	}
	return nil
}

func (c *Client) publishOnce(ctx context.Context, opts PublishOptions, publishing amqp.Publishing) error {
	ch, err := c.currentChannel()
	if err != nil {
		return err
	}
	return ch.PublishWithContext(ctx, opts.Exchange, opts.RoutingKey, opts.Mandatory, opts.Immediate, publishing)
}

// ConsumeWithHandler starts a self-healing consumer on its own channel. The
// consumer survives channel/connection loss (re-attaching automatically) and
// stops when ctx is canceled or the client is closed.
func (c *Client) ConsumeWithHandler(ctx context.Context, opts ConsumeOptions, handler func(ctx context.Context, msg amqp.Delivery) error) error {
	c.consumerWG.Add(1)
	go func() {
		defer c.consumerWG.Done()
		c.consumeLoop(ctx, opts, handler)
	}()
	return nil
}

// WaitConsumers blocks until all consumer loops have exited (their contexts
// canceled and in-flight messages finished) or ctx expires. Use it during
// graceful shutdown before Close so in-flight messages are not cut off.
func (c *Client) WaitConsumers(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		c.consumerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (c *Client) consumeLoop(ctx context.Context, opts ConsumeOptions, handler func(ctx context.Context, msg amqp.Delivery) error) {
	backoff := reconnectMinBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-c.done:
			return
		default:
		}

		ch, deliveries, err := c.startConsumer(opts)
		if err != nil {
			c.logger.Warn("consumer attach failed; retrying",
				zap.String("queue", opts.Queue), zap.Error(err), zap.Duration("backoff", backoff))
			if !sleepOrDone(ctx, c.done, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		c.logger.Info("consumer attached", zap.String("queue", opts.Queue), zap.String("consumer_tag", opts.ConsumerTag))
		backoff = reconnectMinBackoff
		c.runConsumer(ctx, opts, handler, deliveries)
		_ = ch.Close()

		if ctx.Err() != nil {
			return
		}
		// Delivery channel closed (connection/channel drop) — loop re-attaches.
	}
}

// startConsumer opens a dedicated channel from the current connection and
// begins consuming.
func (c *Client) startConsumer(opts ConsumeOptions) (*amqp.Channel, <-chan amqp.Delivery, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil || conn.IsClosed() {
		return nil, nil, ErrNotConnected
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}
	if opts.PrefetchCount > 0 {
		if err := ch.Qos(opts.PrefetchCount, 0, false); err != nil {
			_ = ch.Close()
			return nil, nil, err
		}
	}

	deliveries, err := ch.Consume(
		opts.Queue, opts.ConsumerTag, opts.AutoAck,
		opts.Exclusive, opts.NoLocal, opts.NoWait, opts.Args,
	)
	if err != nil {
		_ = ch.Close()
		return nil, nil, err
	}
	return ch, deliveries, nil
}

func (c *Client) runConsumer(ctx context.Context, opts ConsumeOptions, handler func(ctx context.Context, msg amqp.Delivery) error, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case msg, ok := <-deliveries:
			if !ok {
				return // channel closed; caller re-attaches
			}
			c.handleDelivery(ctx, opts, handler, msg)
		}
	}
}

func (c *Client) handleDelivery(ctx context.Context, opts ConsumeOptions, handler func(ctx context.Context, msg amqp.Delivery) error, msg amqp.Delivery) {
	// Extract trace context from headers.
	carrier := make(propagation.MapCarrier)
	for k, v := range msg.Headers {
		if s, ok := v.(string); ok {
			carrier[k] = s
		}
	}
	msgCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
	msgCtx, span := tracer.Start(msgCtx, "rabbitmq.Consume",
		trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", opts.Queue)))
	defer span.End()

	err := safeHandle(msgCtx, handler, msg)
	if err != nil {
		span.RecordError(err)
		c.logger.Error("failed to process message", zap.Error(err), zap.String("queue", opts.Queue))
		if opts.AutoAck {
			return
		}
		// Poison-message guard: a message that already failed once (Redelivered)
		// is dropped (requeue=false) instead of being requeued forever. With a
		// dead-letter exchange configured on the queue it will be routed there;
		// otherwise it is discarded. This bounds retries without a DLX.
		requeue := !msg.Redelivered
		if nackErr := msg.Nack(false, requeue); nackErr != nil {
			c.logger.Error("failed to nack message", zap.Error(nackErr), zap.String("queue", opts.Queue))
		}
		return
	}

	if !opts.AutoAck {
		if ackErr := msg.Ack(false); ackErr != nil {
			c.logger.Error("failed to ack message", zap.Error(ackErr), zap.String("queue", opts.Queue))
		}
	}
}

// safeHandle runs the handler with panic recovery so one bad message cannot
// crash the worker.
func safeHandle(ctx context.Context, handler func(ctx context.Context, msg amqp.Delivery) error, msg amqp.Delivery) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return handler(ctx, msg)
}

// sleepOrDone waits for d, returning false if ctx or done fires first.
func sleepOrDone(ctx context.Context, done <-chan struct{}, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return false
	case <-time.After(d):
		return true
	}
}

// PublishJSON is a convenience method for publishing JSON messages.
func (c *Client) PublishJSON(ctx context.Context, exchange, routingKey string, message interface{}) error {
	return c.Publish(ctx, PublishOptions{
		Exchange:    exchange,
		RoutingKey:  routingKey,
		ContentType: "application/json",
	}, message)
}
