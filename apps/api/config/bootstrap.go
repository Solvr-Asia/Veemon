package config

import (
	"context"
	"fmt"

	"veemon-common/infra/rabbitmq"
	"veemon-common/infra/redis"
	"veemon-common/monitoring/metrics"
	"veemon/docs"
	pb_user "veemon/handler/grpc/user"
	"veemon/pkg/authguard"
	"veemon/pkg/errors"
	"veemon/pkg/middleware"
	"veemon/pkg/token"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"
)

// This file holds the user-domain wiring that used to live in the manual
// Bootstrap() function: constructing the token service, the login-attempt
// guard, the token validator, and the gRPC server, plus registering the
// generated HTTP routes and the observability/health endpoints. Each piece is
// now an fx.Provide constructor or an fx.Invoke registration (see
// fx_server.go for the ServerModule that wires them together) — WHO
// constructs and wires these changed, not the layering itself (repository ->
// usecase -> handler is untouched).

// provideTokenService builds the PASETO token service from config.
func provideTokenService(cfg *Config) (*token.TokenService, error) {
	tokenService, err := token.NewTokenService(cfg.JWTSecret, cfg.JWTExpiration)
	if err != nil {
		return nil, fmt.Errorf("init token service: %w", err)
	}
	return tokenService, nil
}

// provideAuthGuard builds the login-lockout + token-revocation guard. A nil
// Redis client (see provideOptionalRedis) degrades it to a no-op, same as before.
func provideAuthGuard(redisClient *redis.Client, cfg *Config) *authguard.Guard {
	return authguard.New(redisClient, cfg.LoginMaxAttempts, cfg.LoginLockoutMinutes)
}

// provideTokenValidator adapts the token service + guard into the closure
// shape pkg/middleware expects for REST and gRPC auth.
func provideTokenValidator(tokenService *token.TokenService, guard *authguard.Guard) middleware.TokenValidator {
	return func(tokenStr string) (*middleware.AuthContext, error) {
		claims, err := tokenService.ValidateToken(tokenStr)
		if err != nil {
			return nil, err
		}

		// Reject tokens that have been revoked (logout / refresh rotation).
		if guard.IsRevoked(context.Background(), claims.TokenID) {
			return nil, token.ErrInvalidToken
		}

		return &middleware.AuthContext{
			UserID:      claims.UserID,
			Email:       claims.Email,
			Roles:       claims.Roles,
			CompanyCode: claims.CompanyCode,
			Token:       tokenStr,
			TokenID:     claims.TokenID,
			ExpiresAt:   claims.ExpiresAt,
		}, nil
	}
}

// provideGRPCServer builds the gRPC server with its interceptor chain and
// registers the UserApi service. Interceptor order (outermost first):
// recovery catches panics from everything downstream, then logging, then
// auth. Tracing is attached via the OTel stats handler.
func provideGRPCServer(log *zap.Logger, tokenValidator middleware.TokenValidator, userHandler pb_user.UserApiServer, cfg *Config) *grpc.Server {
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			middleware.GRPCRecoveryInterceptor(log),
			middleware.GRPCLoggingInterceptor(log),
			middleware.GRPCAuthInterceptor(tokenValidator, pb_user.UserApiAuthConfig),
		),
	)
	pb_user.RegisterUserApiServer(grpcServer, userHandler)

	// Reflection eases local debugging (grpcurl) but exposes the full service
	// surface; keep it out of production.
	if cfg.Environment != "production" {
		reflection.Register(grpcServer)
	}

	return grpcServer
}

// registerHTTPRoutes wires the generated REST routes (from veemon.route
// options in the .proto) onto the Fiber app.
func registerHTTPRoutes(app *fiber.App, userHandler pb_user.UserApiServer, tokenValidator middleware.TokenValidator) {
	pb_user.RegisterUserApiRoutes(app, userHandler, tokenValidator)
}

func registerObservabilityRoutes(app *fiber.App, cfg *Config) {
	m := metrics.Init(cfg.ServiceName)
	app.Use(m.Middleware())
	app.Get("/metrics", metricsAuth(cfg.MetricsAuthToken), m.Handler())
	docs.SetupScalar(app)
}

// metricsAuth optionally guards the /metrics endpoint with a bearer token. When
// no token is configured it is a pass-through (restrict at the network layer).
func metricsAuth(token string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if token == "" {
			return c.Next()
		}
		if c.Get("Authorization") != "Bearer "+token {
			return errors.Unauthorized("unauthorized").FiberError(c)
		}
		return c.Next()
	}
}

// registerHealthChecks wires /health (liveness) and /ready (readiness, which
// actually pings DB/Redis/RabbitMQ rather than just checking non-nil).
func registerHealthChecks(app *fiber.App, cfg *Config, db *gorm.DB, redisClient *redis.Client, rabbitClient *rabbitmq.Client) {
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": cfg.ServiceName,
		})
	})

	app.Get("/ready", func(c fiber.Ctx) error {
		checks := make(map[string]string)

		// Check database
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			checks["database"] = "unhealthy"
		} else {
			checks["database"] = "healthy"
		}

		// Check Redis
		if redisClient != nil {
			conn := redisClient.Conn()
			_, err := conn.Do("PING")
			_ = conn.Close()
			if err != nil {
				checks["redis"] = "unhealthy"
			} else {
				checks["redis"] = "healthy"
			}
		} else {
			checks["redis"] = "disabled"
		}

		// Check RabbitMQ (actually verify the connection, not just non-nil).
		if rabbitClient != nil {
			if err := rabbitClient.Ping(); err != nil {
				checks["rabbitmq"] = "unhealthy"
			} else {
				checks["rabbitmq"] = "healthy"
			}
		} else {
			checks["rabbitmq"] = "disabled"
		}

		allHealthy := true
		for _, v := range checks {
			if v == "unhealthy" {
				allHealthy = false
				break
			}
		}

		status := fiber.StatusOK
		if !allHealthy {
			status = fiber.StatusServiceUnavailable
		}

		return c.Status(status).JSON(fiber.Map{
			"status": checks,
		})
	})
}

// registerObservabilityAndHealthRoutes is the fx.Invoke entry point that
// registers /metrics, the API docs, and the /health + /ready checks.
func registerObservabilityAndHealthRoutes(app *fiber.App, cfg *Config, db *gorm.DB, redisClient *redis.Client, rabbitClient *rabbitmq.Client) {
	registerObservabilityRoutes(app, cfg)
	registerHealthChecks(app, cfg, db, redisClient, rabbitClient)
}
