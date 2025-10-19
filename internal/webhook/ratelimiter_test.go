package webhook

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	// 60 requests per minute = 1 per second
	rl := NewRateLimiter(60)

	// Exhaust all tokens first (bucket starts full with 60 tokens)
	for i := 0; i < 100; i++ {
		rl.Allow()
	}

	// Now should be denied (no tokens)
	if rl.Allow() {
		t.Error("Request should be denied when tokens exhausted")
	}

	// Wait for token refill (1 second = 1 token at 60/min rate)
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again (refilled 1 token)
	if !rl.Allow() {
		t.Error("Request after refill should be allowed")
	}

	// Immediate next request should be denied (just used the refilled token)
	if rl.Allow() {
		t.Error("Immediate request after using refilled token should be denied")
	}
}

func TestRateLimiterBurst(t *testing.T) {
	// 120 requests per minute, capacity allows initial burst
	rl := NewRateLimiter(120)

	// Should allow multiple requests initially (bucket is full)
	allowedCount := 0
	for i := 0; i < 200; i++ {
		if rl.Allow() {
			allowedCount++
		}
	}

	// Should have allowed approximately the capacity (120)
	if allowedCount < 100 || allowedCount > 140 {
		t.Errorf("Expected ~120 initial requests, got %d", allowedCount)
	}

	// Next request should be denied
	if rl.Allow() {
		t.Error("Request after exhausting tokens should be denied")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	// 60 requests per minute = 1 per second
	rl := NewRateLimiter(60)

	// Exhaust tokens
	for i := 0; i < 100; i++ {
		rl.Allow()
	}

	// Should be denied
	if rl.Allow() {
		t.Error("Should be denied after exhausting tokens")
	}

	// Wait for refill (1 second = 1 token at 60/min rate)
	time.Sleep(1100 * time.Millisecond)

	// Should have ~1 token now
	if !rl.Allow() {
		t.Error("Should be allowed after refill")
	}

	// Should be denied again
	if rl.Allow() {
		t.Error("Should be denied after using refilled token")
	}
}

func TestRateLimiterWait(t *testing.T) {
	rl := NewRateLimiter(120) // 2 per second

	// Exhaust tokens
	for i := 0; i < 150; i++ {
		rl.Allow()
	}

	// Wait should block and then succeed
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait should succeed, got error: %v", err)
	}

	// Should have waited at least a few hundred milliseconds
	if elapsed < 100*time.Millisecond {
		t.Errorf("Wait should have blocked, elapsed time: %v", elapsed)
	}
}

func TestRateLimiterWaitContextCancellation(t *testing.T) {
	rl := NewRateLimiter(60)

	// Exhaust tokens
	for i := 0; i < 100; i++ {
		rl.Allow()
	}

	// Context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestRateLimiterCapLimit(t *testing.T) {
	rl := NewRateLimiter(60) // capacity = 60

	// Wait for tokens to accumulate beyond capacity
	time.Sleep(3 * time.Second)

	// Should not have more than capacity tokens
	allowedCount := 0
	for i := 0; i < 100; i++ {
		if rl.Allow() {
			allowedCount++
		}
	}

	// Should have allowed at most capacity + a few from refill during loop
	if allowedCount > 70 {
		t.Errorf("Rate limiter allowed too many requests: %d (capacity: 60)", allowedCount)
	}
}

func TestRateLimiterGetStats(t *testing.T) {
	rl := NewRateLimiter(120)

	// Use some tokens
	for i := 0; i < 10; i++ {
		rl.Allow()
	}

	tokens, capacity, rate := rl.GetStats()

	if capacity != 120 {
		t.Errorf("Expected capacity 120, got %d", capacity)
	}

	if rate != 2.0 { // 120/60 = 2 per second
		t.Errorf("Expected rate 2.0, got %f", rate)
	}

	// Tokens should be less than capacity after usage
	if tokens >= float64(capacity) {
		t.Errorf("Expected tokens < capacity after usage, got %f", tokens)
	}

	// Tokens should be positive (some remain)
	if tokens < 100 {
		t.Errorf("Expected some tokens remaining, got %f", tokens)
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	rl := NewRateLimiter(60)

	// Concurrent requests
	done := make(chan bool, 100)
	allowedCount := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			if rl.Allow() {
				allowedCount <- true
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	close(allowedCount)
	count := 0
	for range allowedCount {
		count++
	}

	// Should have allowed approximately capacity
	if count < 50 || count > 70 {
		t.Errorf("Expected ~60 concurrent requests allowed, got %d", count)
	}
}

func TestRateLimiterZeroRate(t *testing.T) {
	rl := NewRateLimiter(0)

	// With 0 rate, should only allow initial capacity (which is 0)
	if rl.Allow() {
		t.Error("Should not allow requests with 0 rate")
	}
}

func TestRateLimiterHighRate(t *testing.T) {
	rl := NewRateLimiter(6000) // 100 per second

	// Should allow many rapid requests
	allowedCount := 0
	for i := 0; i < 1000; i++ {
		if rl.Allow() {
			allowedCount++
		}
	}

	// Should have allowed a large portion
	if allowedCount < 1000 {
		t.Errorf("High rate limiter should allow most requests, got %d/1000", allowedCount)
	}
}

func TestRateLimiterSteadyState(t *testing.T) {
	rl := NewRateLimiter(60) // 1 per second

	// Exhaust initial tokens
	for i := 0; i < 100; i++ {
		rl.Allow()
	}

	// Over 3 seconds at 1/sec rate, should get ~3 requests
	start := time.Now()
	allowedCount := 0

	for time.Since(start) < 3*time.Second {
		if rl.Allow() {
			allowedCount++
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Should have allowed 2-4 requests (accounting for timing variance)
	if allowedCount < 2 || allowedCount > 4 {
		t.Errorf("Expected 2-4 requests over 3 seconds, got %d", allowedCount)
	}
}
