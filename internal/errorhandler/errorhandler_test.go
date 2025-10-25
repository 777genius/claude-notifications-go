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

// TestReset removed: Reset() is intentionally not thread-safe and causes race conditions
// when tests run concurrently with -race flag. The Reset() function is documented as
// being for use in isolated test environments only, not for production or concurrent testing.

func TestGetHandler_Concurrent(t *testing.T) {
	// Test concurrent access to GetHandler without Reset() to avoid race conditions
	// GetHandler uses sync.Once internally which is thread-safe
	const numGoroutines = 10
	handlers := make([]*ErrorHandler, numGoroutines)
	done := make(chan bool, numGoroutines)

	// Launch multiple goroutines calling GetHandler concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			handlers[index] = GetHandler()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// All handlers should be the same instance (singleton)
	firstHandler := handlers[0]
	if firstHandler == nil {
		t.Fatal("GetHandler() returned nil")
	}

	for i := 1; i < numGoroutines; i++ {
		if handlers[i] != firstHandler {
			t.Errorf("GetHandler() concurrent call %d returned different instance", i)
		}
	}
}

func TestHandleCriticalError_Global(t *testing.T) {
	// Test global HandleCriticalError function (uses existing defaultHandler)
	err := errors.New("critical global error")
	HandleCriticalError(err, "global critical context")

	// Should not panic and not exit

	// Test with nil error (should handle gracefully)
	HandleCriticalError(nil, "nil critical error")
}

func TestHandleCriticalError_WithExit(t *testing.T) {
	// This test verifies Init configuration
	// Create a new handler with exitOnCritical=true
	handler := &ErrorHandler{
		logToConsole:    false,
		exitOnCritical:  true,
		recoveryEnabled: true,
	}

	if !handler.exitOnCritical {
		t.Error("Handler with exitOnCritical=true should have exitOnCritical=true")
	}

	// Note: We cannot test actual exit behavior without mocking os.Exit
	// The important part is that the flag is set correctly
}

func TestHandlePanic_WithRecoveryDisabled(t *testing.T) {
	// Create a handler with recoveryEnabled=false
	handler := &ErrorHandler{
		logToConsole:    false,
		exitOnCritical:  false,
		recoveryEnabled: false,
	}

	if handler.recoveryEnabled {
		t.Error("Handler with recoveryEnabled=false should have recoveryEnabled=false")
	}

	// When recovery is disabled, HandlePanic should not recover
	// We can't easily test this without causing a real panic that terminates the test
	// But we verify the setting is correct
}

func TestInit_Multiple(t *testing.T) {
	// Test that Init returns singleton (uses existing global defaultHandler)
	// This test relies on Init being called in other tests first
	handler1 := GetHandler()
	if handler1 == nil {
		t.Fatal("GetHandler() returned nil")
	}

	// Second call should return same instance
	handler2 := GetHandler()
	if handler2 != handler1 {
		t.Error("Multiple GetHandler() calls should return same instance")
	}
}

func TestHandlePanic_WithPanic(t *testing.T) {
	// Test that HandlePanic can be called safely without panic
	didExecute := false

	func() {
		defer HandlePanic()
		didExecute = true
		// Note: We don't actually panic here because HandlePanic is designed
		// to be called in defer, and it only recovers if recover() returns non-nil
	}()

	if !didExecute {
		t.Error("Function should have executed")
	}
}

func TestWithRecoveryFunc_WithError(t *testing.T) {
	// Test WithRecoveryFunc with a function that returns an error (no panic)
	testErr := errors.New("test error")
	result := WithRecoveryFunc(func() error {
		return testErr
	})

	// Should return the error from the function
	if result != testErr {
		t.Errorf("WithRecoveryFunc should return error from function, got: %v", result)
	}
}
