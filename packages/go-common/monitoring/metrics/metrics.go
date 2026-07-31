// Package metrics defines Prometheus metrics and HTTP instrumentation.
package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all application metrics
type Metrics struct {
	// HTTP metrics
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestsInFlight prometheus.Gauge
	httpResponseSize     *prometheus.HistogramVec

	// Business metrics
	usersRegistered prometheus.Counter
	usersLoggedIn   prometheus.Counter
	activeUsers     prometheus.Gauge

	// Database metrics
	dbQueriesTotal    *prometheus.CounterVec
	dbQueryDuration   *prometheus.HistogramVec
	dbConnectionsOpen prometheus.Gauge

	// Cache metrics
	cacheHitsTotal   *prometheus.CounterVec
	cacheMissesTotal *prometheus.CounterVec

	// Queue metrics
	messagesPublished *prometheus.CounterVec
	messagesConsumed  *prometheus.CounterVec

	// Circuit breaker metrics
	circuitBreakerState *prometheus.GaugeVec

	// Custom registry
	registry *prometheus.Registry
}

// New creates a new Metrics instance
func New(namespace string) *Metrics {
	registry := prometheus.NewRegistry()

	// Register default collectors
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())

	m := &Metrics{
		registry: registry,

		// HTTP metrics
		httpRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),

		httpRequestDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path", "status"},
		),

		httpRequestsInFlight: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Current number of HTTP requests being processed",
			},
		),

		httpResponseSize: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   []float64{100, 1000, 10000, 100000, 1000000},
			},
			[]string{"method", "path"},
		),

		// Business metrics
		usersRegistered: promauto.With(registry).NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "users_registered_total",
				Help:      "Total number of users registered",
			},
		),

		usersLoggedIn: promauto.With(registry).NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "users_logged_in_total",
				Help:      "Total number of user logins",
			},
		),

		activeUsers: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_users",
				Help:      "Current number of active users",
			},
		),

		// Database metrics
		dbQueriesTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "db_queries_total",
				Help:      "Total number of database queries",
			},
			[]string{"operation", "table"},
		),

		dbQueryDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "db_query_duration_seconds",
				Help:      "Database query duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"operation", "table"},
		),

		dbConnectionsOpen: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_connections_open",
				Help:      "Number of open database connections",
			},
		),

		// Cache metrics
		cacheHitsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_hits_total",
				Help:      "Total number of cache hits",
			},
			[]string{"cache"},
		),

		cacheMissesTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_misses_total",
				Help:      "Total number of cache misses",
			},
			[]string{"cache"},
		),

		// Queue metrics
		messagesPublished: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_published_total",
				Help:      "Total number of messages published",
			},
			[]string{"exchange", "routing_key"},
		),

		messagesConsumed: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_consumed_total",
				Help:      "Total number of messages consumed",
			},
			[]string{"queue"},
		),

		// Circuit breaker metrics
		circuitBreakerState: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "circuit_breaker_state",
				Help:      "Circuit breaker state (0=closed, 1=half-open, 2=open)",
			},
			[]string{"name"},
		),
	}

	return m
}

// Handler returns the Prometheus metrics HTTP handler
func (m *Metrics) Handler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
}

// Middleware returns a Fiber middleware that records HTTP metrics
func (m *Metrics) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		m.httpRequestsInFlight.Inc()
		defer m.httpRequestsInFlight.Dec()

		// Process request
		err := c.Next()

		// Derive the true status: on a handler error, Fiber's ErrorHandler runs
		// after this middleware, so c.Response().StatusCode() is still the
		// default here.
		statusCode := c.Response().StatusCode()
		if err != nil {
			if fe, ok := err.(*fiber.Error); ok {
				statusCode = fe.Code
			} else if statusCode < 400 {
				statusCode = fiber.StatusInternalServerError
			}
		}

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(statusCode)
		method := c.Method()
		path := c.Route().Path // route pattern, not raw path — bounds label cardinality
		if path == "" {
			path = "unmatched"
		}

		m.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		m.httpRequestDuration.WithLabelValues(method, path, status).Observe(duration)
		m.httpResponseSize.WithLabelValues(method, path).Observe(float64(len(c.Response().Body())))

		return err
	}
}

// RecordUserRegistered increments user registration counter
func (m *Metrics) RecordUserRegistered() {
	m.usersRegistered.Inc()
}

// RecordUserLogin increments user login counter
func (m *Metrics) RecordUserLogin() {
	m.usersLoggedIn.Inc()
}

// SetActiveUsers sets the number of active users
func (m *Metrics) SetActiveUsers(count float64) {
	m.activeUsers.Set(count)
}

// RecordDBQuery records a database query
func (m *Metrics) RecordDBQuery(operation, table string, duration time.Duration) {
	m.dbQueriesTotal.WithLabelValues(operation, table).Inc()
	m.dbQueryDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// SetDBConnections sets the number of open database connections
func (m *Metrics) SetDBConnections(count float64) {
	m.dbConnectionsOpen.Set(count)
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cache string) {
	m.cacheHitsTotal.WithLabelValues(cache).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cache string) {
	m.cacheMissesTotal.WithLabelValues(cache).Inc()
}

// RecordMessagePublished records a published message
func (m *Metrics) RecordMessagePublished(exchange, routingKey string) {
	m.messagesPublished.WithLabelValues(exchange, routingKey).Inc()
}

// RecordMessageConsumed records a consumed message
func (m *Metrics) RecordMessageConsumed(queue string) {
	m.messagesConsumed.WithLabelValues(queue).Inc()
}

// SetCircuitBreakerState sets the circuit breaker state
// 0 = closed, 1 = half-open, 2 = open
func (m *Metrics) SetCircuitBreakerState(name string, state int) {
	m.circuitBreakerState.WithLabelValues(name).Set(float64(state))
}

// Global metrics instance
var globalMetrics *Metrics

// Init initializes the global metrics instance
func Init(namespace string) *Metrics {
	globalMetrics = New(namespace)
	return globalMetrics
}

// Get returns the global metrics instance
func Get() *Metrics {
	return globalMetrics
}
