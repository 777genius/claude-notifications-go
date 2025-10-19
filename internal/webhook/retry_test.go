package webhook

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRetrySuccess(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return &HTTPError{StatusCode: 503, Body: "Service Unavailable"}
		}
		return nil
	}

	err := retryer.Do(context.Background(), fn)
	if err != nil {
		t.Errorf("Expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestRetryMaxAttemptsExhausted(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		return &HTTPError{StatusCode: 503, Body: "Service Unavailable"}
	}

	err := retryer.Do(context.Background(), fn)
	if err == nil {
		t.Error("Expected error after max retries, got nil")
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
	if !strings.Contains(err.Error(), "max retry attempts") {
		t.Errorf("Expected error about max retry attempts, got: %v", err)
	}
}

func TestRetryPermanentError(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		return &HTTPError{StatusCode: 400, Body: "Bad Request"}
	}

	err := retryer.Do(context.Background(), fn)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for permanent error, got %d", attempts)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    5,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	fn := func(ctx context.Context) error {
		attempts++
		if attempts == 2 {
			cancel() // Cancel after second attempt
		}
		return &HTTPError{StatusCode: 503, Body: "Service Unavailable"}
	}

	err := retryer.Do(ctx, fn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
	if attempts < 2 {
		t.Errorf("Expected at least 2 attempts before cancellation, got %d", attempts)
	}
}

func TestRetryBackoffProgression(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    4,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	attempts := 0
	timings := []time.Time{}

	fn := func(ctx context.Context) error {
		attempts++
		timings = append(timings, time.Now())
		return &HTTPError{StatusCode: 503, Body: "Service Unavailable"}
	}

	start := time.Now()
	_ = retryer.Do(context.Background(), fn)
	elapsed := time.Since(start)

	// Should have made 4 attempts
	if attempts != 4 {
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}

	// Total time should be at least initial + 2*initial + 4*initial = 7*initial
	// But with jitter it could be less, so we check for at least 5*initial
	minExpected := 5 * config.InitialBackoff
	if elapsed < minExpected {
		t.Errorf("Expected at least %v elapsed time, got %v", minExpected, elapsed)
	}
}

func TestRetryDisabled(t *testing.T) {
	config := RetryConfig{
		Enabled:        false,
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		return &HTTPError{StatusCode: 503, Body: "Service Unavailable"}
	}

	err := retryer.Do(context.Background(), fn)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt when retry disabled, got %d", attempts)
	}
}

func TestIsRetryable(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"500 error", &HTTPError{StatusCode: 500, Body: "Internal Server Error"}, true},
		{"502 error", &HTTPError{StatusCode: 502, Body: "Bad Gateway"}, true},
		{"503 error", &HTTPError{StatusCode: 503, Body: "Service Unavailable"}, true},
		{"504 error", &HTTPError{StatusCode: 504, Body: "Gateway Timeout"}, true},
		{"429 error", &HTTPError{StatusCode: 429, Body: "Too Many Requests"}, true},
		{"400 error", &HTTPError{StatusCode: 400, Body: "Bad Request"}, false},
		{"401 error", &HTTPError{StatusCode: 401, Body: "Unauthorized"}, false},
		{"404 error", &HTTPError{StatusCode: 404, Body: "Not Found"}, false},
		{"Network error", errors.New("connection refused"), true},
		{"Context timeout", context.DeadlineExceeded, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retryer.isRetryable(tt.err)
			if result != tt.retryable {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, result, tt.retryable)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	config := RetryConfig{
		Enabled:        true,
		MaxAttempts:    5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}
	retryer := NewRetryer(config)

	// Test backoff increases exponentially
	backoff1 := retryer.calculateBackoff(1)
	backoff2 := retryer.calculateBackoff(2)
	backoff3 := retryer.calculateBackoff(3)

	// Backoff should increase (with some tolerance for jitter)
	if backoff2 < backoff1 {
		t.Errorf("Backoff should increase: backoff2 (%v) < backoff1 (%v)", backoff2, backoff1)
	}
	if backoff3 < backoff2 {
		t.Errorf("Backoff should increase: backoff3 (%v) < backoff2 (%v)", backoff3, backoff2)
	}

	// Backoff should not exceed max
	backoff10 := retryer.calculateBackoff(10)
	// Max backoff + 25% jitter = 1.25s
	if backoff10 > config.MaxBackoff+250*time.Millisecond {
		t.Errorf("Backoff should not exceed max+jitter: got %v", backoff10)
	}
}
