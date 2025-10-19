package webhook

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	Enabled        bool
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// DefaultRetryConfig returns sensible defaults for retry
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
	}
}

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context) error

// Retryer handles retry logic with exponential backoff
type Retryer struct {
	config RetryConfig
	rand   *rand.Rand
}

// NewRetryer creates a new Retryer
func NewRetryer(config RetryConfig) *Retryer {
	return &Retryer{
		config: config,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Do executes the function with retry logic
// Returns error if all retries are exhausted
func (r *Retryer) Do(ctx context.Context, fn RetryableFunc) error {
	if !r.config.Enabled {
		return fn(ctx)
	}

	var lastErr error
	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		// Execute the function
		err := fn(ctx)

		// Success!
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !r.isRetryable(err) {
			return fmt.Errorf("permanent error (non-retryable): %w", err)
		}

		// Last attempt - don't sleep
		if attempt == r.config.MaxAttempts {
			break
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Calculate backoff with jitter
		backoff := r.calculateBackoff(attempt)

		// Sleep before next retry
		select {
		case <-time.After(backoff):
			// Continue to next attempt
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
		}
	}

	return fmt.Errorf("max retry attempts (%d) exhausted: %w", r.config.MaxAttempts, lastErr)
}

// calculateBackoff calculates backoff duration with exponential growth and jitter
func (r *Retryer) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: initialBackoff * (multiplier ^ (attempt - 1))
	backoff := float64(r.config.InitialBackoff) * math.Pow(r.config.Multiplier, float64(attempt-1))

	// Cap at max backoff
	if backoff > float64(r.config.MaxBackoff) {
		backoff = float64(r.config.MaxBackoff)
	}

	// Add jitter: random value between 0 and 25% of backoff
	// This prevents thundering herd problem
	jitter := r.rand.Float64() * backoff * 0.25
	backoff += jitter

	return time.Duration(backoff)
}

// isRetryable determines if an error is retryable
// Permanent errors (4xx except 429) should not be retried
// Temporary errors (5xx, network errors, timeouts) should be retried
func (r *Retryer) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTPError
	if httpErr, ok := err.(*HTTPError); ok {
		// 4xx Client Errors (except 429 Too Many Requests) are permanent
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			return httpErr.StatusCode == 429 // Only 429 is retryable
		}

		// 5xx Server Errors are retryable
		if httpErr.StatusCode >= 500 {
			return true
		}
	}

	// Network errors, timeouts are retryable
	// (context.Canceled is handled separately above)
	return true
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		// Truncate body to 200 chars for error message
		body := e.Body
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		return fmt.Sprintf("HTTP %d: %s - %s", e.StatusCode, e.Status, body)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Status)
}

// NewHTTPError creates a new HTTPError from an HTTP response
func NewHTTPError(resp *http.Response, body string) *HTTPError {
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       body,
	}
}
