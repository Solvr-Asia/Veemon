package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/utils/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("fiber-middleware")

func TracingMiddleware(serviceName string) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Extract trace context from incoming request headers.
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(c.Context(), propagation.HeaderCarrier(c.GetReqHeaders()))

		// Fiber/fasthttp return zero-copy strings backed by the request buffer,
		// which is reused after the handler returns. Spans are exported
		// asynchronously (batched), so every string stored on a span MUST be
		// copied or it may be read after the buffer is recycled — a data race.
		method := utils.CopyString(c.Method())
		routePath := utils.CopyString(c.Route().Path)
		if routePath == "" {
			routePath = utils.CopyString(c.Path())
		}
		originalURL := utils.CopyString(c.OriginalURL())
		hostname := utils.CopyString(c.Hostname())
		userAgent := utils.CopyString(c.Get("User-Agent"))
		clientIP := utils.CopyString(c.IP())

		// Use the low-cardinality route pattern (e.g. /api/v1/users/:id) as the
		// span name, not the raw path with IDs, to avoid metric/label explosion.
		ctx, span := tracer.Start(ctx, method+" "+routePath,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethodKey.String(method),
				semconv.HTTPURLKey.String(originalURL),
				semconv.HTTPRouteKey.String(routePath),
				semconv.NetHostNameKey.String(hostname),
				semconv.UserAgentOriginalKey.String(userAgent),
				attribute.String("http.client_ip", clientIP),
			),
		)
		defer span.End()

		// Correlate the request ID (set by RequestIDMiddleware, which runs first)
		// with the trace.
		if reqID, ok := c.Locals("request_id").(string); ok && reqID != "" {
			span.SetAttributes(attribute.String("request.id", reqID))
		}

		c.SetContext(ctx)

		if span.SpanContext().IsValid() {
			c.Set("X-Trace-ID", span.SpanContext().TraceID().String())
		}

		err := c.Next()

		statusCode := c.Response().StatusCode()
		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(statusCode))

		// Record standard span error status so tracing backends surface failures.
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if statusCode >= 500 {
			span.SetStatus(codes.Error, "server error")
		}

		return err
	}
}
