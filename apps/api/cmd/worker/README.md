# Worker Service

This worker service consumes messages from RabbitMQ queues and processes them asynchronously.

## Features

- **Multiple Concurrent Workers**: Configurable number of worker goroutines for parallel message processing
- **Graceful Shutdown**: Handles SIGINT/SIGTERM signals to finish processing current messages before shutting down
- **Quality of Service (QoS)**: Configurable prefetch count to control how many messages each worker prefetches
- **OpenTelemetry Integration**: Built-in distributed tracing for message processing
- **Auto-reconnection**: Handles connection failures and reconnects automatically
- **Message Acknowledgment**: Manual ACK/NACK with retry logic

## Configuration

The worker uses the same configuration as the main server application. Make sure your `.env` file contains:

```env
# RabbitMQ Configuration
RABBITMQ_HOST=localhost
RABBITMQ_PORT=5672
RABBITMQ_USER=guest
RABBITMQ_PASSWORD=guest
RABBITMQ_VHOST=/

# Optional: Database and Redis if worker needs them
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=veemon_db

REDIS_HOST=localhost
REDIS_PORT=6379
```

## Queue Configuration

The worker is configured with the following defaults in `cmd/worker/main.go`:

```go
const (
    DefaultQueue        = "default_queue"
    DefaultExchange     = "default_exchange"
    DefaultExchangeType = "topic"
    DefaultRoutingKey   = "default.#"
    ConsumerTag         = "worker-consumer"
    PrefetchCount       = 10
    ConcurrentWorkers   = 5
)
```

### Customizing Queues

To customize the queues and exchanges, modify the constants in `cmd/worker/main.go`:

1. **Queue Name**: Change `DefaultQueue` to your queue name
2. **Exchange**: Change `DefaultExchange` and `DefaultExchangeType`
3. **Routing Key**: Change `DefaultRoutingKey` to match your routing pattern
4. **Worker Count**: Adjust `ConcurrentWorkers` based on your workload
5. **Prefetch Count**: Adjust `PrefetchCount` to control message batching

## Running the Worker

### Development Mode

```bash
# Run worker directly with Go
make run-worker

# Or using go run
go run ./cmd/worker
```

### Production Mode

```bash
# Build the worker binary
make build-worker

# Run the built binary
./bin/veemon-worker
```

### Running Multiple Workers

You can run multiple worker processes for better scalability:

```bash
# Terminal 1
./bin/veemon-worker

# Terminal 2
./bin/veemon-worker

# Terminal 3
./bin/veemon-worker
```

Each worker process will start its own set of concurrent consumers.

## Message Handler Implementation

The message handler is defined in `handleMessage` function in `cmd/worker/main.go`. 

### Example: Processing Different Message Types

```go
func handleMessage(ctx context.Context, msg amqp.Delivery, log *zap.Logger, db interface{}, redis interface{}) error {
    log.Info("Processing message",
        zap.String("routing_key", msg.RoutingKey),
        zap.String("content_type", msg.ContentType),
    )

    // Parse message body
    var payload map[string]interface{}
    if err := json.Unmarshal(msg.Body, &payload); err != nil {
        return fmt.Errorf("invalid message format: %w", err)
    }

    // Route based on routing key
    switch {
    case strings.HasPrefix(msg.RoutingKey, "email."):
        return handleEmailMessage(ctx, payload, log)
    case strings.HasPrefix(msg.RoutingKey, "notification."):
        return handleNotificationMessage(ctx, payload, log)
    case strings.HasPrefix(msg.RoutingKey, "analytics."):
        return handleAnalyticsMessage(ctx, payload, log, db)
    default:
        log.Warn("Unknown message type", zap.String("routing_key", msg.RoutingKey))
        return nil
    }
}

func handleEmailMessage(ctx context.Context, payload map[string]interface{}, log *zap.Logger) error {
    // Send email logic
    return nil
}

func handleNotificationMessage(ctx context.Context, payload map[string]interface{}, log *zap.Logger) error {
    // Send notification logic
    return nil
}

func handleAnalyticsMessage(ctx context.Context, payload map[string]interface{}, log *zap.Logger, db interface{}) error {
    // Store analytics data
    return nil
}
```

