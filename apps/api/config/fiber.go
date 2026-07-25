package config

import (
	"strings"
	"time"

	"veemon/pkg/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"go.uber.org/zap"
)

func NewFiber(cfg *Config, log *zap.Logger) *fiber.App {
	// Fiber v3 moved Prefork and DisableStartupMessage out of fiber.Config and
	// into fiber.ListenConfig; they are applied by NewListenConfig at Listen time.
	app := fiber.New(fiber.Config{
		AppName:      cfg.ServiceName,
		ErrorHandler: NewErrorHandler(log),
		// Bound slow/idle connections so a stuck client cannot hold a worker
		// indefinitely. Values are conservative defaults; tune per workload.
		ReadTimeout:  time.Duration(cfg.HTTPReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTPWriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.HTTPIdleTimeout) * time.Second,
	})

	// Security response headers (X-Frame-Options, X-Content-Type-Options, etc.)
	app.Use(helmet.New())

	// CORS. Fiber v3 takes these as slices rather than comma-joined strings.
	corsConfig := cors.Config{
		AllowOrigins: splitAndTrim(cfg.CORSOrigins),
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-Trace-ID"},
	}
	// AllowCredentials cannot be used with wildcard origins
	if cfg.CORSOrigins != "*" {
		corsConfig.AllowCredentials = true
	}
	app.Use(cors.New(corsConfig))

	// Middleware (order matters — outermost first). Logger is placed OUTSIDE
	// Recovery so that a panic (recovered below into a 500) is still logged and
	// traced; Recovery wraps the handler so it can turn panics into responses.
	app.Use(middleware.RequestIDMiddleware())
	app.Use(middleware.TracingMiddleware(cfg.ServiceName))
	app.Use(middleware.LoggerMiddleware(log))
	// Global per-IP rate limit as a coarse abuse guard. Stricter, endpoint-
	// specific limits are applied on auth routes during route registration.
	app.Use(middleware.RateLimitMiddleware(middleware.DefaultRateLimitConfig()))
	app.Use(middleware.RecoveryMiddleware(log))
	app.Use(middleware.TimeoutMiddleware(time.Duration(cfg.RequestTimeout) * time.Second))

	return app
}

// NewListenConfig carries the settings Fiber v3 moved out of fiber.Config.
// Prefork stays configurable here, but Config.Validate() rejects PREFORK=true
// because the embedded gRPC server is incompatible with it.
func NewListenConfig(cfg *Config) fiber.ListenConfig {
	return fiber.ListenConfig{
		DisableStartupMessage: true,
		EnablePrefork:         cfg.Prefork,
	}
}

// splitAndTrim turns a comma-separated config value into a slice, dropping
// empty entries so a trailing comma does not become an empty origin.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func NewErrorHandler(log *zap.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		message := "Internal Server Error"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		}

		log.Error("Request error",
			zap.Int("status", code),
			zap.String("message", message),
			zap.Error(err),
			zap.String("path", c.Path()),
		)

		return c.Status(code).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"code":    code,
				"message": message,
			},
		})
	}
}
