// Package resilience provides circuit-breaker, retry, and timeout wrappers.
package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
	"go.uber.org/zap"
)

// Common errors
var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
	ErrTimeout     = errors.New("operation timed out")
	ErrMaxRetries  = errors.New("max retries exceeded")
)

// Config holds resilience configuration
type Config struct {
	// Circuit Breaker
	CBFailureThreshold     uint          `mapstructure:"CB_FAILURE_THRESHOLD"`
	CBSuccessThreshold     uint          `mapstructure:"CB_SUCCESS_THRESHOLD"`
	CBDelay                time.Duration `mapstructure:"CB_DELAY"`
	CBFailureRateThreshold float64       `mapstructure:"CB_FAILURE_RATE_THRESHOLD"`

	// Retry
	RetryMaxAttempts int           `mapstructure:"RETRY_MAX_ATTEMPTS"`
	RetryDelay       time.Duration `mapstructure:"RETRY_DELAY"`
	RetryMaxDelay    time.Duration `mapstructure:"RETRY_MAX_DELAY"`

	// Timeout
	Timeout time.Duration `mapstructure:"TIMEOUT"`
}

// DefaultConfig returns default resilience configuration
func DefaultConfig() Config {
	return Config{
		CBFailureThreshold:     5,
		CBSuccessThreshold:     3,
		CBDelay:                30 * time.Second,
		CBFailureRateThreshold: 0.5,
		RetryMaxAttempts:       3,
		RetryDelay:             100 * time.Millisecond,
		RetryMaxDelay:          2 * time.Second,
		Timeout:                10 * time.Second,
	}
}

// Executor wraps failsafe executor with common policies
type Executor[T any] struct {
	executor       failsafe.Executor[T]
	circuitBreaker circuitbreaker.CircuitBreaker[T]
	retryPolicy    retrypolicy.RetryPolicy[T]
	timeoutPolicy  timeout.Timeout[T]
	fallbackPolicy fallback.Fallback[T]
	logger         *zap.Logger
	name           string
}

// Option is a functional option for Executor
type Option[T any] func(*Executor[T])

// WithLogger sets the logger
func WithLogger[T any](logger *zap.Logger) Option[T] {
	return func(e *Executor[T]) {
		e.logger = logger
	}
}

// WithFallback sets a fallback value
func WithFallback[T any](fallbackFn func(exec failsafe.Execution[T]) (T, error)) Option[T] {
	return func(e *Executor[T]) {
		e.fallbackPolicy = fallback.NewBuilderWithFunc(fallbackFn).Build()
	}
}

// WithFallbackValue sets a static fallback value
func WithFallbackValue[T any](value T) Option[T] {
	return func(e *Executor[T]) {
		e.fallbackPolicy = fallback.NewWithResult(value)
	}
}

// New creates a new resilience executor
func New[T any](name string, cfg Config, opts ...Option[T]) *Executor[T] {
	e := &Executor[T]{
		name:   name,
		logger: zap.NewNop(),
	}

	// Apply options
	for _, opt := range opts {
		opt(e)
	}

	// Build circuit breaker
	e.circuitBreaker = circuitbreaker.NewBuilder[T]().
		WithFailureThreshold(cfg.CBFailureThreshold).
		WithSuccessThreshold(cfg.CBSuccessThreshold).
		WithDelay(cfg.CBDelay).
		OnStateChanged(func(event circuitbreaker.StateChangedEvent) {
			e.logger.Info("circuit breaker state changed",
				zap.String("executor", name),
				zap.String("from", event.OldState.String()),
				zap.String("to", event.NewState.String()),
			)
		}).
		Build()

	// Build retry policy
	e.retryPolicy = retrypolicy.NewBuilder[T]().
		WithMaxAttempts(cfg.RetryMaxAttempts).
		WithBackoff(cfg.RetryDelay, cfg.RetryMaxDelay).
		OnRetry(func(event failsafe.ExecutionEvent[T]) {
			e.logger.Warn("retrying operation",
				zap.String("executor", name),
				zap.Int("attempt", event.Attempts()),
				zap.Error(event.LastError()),
			)
		}).
		Build()

	// Build timeout policy
	e.timeoutPolicy = timeout.NewBuilder[T](cfg.Timeout).
		OnTimeoutExceeded(func(event failsafe.ExecutionDoneEvent[T]) {
			e.logger.Error("operation timed out",
				zap.String("executor", name),
				zap.Duration("timeout", cfg.Timeout),
			)
		}).
		Build()

	// Build executor with policies (order matters: timeout -> retry -> circuit breaker -> fallback)
	policies := []failsafe.Policy[T]{e.timeoutPolicy, e.retryPolicy, e.circuitBreaker}
	if e.fallbackPolicy != nil {
		policies = append([]failsafe.Policy[T]{e.fallbackPolicy}, policies...)
	}

	e.executor = failsafe.With(policies...)

	return e
}