## Publishing Messages

From your main application, publish messages to be consumed by the worker:

```go
import (
    "context"
    "veemon-common/infra/rabbitmq"
)

// In your service or handler
func (s *Service) CreateTask(ctx context.Context, task Task) error {
    // Publish message to RabbitMQ
    err := s.rabbitMQ.Publish(ctx, rabbitmq.PublishOptions{
        Exchange:   "default_exchange",
        RoutingKey: "task.created",
    }, task)
    
    if err != nil {
        return fmt.Errorf("failed to publish message: %w", err)
    }
    
    return nil
}
```

## Monitoring

The worker logs all message processing activities. Monitor the logs to track:

- Worker startup and shutdown
- Message processing success/failures
- Connection status
- Performance metrics (via OpenTelemetry)

Example log output:

```json
{
  "level": "info",
  "msg": "Starting worker",
  "service": "veemon-worker",
  "environment": "production"
}
{
  "level": "info",
  "msg": "Starting consumer",
  "worker_id": 1,
  "consumer_tag": "worker-consumer-1",
  "queue": "default_queue"
}
{
  "level": "info",
  "msg": "Processing message",
  "routing_key": "task.created",
  "content_type": "application/json",
  "body_size": 256
}
```

## Error Handling

The worker implements automatic error handling:

- **Transient Errors**: Messages are NACK'd and requeued for retry
- **Permanent Errors**: Messages are NACK'd without requeue (sent to DLQ if configured)
- **Poison Messages**: Logged and skipped to prevent infinite loops

To configure a Dead Letter Queue (DLQ):

```go
// In setupTopology function
args := amqp.Table{
    "x-dead-letter-exchange":    "dlx_exchange",
    "x-dead-letter-routing-key": "dead_letter",
}

queue, err := client.DeclareQueue(
    DefaultQueue,
    true,  // durable
    false, // auto-delete
    false, // exclusive
    false, // no-wait
    args,  // arguments with DLQ config
)
```

## Graceful Shutdown

The worker handles shutdown signals gracefully:

1. Receives SIGINT/SIGTERM
2. Stops consuming new messages
3. Waits up to 30 seconds for current messages to finish processing
4. Closes all connections
5. Exits cleanly

This ensures no messages are lost during deployment or shutdown.

## Best Practices

1. **Idempotency**: Make message handlers idempotent to handle duplicate messages safely
2. **Timeout**: Set appropriate timeouts for long-running operations
3. **Resource Management**: Close connections and release resources properly
4. **Error Logging**: Log errors with sufficient context for debugging
5. **Monitoring**: Set up alerts for queue depth and processing failures
6. **Scaling**: Adjust `ConcurrentWorkers` and `PrefetchCount` based on your workload
7. **Testing**: Test message handlers with various payload types and error scenarios

## Troubleshooting

### Worker not consuming messages

1. Check RabbitMQ connection: Verify RABBITMQ_* environment variables
2. Check queue exists: Use RabbitMQ management UI to verify queue creation
3. Check queue bindings: Ensure queue is bound to the correct exchange with routing key

### High CPU usage

1. Reduce `ConcurrentWorkers` count
2. Add delays or rate limiting in message handlers
3. Optimize database queries in handlers

### Messages not being acknowledged

1. Check handler return values: Ensure handlers return nil on success
2. Check for panics: Use recovery middleware to catch panics
3. Verify AutoAck setting: Should be `false` for manual acknowledgment

### Memory leaks

1. Close database transactions in handlers
2. Release HTTP client resources
3. Use context cancellation for cleanup
4. Profile with `pprof` to identify leaks
