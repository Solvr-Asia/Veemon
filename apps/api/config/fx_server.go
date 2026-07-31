package config

import (
	"context"
	"fmt"
	"net"

	"veemon-common/monitoring/telemetry"
	"veemon/app/usecase/user"
	"veemon/handler"
	"veemon/repository/user_repository"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// ServerModule provides everything specific to cmd/server: config
// validation, the Fiber app, the optional RabbitMQ client, the user-domain
// layers (repository -> usecase -> handler, unchanged), the gRPC server, and
// the route/server startup wiring. Combine with CommonModule.
var ServerModule = fx.Module("server",
	fx.Provide(
		NewFiber,
		provideOptionalRabbitMQ,
		user_repository.New,
		user.NewUseCase,
		provideTokenService,
		provideAuthGuard,
		handler.NewUserHandler,
		provideTokenValidator,
		provideGRPCServer,
	),
	fx.Invoke(
		validateConfig,
		registerHTTPRoutes,
		registerObservabilityAndHealthRoutes,
		runServers,
	),
)

// validateConfig fails fast on insecure/invalid security config (e.g. a
// missing or placeholder JWT_SECRET) before anything starts serving traffic
// — same invariant as the original main.go's explicit cfg.Validate() call,
// just run as an fx.Invoke instead of a panic.
func validateConfig(cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	return nil
}

// runServers replaces the manual goroutine + signal-handling + ordered
// graceful-shutdown block that used to live at the bottom of cmd/server's
// main(): OnStart launches the HTTP and gRPC listeners in background
// goroutines (Listen/Serve block, so they can't run inline in OnStart); if
// either fails, the failure is reported via fx.Shutdowner so fx tears the
// whole app down with a non-zero exit code, exactly like the original
// errChan. OnStop drains HTTP first, then gRPC, bounded by the context fx
// supplies (see fx.StopTimeout in cmd/server/main.go).
//
// The unused *telemetry.Telemetry parameter is deliberate: fx constructors
// are lazy, and nothing else in the graph depends on Telemetry's return value
// (its job — registering the global OTel tracer provider — is a
// constructor-time side effect, consumed later through the global otel API by
// middleware.TracingMiddleware/otelgrpc/gorm's tracing plugin, not through a
// direct reference). Without a consumer, fx would never construct it at all.
func runServers(lc fx.Lifecycle, sd fx.Shutdowner, app *fiber.App, grpcServer *grpc.Server, cfg *Config, log *zap.Logger, _ *telemetry.Telemetry) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
				if err != nil {
					log.Error("gRPC listen error", zap.Error(err))
					_ = sd.Shutdown(fx.ExitCode(1))
					return
				}
				log.Info("gRPC server listening", zap.Int("port", cfg.GRPCPort))
				if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
					log.Error("gRPC serve error", zap.Error(err))
					_ = sd.Shutdown(fx.ExitCode(1))
				}
			}()

			go func() {
				log.Info("HTTP server listening", zap.Int("port", cfg.HTTPPort))
				if err := app.Listen(fmt.Sprintf(":%d", cfg.HTTPPort), NewListenConfig(cfg)); err != nil {
					log.Error("fiber listen error", zap.Error(err))
					_ = sd.Shutdown(fx.ExitCode(1))
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 1. Drain HTTP: stop accepting new connections, let in-flight requests finish.
			if err := app.ShutdownWithContext(ctx); err != nil {
				log.Error("Fiber shutdown error", zap.Error(err))
			}

			// 2. Stop gRPC gracefully, bounded by the shutdown deadline; force-stop on timeout.
			grpcStopped := make(chan struct{})
			go func() {
				grpcServer.GracefulStop()
				close(grpcStopped)
			}()
			select {
			case <-grpcStopped:
			case <-ctx.Done():
				log.Warn("gRPC graceful stop timed out; forcing stop")
				grpcServer.Stop()
			}

			log.Info("Servers stopped gracefully")
			return nil
		},
	})
}
