package webhook

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerClosed(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	// Execute successful calls
	for i := 0; i < 5; i++ {
		err := cb.Execute(context.Background(), func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Unexpected error on attempt %d: %v", i+1, err)
		}
	}

	// Circuit should remain closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.GetState())
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	// Execute failing calls to open circuit
	for i := 0; i < 3; i++ {
		err := cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
		if err == nil {
			t.Error("Expected error, got nil")
		}
	}

	// Circuit should now be open
	if cb.GetState() != StateOpen {
		t.Errorf("Expected StateOpen, got %v", cb.GetState())
	}

	// Subsequent calls should fail immediately with ErrCircuitOpen
	err := cb.Execute(context.Background(), func() error {
		t.Error("Function should not be called when circuit is open")
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	if cb.GetState() != StateOpen {
		t.Fatalf("Circuit should be open, got %v", cb.GetState())
	}

	// Wait for timeout to transition to half-open
	time.Sleep(60 * time.Millisecond)

	// Next call should transition to half-open
	executed := false
	_ = cb.Execute(context.Background(), func() error {
		executed = true
		return nil
	})

	if !executed {
		t.Error("Function should be executed in half-open state")
	}
}

func TestCircuitBreakerHalfOpenToClosedSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Execute successful calls to close circuit
	for i := 0; i < 2; i++ {
		err := cb.Execute(context.Background(), func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Unexpected error on recovery attempt %d: %v", i+1, err)
		}
	}

	// Circuit should now be closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed after recovery, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenToOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	// Wait for timeout to transition to half-open
	time.Sleep(60 * time.Millisecond)

	// Execute one failing call - should immediately go back to open
	_ = cb.Execute(context.Background(), func() error {
		return errors.New("still failing")
	})

	// Circuit should be open again
	if cb.GetState() != StateOpen {
		t.Errorf("Expected StateOpen after half-open failure, got %v", cb.GetState())
	}

	// Next call should fail with ErrCircuitOpen
	err := cb.Execute(context.Background(), func() error {
		t.Error("Function should not be called when circuit is open")
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerResetOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	// Execute 2 failing calls (less than threshold)
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	// Execute successful call - should reset failure count
	err := cb.Execute(context.Background(), func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Circuit should still be closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.GetState())
	}

	// Execute 2 more failing calls (would open if count wasn't reset)
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	// Circuit should still be closed (only 2 consecutive failures)
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed after reset, got %v", cb.GetState())
	}
}

func TestCircuitBreakerGetStats(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	// Execute some failures
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	state, failures, successes := cb.GetStats()
	if state != StateClosed {
		t.Errorf("Expected StateClosed, got %v", state)
	}
	if failures != 2 {
		t.Errorf("Expected 2 failures, got %d", failures)
	}
	if successes != 0 {
		t.Errorf("Expected 0 successes, got %d", successes)
	}
}

func TestCircuitBreakerStateString(t *testing.T) {
	tests := []struct {
		state    CircuitBreakerState
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitBreakerState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCircuitBreakerConcurrency(t *testing.T) {
	cb := NewCircuitBreaker(10, 2, 100*time.Millisecond)

	// Execute concurrent requests
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			cb.Execute(context.Background(), func() error {
				time.Sleep(1 * time.Millisecond)
				if idx%2 == 0 {
					return nil
				}
				return errors.New("error")
			})
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Circuit state should be consistent
	state := cb.GetState()
	if state != StateClosed && state != StateOpen && state != StateHalfOpen {
		t.Errorf("Invalid circuit state: %v", state)
	}
}

func TestCircuitBreakerPartialSuccessInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 3, 50*time.Millisecond)

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("service error")
		})
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Execute 2 successful calls (need 3 for threshold)
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func() error {
			return nil
		})
	}

	// Should still be in half-open (need 3 successes)
	state, _, successes := cb.GetStats()
	if state != StateHalfOpen {
		t.Errorf("Expected StateHalfOpen, got %v", state)
	}
	if successes != 2 {
		t.Errorf("Expected 2 successes, got %d", successes)
	}

	// One more success should close it
	cb.Execute(context.Background(), func() error {
		return nil
	})

	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed after threshold, got %v", cb.GetState())
	}
}
