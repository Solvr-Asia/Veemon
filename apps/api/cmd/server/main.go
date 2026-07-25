// Command server runs the HTTP + gRPC API server.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"veemon/config"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func main() {
	// Load configuration
	cfg, err := config.New()
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Fail fast on insecure/invalid security config (e.g. a missing or
	// placeholder JWT_SECRET) before anything starts serving traffic.
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("Invalid configuration: %v", err))
	}

	// Initialize logger
	log, err := config.NewLogger(cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting application",
		zap.String("service", cfg.ServiceName),
		zap.String("environment", cfg.Environment),
		zap.Int("http_port", cfg.HTTPPort),
		zap.Int("grpc_port", cfg.GRPCPort),
		zap.Bool("prefork", cfg.Prefork),
	)

	// Initialize OpenTelemetry
	ctx := context.Background()
	otel, err := config.NewTelemetry(ctx, cfg)
	if err != nil {
		log.Fatal("Failed to initialize telemetry", zap.Error(err))
	}
	defer func() { _ = otel.Shutdown(ctx) }()

	log.Info("OpenTelemetry initialized",
		zap.Bool("enabled", cfg.OTelEnabled),
		zap.String("exporter", cfg.OTelExporterType),
	)

	// Initialize database
	db, err := config.NewDatabase(cfg, log.Logger)
	if err != nil {
		log.Fatal("Failed to connect to database", zap.Error(err))
	}

	// Initialize Redis
	redisClient, err := config.NewRedis(cfg)
	if err != nil {
		log.Warn("Failed to connect to Redis, caching disabled", zap.Error(err))
	} else {
		defer func() { _ = redisClient.Close() }()
		log.Info("Redis connection established",
			zap.String("host", cfg.RedisHost),
			zap.Int("port", cfg.RedisPort),
		)
	}

	// Initialize RabbitMQ
	rabbitClient, err := config.NewRabbitMQ(cfg, log.Logger)
	if err != nil {
		log.Warn("Failed to connect to RabbitMQ, messaging disabled", zap.Error(err))
	} else {
		defer func() { _ = rabbitClient.Close() }()
		log.Info("RabbitMQ connection established",
			zap.String("host", cfg.RabbitMQHost),
			zap.Int("port", cfg.RabbitMQPort),
		)
	}

	// Create Fiber app
	app := config.NewFiber(cfg, log.Logger)

	// Bootstrap application (wire layers, routes, health checks)
	result, err := config.Bootstrap(&config.BootstrapConfig{
		DB:       db,
		App:      app,
		Log:      log.Logger,
		Cfg:      cfg,
		Redis:    redisClient,
		RabbitMQ: rabbitClient,
	})
	if err != nil {
		log.Fatal("Failed to bootstrap application", zap.Error(err))
	}

	// Start servers
	errChan := make(chan error, 2)

	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
		if err != nil {
			errChan <- fmt.Errorf("gRPC listen error: %w", err)
			return
		}
		log.Info("gRPC server listening", zap.Int("port", cfg.GRPCPort))
		if err := result.GRPCServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			errChan <- fmt.Errorf("gRPC serve error: %w", err)
		}
	}()

	go func() {
		log.Info("HTTP server listening", zap.Int("port", cfg.HTTPPort))
		if err := app.Listen(fmt.Sprintf(":%d", cfg.HTTPPort), config.NewListenConfig(cfg)); err != nil {
			errChan <- fmt.Errorf("fiber listen error: %w", err)
		}
	}()

	// Wait for a shutdown signal or a fatal server error. Both paths fall
	// through to the same graceful shutdown so deferred cleanup (telemetry
	// flush, Redis/RabbitMQ close, log sync) always runs — we never os.Exit here.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	exitCode := 0
	select {
	case <-quit:
		log.Info("Shutdown signal received; draining servers...")
	case err := <-errChan:
		log.Error("Server error; shutting down", zap.Error(err))
		exitCode = 1
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Drain HTTP: stop accepting new connections, let in-flight requests finish.
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error("Fiber shutdown error", zap.Error(err))
	}

	// 2. Stop gRPC gracefully, bounded by the shutdown deadline; force-stop on timeout.
	grpcStopped := make(chan struct{})
	go func() {
		result.GRPCServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		log.Warn("gRPC graceful stop timed out; forcing stop")
		result.GRPCServer.Stop()
	}

	// 3. Close the database connection pool.
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}

	log.Info("Servers stopped gracefully")

	// Flush deferred cleanup (see defers above) before exiting non-zero.
	if exitCode != 0 {
		_ = log.Sync()
		_ = otel.Shutdown(context.Background())
		os.Exit(exitCode)
	}
}
