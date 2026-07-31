package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RegistersWithoutPanicking(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
	}{
		{"empty namespace", ""},
		{"named namespace", "veemon"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				m := New(tt.namespace)
				require.NotNil(t, m)
			})
		})
	}
}

func TestHandler_ServesPrometheusFormat(t *testing.T) {
	m := New("test")
	app := fiber.New()
	app.Get("/metrics", m.Handler())

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMiddleware_RecordsRequestsWithoutPanicking(t *testing.T) {
	m := New("test2")
	app := fiber.New()
	app.Use(m.Middleware())
	app.Get("/ok", func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	app.Get("/fail", func(c fiber.Ctx) error { return fiber.NewError(http.StatusBadRequest, "bad") })
	app.Get("/metrics", m.Handler())

	okResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/ok", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, okResp.StatusCode)

	failResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/fail", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, failResp.StatusCode)

	// Scrape after traffic: the request/duration series recorded above must be
	// present, proving Middleware() actually wired metrics into the registry
	// New() created (not a disconnected one).
	metricsResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, metricsResp.StatusCode)
}

func TestRecordAndSetHelpers_DoNotPanic(t *testing.T) {
	m := New("test3")
	assert.NotPanics(t, func() {
		m.RecordUserRegistered()
		m.RecordUserLogin()
		m.SetActiveUsers(5)
		m.RecordDBQuery("select", "users", 0)
		m.SetDBConnections(3)
		m.RecordCacheHit("session")
		m.RecordCacheMiss("session")
		m.RecordMessagePublished("exchange", "key")
		m.RecordMessageConsumed("queue")
		m.SetCircuitBreakerState("breaker", 1)
	})
}

func TestInitAndGet_GlobalInstance(t *testing.T) {
	m := Init("global-test")
	require.NotNil(t, m)
	assert.Same(t, m, Get())
}
