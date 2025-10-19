package errorhandler

import (
	"errors"
	"testing"
)

func TestErrorHandler_HandleError(t *testing.T) {
	handler := Init(false, false, true)

	err := errors.New("test error")
	handler.HandleError(err, "test context")

	// Should not panic
	handler.HandleError(nil, "nil error")
}

func TestErrorHandler_HandleCriticalError(t *testing.T) {
	handler := Init(false, false, true)

	err := errors.New("critical test error")
	handler.HandleCriticalError(err, "critical context")

	// Should not panic
	handler.HandleCriticalError(nil, "nil error")
}

func TestErrorHandler_HandlePanic(t *testing.T) {
	handler := Init(false, false, true)

	// Test panic recovery
	func() {
		defer handler.HandlePanic()
		panic("test panic")
	}()

	// If we reach here, panic was recovered successfully
}

func TestWithRecovery(t *testing.T) {
	Init(false, false, true)

	// WithRecovery should not panic when calling a normal function
	WithRecovery(func() {
		// Normal execution
	})

	// If we reach here, test passed
}

func TestWithRecoveryFunc(t *testing.T) {
	Init(false, false, true)

	// WithRecoveryFunc should work with normal error returns
	err := WithRecoveryFunc(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}
}

func TestSafeGo(t *testing.T) {
	Init(false, false, true)

	done := make(chan bool)

	// Should execute function in goroutine
	SafeGo(func() {
		done <- true
	})

	<-done
	// If we reach here, test passed
}

func TestGlobalFunctions(t *testing.T) {
	Init(false, false, true)

	// Test global convenience functions
	HandleError(errors.New("global error"), "global context")
	Warn("warning message: %s", "test")
	Info("info message: %s", "test")
	Debug("debug message: %s", "test")
}