// Execute runs the function with resilience policies
func (e *Executor[T]) Execute(ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	return e.executor.WithContext(ctx).Get(func() (T, error) {
		return fn(ctx)
	})
}

// ExecuteAsync runs the function asynchronously with resilience policies
func (e *Executor[T]) ExecuteAsync(ctx context.Context, fn func(context.Context) (T, error)) failsafe.ExecutionResult[T] {
	return e.executor.WithContext(ctx).GetAsync(func() (T, error) {
		return fn(ctx)
	})
}

// CircuitState returns the current circuit breaker state
func (e *Executor[T]) CircuitState() circuitbreaker.State {
	return e.circuitBreaker.State()
}

// IsCircuitClosed returns true if circuit breaker is closed (healthy)
func (e *Executor[T]) IsCircuitClosed() bool {
	return e.circuitBreaker.IsClosed()
}

// ResetCircuit resets the circuit breaker
func (e *Executor[T]) ResetCircuit() {
	e.circuitBreaker.Close()
}

// Metrics returns circuit breaker metrics
func (e *Executor[T]) Metrics() circuitbreaker.Metrics {
	return e.circuitBreaker.Metrics()
}

// SimpleExecutor provides simplified resilience without generics complexity
type SimpleExecutor struct {
	executor       failsafe.Executor[any]
	circuitBreaker circuitbreaker.CircuitBreaker[any]
	logger         *zap.Logger
	name           string
}

// NewSimple creates a simple executor for any type
func NewSimple(name string, cfg Config, logger *zap.Logger) *SimpleExecutor {
	e := &SimpleExecutor{
		name:   name,
		logger: logger,
	}

	if e.logger == nil {
		e.logger = zap.NewNop()
	}

	// Build circuit breaker
	e.circuitBreaker = circuitbreaker.NewBuilder[any]().
		WithFailureThreshold(cfg.CBFailureThreshold).
		WithSuccessThreshold(cfg.CBSuccessThreshold).
		WithDelay(cfg.CBDelay).
		OnStateChanged(func(event circuitbreaker.StateChangedEvent) {
			e.logger.Info("circuit breaker state changed",
				zap.String("executor", name),
				zap.String("from", event.OldState.String()),
				zap.String("to", event.NewState.String()),
			)
		}).
		Build()

	// Build retry policy
	retryPolicy := retrypolicy.NewBuilder[any]().
		WithMaxAttempts(cfg.RetryMaxAttempts).
		WithBackoff(cfg.RetryDelay, cfg.RetryMaxDelay).
		OnRetry(func(event failsafe.ExecutionEvent[any]) {
			e.logger.Warn("retrying operation",
				zap.String("executor", name),
				zap.Int("attempt", event.Attempts()),
				zap.Error(event.LastError()),
			)
		}).
		Build()

	// Build timeout policy
	timeoutPolicy := timeout.NewBuilder[any](cfg.Timeout).
		OnTimeoutExceeded(func(event failsafe.ExecutionDoneEvent[any]) {
			e.logger.Error("operation timed out",
				zap.String("executor", name),
				zap.Duration("timeout", cfg.Timeout),
			)
		}).
		Build()

	e.executor = failsafe.With[any](timeoutPolicy, retryPolicy, e.circuitBreaker)

	return e
}

// Run executes a function with resilience
func (e *SimpleExecutor) Run(ctx context.Context, fn func(context.Context) error) error {
	_, err := e.executor.WithContext(ctx).Get(func() (any, error) {
		return nil, fn(ctx)
	})
	return err
}

// Get executes a function and returns a value with resilience
func (e *SimpleExecutor) Get(ctx context.Context, fn func(context.Context) (any, error)) (any, error) {
	return e.executor.WithContext(ctx).Get(func() (any, error) {
		return fn(ctx)
	})
}

// State returns current circuit breaker state
func (e *SimpleExecutor) State() string {
	return e.circuitBreaker.State().String()
}

// IsClosed returns true if circuit is closed (healthy)
func (e *SimpleExecutor) IsClosed() bool {
	return e.circuitBreaker.IsClosed()
}
