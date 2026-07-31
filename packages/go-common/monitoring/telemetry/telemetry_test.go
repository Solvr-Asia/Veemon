package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestNew_NoOpPathWhenDisabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"disabled entirely", Config{ServiceName: "svc", Enabled: false}},
		{"enabled but exporter type noop", Config{ServiceName: "svc", Enabled: true, ExporterType: "noop"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tel, err := New(context.Background(), tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, tel)

			// The no-op path never installs an SDK provider, so Shutdown is a
			// harmless no-op rather than an attempt to flush/close anything.
			assert.NoError(t, tel.Shutdown(context.Background()))

			// StartSpan/Tracer still work (backed by the global no-op tracer),
			// they just record nothing.
			ctx, span := tel.StartSpan(context.Background(), "op")
			require.NotNil(t, span)
			span.End()
			assert.NotNil(t, ctx)
		})
	}
}

func TestNew_StdoutExporterInstallsRealProvider(t *testing.T) {
	tel, err := New(context.Background(), Config{
		ServiceName:  "svc",
		Environment:  "test",
		ExporterType: "stdout",
		Enabled:      true,
		SampleRatio:  1.0,
	})
	require.NoError(t, err)
	require.NotNil(t, tel)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assert.NoError(t, tel.Shutdown(ctx))
}

func TestGetTraceIDAndSpanID(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	t.Run("with valid span context", func(t *testing.T) {
		ctx := trace.ContextWithSpanContext(context.Background(), sc)
		assert.Equal(t, traceID.String(), GetTraceID(ctx))
		assert.Equal(t, spanID.String(), GetSpanID(ctx))
	})

	t.Run("without span context", func(t *testing.T) {
		assert.Empty(t, GetTraceID(context.Background()))
		assert.Empty(t, GetSpanID(context.Background()))
	})
}
