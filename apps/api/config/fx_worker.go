package config

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"veemon-common/infra/rabbitmq"
	"veemon-common/infra/redis"
	"veemon-common/monitoring/telemetry"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// Queue names - adjust these to match your queues
	workerDefaultQueue = "default_queue"

	// Exchange configuration
	workerDefaultExchange     = "default_exchange"
	workerDefaultExchangeType = "topic"

	// Routing keys
	workerDefaultRoutingKey = "default.#"

	// Consumer configuration
	workerConsumerTag       = "worker-consumer"
	workerPrefetchCount     = 10
	workerConcurrentWorkers = 5

	workerDrainTimeout = 30 * time.Second
)

// WorkerModule provides everything specific to cmd/worker: the required
// (fatal-on-error) RabbitMQ client, topology setup, and the consumer
// lifecycle. Combine with CommonModule.
var WorkerModule = fx.Module("worker",
	fx.Provide(
		provideRequiredRabbitMQ,
	),
	fx.Invoke(
		setupWorkerTopology,
		runConsumers,
	),
)

// setupWorkerTopology declares the exchange, queue, and binding the worker
// consumes from. A failure here is fatal (matches the original main.go's
// log.Fatal): fx.Invoke returning an error aborts startup.
func setupWorkerTopology(client *rabbitmq.Client, log *zap.Logger) error {
	if err := client.DeclareExchange(
		workerDefaultExchange,
		workerDefaultExchangeType,
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // args
	); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}
	log.Info("Exchange declared",
		zap.String("exchange", workerDefaultExchange),
		zap.String("type", workerDefaultExchangeType),
	)

	queue, err := client.DeclareQueue(
		workerDefaultQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}
	log.Info("Queue declared",
		zap.String("queue", queue.Name),
		zap.Int("messages", queue.Messages),
		zap.Int("consumers", queue.Consumers),
	)

	if err := client.BindQueue(
		workerDefaultQueue,
		workerDefaultRoutingKey,
		workerDefaultExchange,
		false, // no-wait
		nil,   // args
	); err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}
	log.Info("Queue bound to exchange",
		zap.String("queue", workerDefaultQueue),
		zap.String("exchange", workerDefaultExchange),
		zap.String("routing_key", workerDefaultRoutingKey),
	)

	return nil
}

// runConsumers replaces the manual "create a cancelable context, start N
// consumers, wait for a signal, cancel + drain" block that used to live at
// the bottom of cmd/worker's main(). The consumer-lifecycle context is
// deliberately separate from the plain context.Context used for one-shot
// infra init (e.g. telemetry) elsewhere in this package: it must stay alive
// for the whole run and be cancelable from OnStop, so it's created and owned
// here rather than provided through the fx graph.
//
// The unused *telemetry.Telemetry parameter forces fx to actually construct
// it: nothing else in the worker graph depends on Telemetry's return value
// (see the identical note on runServers in fx_server.go), so without a
// consumer it would silently never be built and OTel would stay uninitialized.
func runConsumers(lc fx.Lifecycle, rabbitClient *rabbitmq.Client, db *gorm.DB, redisClient *redis.Client, log *zap.Logger, _ *telemetry.Telemetry) {
	ctx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			for i := 0; i < workerConcurrentWorkers; i++ {
				workerID := i + 1
				consumerTag := fmt.Sprintf("%s-%d", workerConsumerTag, workerID)
				log.Info("Starting consumer",
					zap.Int("worker_id", workerID),
					zap.String("consumer_tag", consumerTag),
					zap.String("queue", workerDefaultQueue),
				)

				if err := rabbitClient.ConsumeWithHandler(ctx, rabbitmq.ConsumeOptions{
					Queue:         workerDefaultQueue,
					ConsumerTag:   consumerTag,
					AutoAck:       false,
					PrefetchCount: workerPrefetchCount,
				}, func(handlerCtx context.Context, msg amqp.Delivery) error {
					return handleMessage(handlerCtx, msg, log, db, redisClient)
				}); err != nil {
					return fmt.Errorf("failed to start consumer %d: %w", workerID, err)
				}
			}

			log.Info("All consumers started",
				zap.Int("concurrent_workers", workerConcurrentWorkers),
				zap.Int("prefetch_count", workerPrefetchCount),
			)
			return nil
		},
		OnStop: func(context.Context) error {
			// Stop consumers accepting new work, then wait (bounded) for
			// in-flight messages to finish before the RabbitMQ connection closes.
			cancel()
			drainCtx, drainCancel := context.WithTimeout(context.Background(), workerDrainTimeout)
			defer drainCancel()
			rabbitClient.WaitConsumers(drainCtx)
			if drainCtx.Err() != nil {
				log.Warn("Shutdown timeout reached before all consumers drained")
			}
			log.Info("Worker stopped gracefully")
			return nil
		},
	})
}

// handleMessage processes a single message.
func handleMessage(ctx context.Context, msg amqp.Delivery, log *zap.Logger, db *gorm.DB, redisClient *redis.Client) error {
	log.Info("Processing message",
		zap.String("routing_key", msg.RoutingKey),
		zap.String("content_type", msg.ContentType),
		zap.Int("body_size", len(msg.Body)),
	)

	// Parse message body. Do NOT log the raw body or decoded payload — messages
	// may contain PII or secrets. Log only non-sensitive metadata.
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		log.Error("Failed to unmarshal message",
			zap.Error(err),
			zap.String("routing_key", msg.RoutingKey),
			zap.Int("body_size", len(msg.Body)),
		)
		return fmt.Errorf("invalid message format: %w", err)
	}

	// TODO: Implement your business logic here
	// Example:
	// - Process the message based on routing key
	// - Update database records (db)
	// - Call external APIs
	// - Send notifications
	// - Cache results in Redis (redisClient)

	log.Info("Message processed successfully",
		zap.String("routing_key", msg.RoutingKey),
		zap.Int("fields", len(payload)),
	)

	// Simulate processing time (remove this in production)
	time.Sleep(100 * time.Millisecond)

	return nil
}
