package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func LoggerMiddleware(logger *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		duration := time.Since(start)

		// When a handler returns an error, Fiber's app-level ErrorHandler runs
		// AFTER this middleware, so c.Response().StatusCode() is still the
		// default here. Derive the real status from the error instead.
		status := c.Response().StatusCode()
		if err != nil {
			if fe, ok := err.(*fiber.Error); ok {
				status = fe.Code
			} else if status < 400 {
				status = fiber.StatusInternalServerError
			}
		}

		fields := []zap.Field{
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Duration("duration", duration),
			zap.String("ip", c.IP()),
			zap.String("user_agent", c.Get("User-Agent")),
		}

		if requestID, ok := c.Locals("request_id").(string); ok {
			fields = append(fields, zap.String("request_id", requestID))
		}

		// Add trace context if available
		if span := trace.SpanFromContext(c.Context()); span.SpanContext().IsValid() {
			fields = append(fields,
				zap.String("trace_id", span.SpanContext().TraceID().String()),
				zap.String("span_id", span.SpanContext().SpanID().String()),
			)
		}

		switch {
		case err != nil:
			fields = append(fields, zap.Error(err))
			logger.Error("Request failed", fields...)
		case status >= 500:
			logger.Error("Request completed with server error", fields...)
		case status >= 400:
			logger.Warn("Request completed with client error", fields...)
		default:
			logger.Info("Request completed", fields...)
		}

		return err
	}
}
