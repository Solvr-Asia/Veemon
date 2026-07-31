package resilience

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

// HTTPClient is a resilient HTTP client with circuit breaker and retry
type HTTPClient struct {
	client   *http.Client
	executor *Executor[*http.Response]
	logger   *zap.Logger
}

// HTTPClientConfig holds HTTP client configuration
type HTTPClientConfig struct {
	Timeout          time.Duration
	MaxIdleConns     int
	IdleConnTimeout  time.Duration
	ResilienceConfig Config
}

// DefaultHTTPClientConfig returns default HTTP client configuration
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		Timeout:          30 * time.Second,
		MaxIdleConns:     100,
		IdleConnTimeout:  90 * time.Second,
		ResilienceConfig: DefaultConfig(),
	}
}

// NewHTTPClient creates a new resilient HTTP client
func NewHTTPClient(name string, cfg HTTPClientConfig, logger *zap.Logger) *HTTPClient {
	transport := &http.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		DisableCompression:  false,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: cfg.MaxIdleConns / 2,
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
		// otelhttp creates a client span per request and injects W3C trace
		// context into outgoing headers so downstream services join the trace.
		Transport: otelhttp.NewTransport(transport),
	}

	//nolint:bodyclose // false positive: *http.Response type param, no response created here
	executor := New[*http.Response](
		fmt.Sprintf("http-client-%s", name),
		cfg.ResilienceConfig,
		WithLogger[*http.Response](logger),
	)

	return &HTTPClient{
		client:   client,
		executor: executor,
		logger:   logger,
	}
}

// Do executes an HTTP request with resilience. The request body is buffered so
// that each retry attempt can re-send it — without this, the body is consumed on
// the first attempt and every retry would send an empty body.
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		bodyBytes = b
	}

	return c.executor.Execute(ctx, func(ctx context.Context) (*http.Response, error) {
		// Clone per attempt so headers/context are not mutated across retries,
		// and attach a fresh body reader from the buffered bytes.
		attempt := req.Clone(ctx)
		if bodyBytes != nil {
			attempt.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			attempt.ContentLength = int64(len(bodyBytes))
			attempt.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}

		resp, err := c.client.Do(attempt) //nolint:bodyclose // body is returned to and closed by the caller
		if err != nil {
			return nil, err
		}

		// Treat 5xx as errors for circuit breaker
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
			resp.Body.Close()                //nolint:errcheck // best-effort cleanup
			return nil, fmt.Errorf("server error: %d - %s", resp.StatusCode, string(body))
		}

		return resp, nil
	})
}

// Get performs a GET request with resilience
func (c *HTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Post performs a POST request with resilience
func (c *HTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(ctx, req)
}

// CircuitState returns the circuit breaker state
func (c *HTTPClient) CircuitState() string {
	return c.executor.CircuitState().String()
}

// IsHealthy returns true if the circuit breaker is closed
func (c *HTTPClient) IsHealthy() bool {
	return c.executor.IsCircuitClosed()
}
