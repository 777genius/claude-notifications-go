package webhook

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitBreakerState represents the current state of the circuit breaker
type CircuitBreakerState int

const (
	// StateClosed - circuit is closed, requests pass through
	StateClosed CircuitBreakerState = iota
	// StateOpen - circuit is open, requests fail immediately
	StateOpen
	// StateHalfOpen - circuit is half-open, testing if backend recovered
	StateHalfOpen
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	failureThreshold int
	successThreshold int
	timeout          time.Duration

	mu               sync.RWMutex
	state            CircuitBreakerState
	failureCount     int
	successCount     int
	lastStateChange  time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		state:            StateClosed,
		lastStateChange:  time.Now(),
	}
}

// Execute runs the function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Check current state
	state := cb.getState()

	// If circuit is open, fail fast
	if state == StateOpen {
		return ErrCircuitOpen
	}

	// Execute the function
	err := fn()

	// Record result
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// getState returns the current state, potentially transitioning from Open to HalfOpen
func (cb *CircuitBreaker) getState() CircuitBreakerState {
	cb.mu.RLock()
	state := cb.state
	lastChange := cb.lastStateChange
	cb.mu.RUnlock()

	// If we're in Open state and timeout has passed, transition to HalfOpen
	if state == StateOpen && time.Since(lastChange) >= cb.timeout {
		cb.mu.Lock()
		// Double-check after acquiring write lock
		if cb.state == StateOpen && time.Since(cb.lastStateChange) >= cb.timeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.failureCount = 0
			cb.lastStateChange = time.Now()
			state = StateHalfOpen
		}
		cb.mu.Unlock()
	}

	return state
}

// recordSuccess records a successful call
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			// Transition to Closed
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
			cb.lastStateChange = time.Now()
		}
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0
	}
}

// recordFailure records a failed call
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		// Any failure in HalfOpen immediately goes back to Open
		cb.state = StateOpen
		cb.failureCount = 0
		cb.successCount = 0
		cb.lastStateChange = time.Now()

	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			// Transition to Open
			cb.state = StateOpen
			cb.failureCount = 0
			cb.lastStateChange = time.Now()
		}
	}
}

// GetState returns the current state (for monitoring/metrics)
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns current statistics
func (cb *CircuitBreaker) GetStats() (state CircuitBreakerState, failures, successes int) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state, cb.failureCount, cb.successCount
}

// String returns the state as a string
func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
