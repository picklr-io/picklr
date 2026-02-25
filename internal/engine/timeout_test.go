package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithTimeout(t *testing.T) {
	ctx := context.Background()

	// Test default timeout
	ctx2, cancel := WithTimeout(ctx, 0)
	defer cancel()
	deadline, ok := ctx2.Deadline()
	assert.True(t, ok)
	assert.True(t, deadline.After(time.Now()))

	// Test custom timeout
	ctx3, cancel2 := WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	deadline2, ok := ctx3.Deadline()
	assert.True(t, ok)
	assert.True(t, deadline2.Before(time.Now().Add(10*time.Second)))
}

func TestRetryWithBackoff_Success(t *testing.T) {
	attempts := 0
	err := RetryWithBackoff(context.Background(), &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}, func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("throttled")
		}
		return nil
	}, func(err error) bool {
		return true
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestRetryWithBackoff_NonRetryable(t *testing.T) {
	attempts := 0
	err := RetryWithBackoff(context.Background(), &RetryPolicy{
		MaxRetries: 5,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}, func() error {
		attempts++
		return fmt.Errorf("permanent error")
	}, func(err error) bool {
		return false // Don't retry
	})

	assert.Error(t, err)
	assert.Equal(t, 1, attempts) // Only tried once
}

func TestRetryWithBackoff_MaxRetries(t *testing.T) {
	attempts := 0
	err := RetryWithBackoff(context.Background(), &RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}, func() error {
		attempts++
		return fmt.Errorf("always fails")
	}, func(err error) bool {
		return true
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max retries")
	assert.Equal(t, 3, attempts) // 1 initial + 2 retries
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{fmt.Errorf("throttling"), true},
		{fmt.Errorf("Rate exceeded"), true},
		{fmt.Errorf("Too Many Requests"), true},
		{fmt.Errorf("Service Unavailable"), true},
		{fmt.Errorf("connection reset by peer"), true},
		{fmt.Errorf("i/o timeout"), true},
		{fmt.Errorf("resource not found"), false},
		{fmt.Errorf("access denied"), false},
	}

	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsTransientError(tt.err))
		})
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := RetryWithBackoff(ctx, &RetryPolicy{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}, func() error {
		return fmt.Errorf("would retry")
	}, func(err error) bool {
		return true
	})

	assert.Error(t, err)
}
