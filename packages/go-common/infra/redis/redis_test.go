package redis

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationOrDefault(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		def     time.Duration
		want    time.Duration
	}{
		{"positive seconds used as-is", 5, 3 * time.Second, 5 * time.Second},
		{"zero falls back to default", 0, 3 * time.Second, 3 * time.Second},
		{"negative falls back to default", -1, 2 * time.Second, 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, durationOrDefault(tt.seconds, tt.def))
		})
	}
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

func TestNew_UnreachableHostReturnsWrappedError(t *testing.T) {
	host, port := unusedTCPAddr(t)

	_, err := New(Config{
		Host:        host,
		Port:        port,
		DialTimeout: 1,
		MaxIdle:     1,
		MaxActive:   1,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to redis")
}

func TestIsErrNil(t *testing.T) {
	assert.True(t, IsErrNil(ErrNil))
	assert.False(t, IsErrNil(nil))
}
