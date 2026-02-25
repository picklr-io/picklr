package engine

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// DefaultTimeout is the default per-resource operation timeout.
const DefaultTimeout = 30 * time.Minute

// DefaultRetryMax is the default maximum number of retries for transient errors.
const DefaultRetryMax = 3

// RetryPolicy defines retry behavior for transient cloud API errors.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: DefaultRetryMax,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

// WithTimeout wraps a context with a per-resource timeout.
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// RetryWithBackoff executes fn with exponential backoff and jitter.
// It retries only if shouldRetry returns true for the error.
func RetryWithBackoff(ctx context.Context, policy *RetryPolicy, fn func() error, shouldRetry func(error) bool) error {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}

	var lastErr error
	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !shouldRetry(lastErr) {
			return lastErr
		}

		if attempt < policy.MaxRetries {
			delay := calculateBackoff(attempt, policy.BaseDelay, policy.MaxDelay)
			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", policy.MaxRetries, lastErr)
}

// calculateBackoff returns exponential backoff with jitter.
func calculateBackoff(attempt int, base, max time.Duration) time.Duration {
	backoff := float64(base) * math.Pow(2, float64(attempt))
	if backoff > float64(max) {
		backoff = float64(max)
	}
	// Add jitter: random between 0 and backoff
	jitter := rand.Float64() * backoff
	return time.Duration(jitter)
}

// IsTransientError checks if an error is likely transient and retryable.
// This checks for common cloud API throttling and network errors.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	transientPatterns := []string{
		"throttl",
		"rate exceed",
		"too many requests",
		"request limit",
		"service unavailable",
		"internal server error",
		"connection reset",
		"connection refused",
		"timeout",
		"TLS handshake",
		"i/o timeout",
		"temporary failure",
	}
	for _, pattern := range transientPatterns {
		if containsIgnoreCase(msg, pattern) {
			return true
		}
	}
	return false
}

func containsIgnoreCase(s, substr string) bool {
	sLower := make([]byte, len(s))
	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			sLower[i] = c + 32
		} else {
			sLower[i] = c
		}
	}
	subLower := make([]byte, len(substr))
	for i, c := range []byte(substr) {
		if c >= 'A' && c <= 'Z' {
			subLower[i] = c + 32
		} else {
			subLower[i] = c
		}
	}
	return bytesContains(sLower, subLower)
}

func bytesContains(s, substr []byte) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := range substr {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
