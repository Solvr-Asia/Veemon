package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// observerCore returns a zap core backed by an in-memory observer, so tests
// can assert on emitted fields without capturing stdout.
func observerCore() (zapcore.Core, *observer.ObservedLogs) {
	return observer.New(zapcore.DebugLevel)
}

func TestNew_LevelAndFormat(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		format      string
		wantEnabled zapcore.Level // the lowest level the resulting core must accept
		wantBlocked zapcore.Level // a level the resulting core must reject
	}{
		{"debug level enables debug", "debug", "json", zapcore.DebugLevel, zapcore.Level(-2)},
		{"info level blocks debug", "info", "json", zapcore.InfoLevel, zapcore.DebugLevel},
		{"warn level blocks info", "warn", "console", zapcore.WarnLevel, zapcore.InfoLevel},
		{"invalid level falls back to info", "not-a-level", "json", zapcore.InfoLevel, zapcore.DebugLevel},
		{"console format still builds", "error", "console", zapcore.ErrorLevel, zapcore.WarnLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := New(Config{
				Level:       tt.level,
				Format:      tt.format,
				Environment: "test",
				ServiceName: "logging-test",
			})
			require.NoError(t, err)
			require.NotNil(t, l)

			assert.True(t, l.Core().Enabled(tt.wantEnabled), "expected level %v to be enabled", tt.wantEnabled)
			assert.False(t, l.Core().Enabled(tt.wantBlocked), "expected level %v to be blocked", tt.wantBlocked)
		})
	}
}

func TestLogger_WithContext_AddsTraceFields(t *testing.T) {
	core, observed := observerCore()
	l := &Logger{Logger: zap.New(core)}

	// No span in context: no trace correlation fields are added.
	l.WithContext(context.Background()).Info("no span")

	// A valid span context: trace_id/span_id are attached.
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	l.WithContext(ctx).Info("with span")

	entries := observed.All()
	require.Len(t, entries, 2)

	assert.Empty(t, entries[0].Context, "no span in context should add no fields")

	fieldMap := entries[1].ContextMap()
	assert.Equal(t, traceID.String(), fieldMap["trace_id"])
	assert.Equal(t, spanID.String(), fieldMap["span_id"])
}

func TestLogger_WithRequestID_WithFields(t *testing.T) {
	core, observed := observerCore()
	l := &Logger{Logger: zap.New(core)}

	l.WithRequestID("req-123").Info("request scoped")
	l.WithFields(zap.String("k", "v")).Info("field scoped")

	entries := observed.All()
	require.Len(t, entries, 2)
	assert.Equal(t, "req-123", entries[0].ContextMap()["request_id"])
	assert.Equal(t, "v", entries[1].ContextMap()["k"])
}
