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

func TestReset(t *testing.T) {
	// Initialize handler
	handler := Init(false, false, true)
	if handler == nil {
		t.Fatal("Init() returned nil")
	}

	// Reset should clear defaultHandler
	Reset()

	// After reset, GetHandler should create a new instance
	newHandler := GetHandler()
	if newHandler == nil {
		t.Fatal("GetHandler() after Reset() returned nil")
	}

	// Verify it's a new instance (default settings)
	if !newHandler.logToConsole {
		t.Error("GetHandler() after Reset() should have logToConsole=true by default")
	}
}

func TestGetHandler_Concurrent(t *testing.T) {
	// Reset to start fresh
	Reset()

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
	for i := 1; i < numGoroutines; i++ {
		if handlers[i] != firstHandler {
			t.Errorf("GetHandler() concurrent call %d returned different instance", i)
		}
	}
}

func TestHandleCriticalError_Global(t *testing.T) {
	// Reset and initialize
	Reset()
	Init(false, false, true) // exitOnCritical=false

	// Test global HandleCriticalError function
	err := errors.New("critical global error")
	HandleCriticalError(err, "global critical context")

	// Should not panic and not exit (since exitOnCritical=false)

	// Test with nil error (should handle gracefully)
	HandleCriticalError(nil, "nil critical error")
}

func TestHandleCriticalError_WithExit(t *testing.T) {
	// This test cannot actually test os.Exit() as it would terminate the test
	// Instead, we verify the handler is configured correctly
	Reset()
	handler := Init(false, true, true) // exitOnCritical=true

	if !handler.exitOnCritical {
		t.Error("Init with exitOnCritical=true should set handler.exitOnCritical=true")
	}

	// Note: We cannot test actual exit behavior without mocking os.Exit
	// The important part is that the flag is set correctly
}

func TestHandlePanic_WithRecoveryDisabled(t *testing.T) {
	Reset()
	handler := Init(false, false, false) // recoveryEnabled=false

	if handler.recoveryEnabled {
		t.Error("Init with recoveryEnabled=false should set handler.recoveryEnabled=false")
	}

	// When recovery is disabled, HandlePanic should not recover
	// We can't easily test this without causing a real panic that terminates the test
	// But we verify the setting is correct
}

func TestInit_Multiple(t *testing.T) {
	Reset()

	// First init
	handler1 := Init(true, false, true)
	if handler1 == nil {
		t.Fatal("First Init() returned nil")
	}

	// Second init should return same instance (singleton pattern)
	handler2 := Init(false, true, false) // Different settings
	if handler2 != handler1 {
		t.Error("Second Init() should return same instance, got different instance")
	}

	// Settings should be from first Init (not second)
	if !handler2.logToConsole {
		t.Error("Handler should retain first Init() settings (logToConsole=true)")
	}
	if handler2.exitOnCritical {
		t.Error("Handler should retain first Init() settings (exitOnCritical=false)")
	}
	if !handler2.recoveryEnabled {
		t.Error("Handler should retain first Init() settings (recoveryEnabled=true)")
	}
}

func TestHandlePanic_WithPanic(t *testing.T) {
	Reset()
	Init(false, false, true) // recoveryEnabled=true

	// Test that HandlePanic actually recovers from panic
	// Note: This test verifies that HandlePanic can be called safely
	// The actual recovery behavior is tested in TestErrorHandler_HandlePanic
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
	Reset()
	Init(false, false, true)

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
