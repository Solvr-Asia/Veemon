package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

// RateLimitConfig holds rate limiter configuration
type RateLimitConfig struct {
	// Max number of requests per duration
	Max int
	// Duration for rate limiting window
	Duration time.Duration
	// Key generator function (default: IP-based)
	KeyGenerator func(fiber.Ctx) string
	// Skip rate limiting for certain requests
	Skip func(fiber.Ctx) bool
	// Custom response when rate limit exceeded
	LimitReached fiber.Handler
	// Storage for distributed rate limiting (nil = in-memory)
	Storage fiber.Storage
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Max:      100,
		Duration: 1 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},
		Skip: func(c fiber.Ctx) bool {
			// Skip health check endpoints
			path := c.Path()
			return path == "/health" || path == "/ready" || path == "/metrics"
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    429,
					"message": "Too many requests. Please try again later.",
				},
			})
		},
	}
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(cfg RateLimitConfig) fiber.Handler {
	config := limiter.Config{
		Max:        cfg.Max,
		Expiration: cfg.Duration,
		KeyGenerator: func(c fiber.Ctx) string {
			if cfg.KeyGenerator != nil {
				return cfg.KeyGenerator(c)
			}
			return c.IP()
		},
		LimitReached:           cfg.LimitReached,
		SkipFailedRequests:     false,
		SkipSuccessfulRequests: false,
	}

	if cfg.Storage != nil {
		config.Storage = cfg.Storage
	}

	handler := limiter.New(config)

	return func(c fiber.Ctx) error {
		// Check if should skip
		if cfg.Skip != nil && cfg.Skip(c) {
			return c.Next()
		}
		return handler(c)
	}
}

// EndpointRateLimit configures a rate limit for a specific method+path.
type EndpointRateLimit struct {
	Path     string
	Method   string
	Max      int
	Duration time.Duration
}

// RateLimitByEndpointMiddleware creates rate limiting with per-endpoint configuration.
// All limiters are built up-front so the lookup map is read-only while serving
// requests — lazily populating it from the handler would be a concurrent map
// write and could crash the process under load.
func RateLimitByEndpointMiddleware(defaults RateLimitConfig, endpoints []EndpointRateLimit) fiber.Handler {
	limiters := make(map[string]fiber.Handler, len(endpoints))
	for _, ep := range endpoints {
		key := ep.Method + " " + ep.Path
		limiters[key] = limiter.New(limiter.Config{
			Max:        ep.Max,
			Expiration: ep.Duration,
			KeyGenerator: func(c fiber.Ctx) string {
				return c.IP() + ":" + key
			},
			LimitReached: defaults.LimitReached,
		})
	}

	defaultLimiter := limiter.New(limiter.Config{
		Max:          defaults.Max,
		Expiration:   defaults.Duration,
		KeyGenerator: defaults.KeyGenerator,
		LimitReached: defaults.LimitReached,
	})

	return func(c fiber.Ctx) error {
		if defaults.Skip != nil && defaults.Skip(c) {
			return c.Next()
		}

		if h, ok := limiters[c.Method()+" "+c.Path()]; ok {
			return h(c)
		}
		return defaultLimiter(c)
	}
}

// APIKeyRateLimiter creates rate limiting based on API key
func APIKeyRateLimiter(cfg RateLimitConfig, headerName string) fiber.Handler {
	cfg.KeyGenerator = func(c fiber.Ctx) string {
		apiKey := c.Get(headerName)
		if apiKey == "" {
			// Fall back to IP if no API key
			return "ip:" + c.IP()
		}
		return "key:" + apiKey
	}

	return RateLimitMiddleware(cfg)
}

// UserRateLimiter creates rate limiting based on authenticated user
func UserRateLimiter(cfg RateLimitConfig) fiber.Handler {
	cfg.KeyGenerator = func(c fiber.Ctx) string {
		// Try to get user from auth context
		if auth, ok := GetAuthContext(c); ok && auth != nil {
			return "user:" + auth.UserID
		}
		// Fall back to IP
		return "ip:" + c.IP()
	}

	return RateLimitMiddleware(cfg)
}
