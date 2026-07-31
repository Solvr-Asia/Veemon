// Package logging provides a structured Zap logger with trace correlation.
package logging

import (
	"context"
	"os"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// globalLogger is stored atomically so concurrent readers (L, package-level
// helpers) never race with the writer in New.
var (
	globalLogger atomic.Pointer[zap.Logger]
	initOnce     sync.Once
)

type Logger struct {
	*zap.Logger
}

type Config struct {
	Level       string
	Format      string // "json" or "console"
	Environment string
	ServiceName string
}

func New(cfg Config) (*Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if cfg.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		level,
	)

	// No AddCallerSkip here: callers use the returned *zap.Logger directly
	// (log.Info(...)), so the caller frame is already correct. The package-level
	// helpers below add the skip themselves.
	logger := zap.New(core,
		zap.AddCaller(),
		zap.Fields(
			zap.String("service", cfg.ServiceName),
			zap.String("environment", cfg.Environment),
		),
	)

	globalLogger.Store(logger)

	return &Logger{Logger: logger}, nil
}

func (l *Logger) WithContext(ctx context.Context) *zap.Logger {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return l.With(
			zap.String("trace_id", spanCtx.TraceID().String()),
			zap.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return l.Logger
}

func (l *Logger) WithRequestID(requestID string) *zap.Logger {
	return l.With(zap.String("request_id", requestID))
}

func (l *Logger) WithFields(fields ...zap.Field) *zap.Logger {
	return l.With(fields...)
}

func (l *Logger) Sync() error {
	return l.Logger.Sync()
}

// L returns the global logger, lazily initializing a production logger the
// first time it is called before New has run. Access is concurrency-safe.
func L() *zap.Logger {
	if l := globalLogger.Load(); l != nil {
		return l
	}
	initOnce.Do(func() {
		if globalLogger.Load() == nil {
			pl, err := zap.NewProduction()
			if err != nil {
				pl = zap.NewNop()
			}
			globalLogger.Store(pl)
		}
	})
	return globalLogger.Load()
}

// skip returns the global logger with one extra caller frame skipped, so the
// package-level helpers below report their caller rather than themselves.
func skip() *zap.Logger {
	return L().WithOptions(zap.AddCallerSkip(1))
}

func Info(msg string, fields ...zap.Field) {
	skip().Info(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	skip().Error(msg, fields...)
}

func Debug(msg string, fields ...zap.Field) {
	skip().Debug(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	skip().Warn(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	skip().Fatal(msg, fields...)
}

func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

func WithContext(ctx context.Context) *zap.Logger {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return L().With(
			zap.String("trace_id", spanCtx.TraceID().String()),
			zap.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return L()
}
