package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
)

// TimeoutMiddleware attaches a deadline to the request's user context so that
// downstream I/O (database, Redis, outbound calls) that honors context is
// bounded and cannot hang a request indefinitely. A non-positive duration
// disables the timeout.
func TimeoutMiddleware(d time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		if d <= 0 {
			return c.Next()
		}
		ctx, cancel := context.WithTimeout(c.Context(), d)
		defer cancel()
		c.SetContext(ctx)
		return c.Next()
	}
}
