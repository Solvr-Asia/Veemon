package config

import (
	"context"
	"fmt"

	"veemon/app/usecase/user"
	"veemon/docs"
	"veemon/handler"
	pb_user "veemon/handler/grpc/user"
	"veemon/pkg/authguard"
	"veemon/pkg/errors"
	"veemon/pkg/metrics"
	"veemon/pkg/middleware"
	"veemon/pkg/rabbitmq"
	"veemon/pkg/redis"
	"veemon/pkg/token"
	"veemon/repository/user_repository"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"
)

// BootstrapConfig holds all dependencies for application wiring.
type BootstrapConfig struct {
	DB       *gorm.DB
	App      *fiber.App
	Log      *zap.Logger
	Cfg      *Config
	Redis    *redis.Client
	RabbitMQ *rabbitmq.Client
}

// BootstrapResult holds the wired components ready to be started.
type BootstrapResult struct {
	GRPCServer *grpc.Server
}

// Bootstrap wires repositories, usecases, handlers, and routes.
func Bootstrap(b *BootstrapConfig) (*BootstrapResult, error) {
	// Layers
	userRepo := user_repository.New(b.DB)
	userUC := user.NewUseCase(userRepo)
	tokenService, err := token.NewTokenService(b.Cfg.JWTSecret, b.Cfg.JWTExpiration)
	if err != nil {
		return nil, fmt.Errorf("init token service: %w", err)
	}
	// Login lockout + token revocation, backed by Redis (no-op if Redis is nil).
	guard := authguard.New(b.Redis, b.Cfg.LoginMaxAttempts, b.Cfg.LoginLockoutMinutes)
	userHandler := handler.NewUserHandler(userUC, tokenService, guard, b.Log)

	// Token validator
	tokenValidator := createTokenValidator(tokenService, guard)

	// Observability routes
	registerObservabilityRoutes(b.App, b.Cfg)

	// Health check
	registerHealthChecks(b)

	// HTTP routes (generated from veemon.route options in the .proto).
	pb_user.RegisterUserApiRoutes(b.App, userHandler, tokenValidator)

	// gRPC server. Interceptor order (outermost first): recovery catches panics
	// from everything downstream, then logging, then auth. Tracing is attached
	// via the OTel stats handler.
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			middleware.GRPCRecoveryInterceptor(b.Log),
			middleware.GRPCLoggingInterceptor(b.Log),
			middleware.GRPCAuthInterceptor(tokenValidator, pb_user.UserApiAuthConfig),
		),
	)
	pb_user.RegisterUserApiServer(grpcServer, userHandler)
	// Reflection eases local debugging (grpcurl) but exposes the full service
	// surface; keep it out of production.
	if b.Cfg.Environment != "production" {
		reflection.Register(grpcServer)
	}

	return &BootstrapResult{
		GRPCServer: grpcServer,
	}, nil
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

func createTokenValidator(tokenService *token.TokenService, guard *authguard.Guard) middleware.TokenValidator {
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

func registerHealthChecks(b *BootstrapConfig) {
	b.App.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": b.Cfg.ServiceName,
		})
	})

	b.App.Get("/ready", func(c fiber.Ctx) error {
		checks := make(map[string]string)

		// Check database
		sqlDB, err := b.DB.DB()
		if err != nil || sqlDB.Ping() != nil {
			checks["database"] = "unhealthy"
		} else {
			checks["database"] = "healthy"
		}

		// Check Redis
		if b.Redis != nil {
			conn := b.Redis.Conn()
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
		if b.RabbitMQ != nil {
			if err := b.RabbitMQ.Ping(); err != nil {
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
