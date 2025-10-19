package webhook

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate       float64 // tokens per second
	capacity   int     // bucket capacity
	tokens     float64 // current tokens
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
// requestsPerMinute: maximum requests allowed per minute
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rate := float64(requestsPerMinute) / 60.0 // convert to per second
	capacity := requestsPerMinute

	return &RateLimiter{
		rate:       rate,
		capacity:   capacity,
		tokens:     float64(capacity), // start with full bucket
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed under the rate limit
// Returns true if allowed, false if rate limit exceeded
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.rate

	// Cap at capacity
	if rl.tokens > float64(rl.capacity) {
		rl.tokens = float64(rl.capacity)
	}

	rl.lastRefill = now

	// Try to consume a token
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}

	return false
}

// Wait blocks until a request is allowed (with context support)
// Returns error if context is cancelled
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}

		// Calculate time to wait until next token
		waitTime := rl.timeUntilNextToken()

		select {
		case <-time.After(waitTime):
			// Try again
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// timeUntilNextToken calculates how long to wait for next token
func (rl *RateLimiter) timeUntilNextToken() time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// If we have tokens, no need to wait
	if rl.tokens >= 1.0 {
		return 0
	}

	// Calculate tokens needed
	tokensNeeded := 1.0 - rl.tokens
	secondsToWait := tokensNeeded / rl.rate

	return time.Duration(secondsToWait * float64(time.Second))
}

// GetStats returns current rate limiter stats
func (rl *RateLimiter) GetStats() (tokens float64, capacity int, rate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.tokens, rl.capacity, rl.rate
}
