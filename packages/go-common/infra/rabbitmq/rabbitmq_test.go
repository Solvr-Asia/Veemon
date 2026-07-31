package rabbitmq

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"doubles under the cap", 1 * time.Second, 2 * time.Second},
		{"doubles again", 2 * time.Second, 4 * time.Second},
		{"caps at max", 20 * time.Second, reconnectMaxBackoff},
		{"stays capped once at max", reconnectMaxBackoff, reconnectMaxBackoff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextBackoff(tt.in))
		})
	}
}

func TestSafeHandle_RecoversFromPanic(t *testing.T) {
	err := safeHandle(context.Background(), func(ctx context.Context, msg amqp.Delivery) error {
		panic("boom")
	}, amqp.Delivery{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler panic")
}

func TestSafeHandle_PropagatesHandlerError(t *testing.T) {
	wantErr := errors.New("handler failed")
	err := safeHandle(context.Background(), func(ctx context.Context, msg amqp.Delivery) error {
		return wantErr
	}, amqp.Delivery{})

	assert.ErrorIs(t, err, wantErr)
}

func TestSleepOrDone(t *testing.T) {
	t.Run("elapses normally", func(t *testing.T) {
		ok := sleepOrDone(context.Background(), make(chan struct{}), time.Millisecond)
		assert.True(t, ok)
	})

	t.Run("ctx canceled short-circuits", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ok := sleepOrDone(ctx, make(chan struct{}), time.Second)
		assert.False(t, ok)
	})

	t.Run("done channel short-circuits", func(t *testing.T) {
		done := make(chan struct{})
		close(done)
		ok := sleepOrDone(context.Background(), done, time.Second)
		assert.False(t, ok)
	})
}

// unusedTCPAddr reserves a free local port, then immediately releases it, to
// guarantee a real "connection refused" from a closed port on Dial.
func unusedTCPAddr(t *testing.T) (string, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().(*net.TCPAddr)
	require.NoError(t, l.Close())
	return addr.IP.String(), addr.Port
}

func TestNew_UnreachableBrokerReturnsWrappedError(t *testing.T) {
	host, port := unusedTCPAddr(t)

	_, err := New(Config{
		Host:     host,
		Port:     port,
		User:     "guest",
		Password: "guest",
		VHost:    "/",
	}, zap.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to RabbitMQ")
}
