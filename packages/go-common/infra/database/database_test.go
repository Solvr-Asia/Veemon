package database

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm/logger"
)

func TestNewZapGormLogger_Defaults(t *testing.T) {
	l := newZapGormLogger(zap.NewNop())

	assert.Equal(t, logger.Warn, l.level, "defaults to Warn so successful queries (which may contain sensitive values) are not logged")
	assert.Equal(t, 200*time.Millisecond, l.slowThreshold)
}

func TestZapGormLogger_LogMode_ClonesWithoutMutatingOriginal(t *testing.T) {
	original := newZapGormLogger(zap.NewNop())

	cloned := original.LogMode(logger.Info)

	require.IsType(t, &zapGormLogger{}, cloned)
	assert.Equal(t, logger.Info, cloned.(*zapGormLogger).level)
	assert.Equal(t, logger.Warn, original.level, "LogMode must not mutate the receiver")
}

// unusedTCPAddr reserves a free local port, then immediately releases it, to
// guarantee a real "connection refused" from a closed port on dial.
func unusedTCPAddr(t *testing.T) (string, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().(*net.TCPAddr)
	require.NoError(t, l.Close())
	return addr.IP.String(), addr.Port
}

// GORM pings on Open by default (config.DisableAutomaticPing is false, and we
// don't set it), so New() is NOT lazy: it fails fast against an unreachable
// host rather than silently deferring the failure to the first query.
func TestNew_UnreachableHostReturnsWrappedError(t *testing.T) {
	host, port := unusedTCPAddr(t)

	_, err := New(Config{
		Host: host, Port: port, User: "u", Name: "d", SSLMode: "disable", Timezone: "UTC",
	}, zap.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to database")
}
