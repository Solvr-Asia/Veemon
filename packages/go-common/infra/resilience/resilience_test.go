package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorRetriesUntilSuccess(t *testing.T) {
	executor := New[string]("test", Config{
		CBFailureThreshold: 10,
		CBSuccessThreshold: 1,
		CBDelay:            time.Millisecond,
		RetryMaxAttempts:   3,
		RetryDelay:         time.Millisecond,
		RetryMaxDelay:      time.Millisecond,
		Timeout:            time.Second,
	})

	attempts := 0
	result, err := executor.Execute(context.Background(), func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("temporary failure")
		}
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 3, attempts)
}

func TestExecutorFallbackValue(t *testing.T) {
	executor := New[string]("test", Config{
		CBFailureThreshold: 10,
		CBSuccessThreshold: 1,
		CBDelay:            time.Millisecond,
		RetryMaxAttempts:   1,
		RetryDelay:         time.Millisecond,
		RetryMaxDelay:      time.Millisecond,
		Timeout:            time.Second,
	}, WithFallbackValue("fallback"))

	result, err := executor.Execute(context.Background(), func(ctx context.Context) (string, error) {
		return "", errors.New("permanent failure")
	})

	require.NoError(t, err)
	assert.Equal(t, "fallback", result)
}

func TestExecutorExecuteAsync(t *testing.T) {
	executor := New[string]("test", DefaultConfig())

	result := executor.ExecuteAsync(context.Background(), func(ctx context.Context) (string, error) {
		return "async-ok", nil
	})

	value, err := result.Get()
	require.NoError(t, err)
	assert.Equal(t, "async-ok", value)
}
