package middleware

import (
	"runtime/debug"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

func RecoveryMiddleware(logger *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered",
					zap.Any("panic", r),
					zap.String("stack", string(debug.Stack())),
					zap.String("path", c.Path()),
					zap.String("method", c.Method()),
				)

				_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"success": false,
					"error": fiber.Map{
						"code":    500,
						"message": "Internal server error",
					},
				})
			}
		}()

		return c.Next()
	}
}
