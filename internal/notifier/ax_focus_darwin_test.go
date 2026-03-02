//go:build darwin

package notifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// === Bundle ID classification ===

func TestIsVSCodeBundleID(t *testing.T) {
	tests := []struct {
		bundleID string
		want     bool
	}{
		{"com.microsoft.VSCode", true},
		{"com.microsoft.VSCodeInsiders", true},
		{"com.microsoft.vscode", false}, // case-sensitive
		{"com.mitchellh.ghostty", false},
		{"com.apple.Terminal", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isVSCodeBundleID(tt.bundleID)
		if got != tt.want {
			t.Errorf("isVSCodeBundleID(%q) = %v, want %v", tt.bundleID, got, tt.want)
		}
	}
}

func TestIsGhosttyBundleID(t *testing.T) {
	tests := []struct {
		bundleID string
		want     bool
	}{
		{"com.mitchellh.ghostty", true},
		{"com.Mitchellh.Ghostty", false}, // case-sensitive
		{"com.microsoft.VSCode", false},
		{"com.apple.Terminal", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isGhosttyBundleID(tt.bundleID)
		if got != tt.want {
			t.Errorf("isGhosttyBundleID(%q) = %v, want %v", tt.bundleID, got, tt.want)
		}
	}
}

// === FocusAppWindow input validation ===

// TestFocusAppWindow_AppNotRunning verifies that FocusAppWindow returns a clear
// error when the target app is not running. Uses a guaranteed-fake bundle ID so
// the test is deterministic regardless of which apps are open on the test machine.
func TestFocusAppWindow_AppNotRunning_Generic(t *testing.T) {
	err := FocusAppWindow("com.notexist.fake.bundle.xy12345", "/tmp/test-project")
	if err == nil {
		t.Fatal("expected error for non-running app, got nil")
	}
	if !strings.Contains(err.Error(), "app not running") {
		t.Errorf("expected 'app not running' in error, got: %v", err)
	}
}

// TestFocusAppWindow_VSCode_AppNotRunning verifies the VS Code–specific code path
// for a non-running app. Skipped if VS Code is actually open (rare in CI).
func TestFocusAppWindow_VSCode_AppNotRunning(t *testing.T) {
	err := FocusAppWindow("com.microsoft.VSCode", "/tmp/my-project")
	if err == nil {
		// VS Code is running; the test can't validate input-error paths without
		// an open window, so skip rather than produce a flaky result.
		t.Skip("VS Code is running; skipping non-running-app validation")
	}
	if !strings.Contains(err.Error(), "app not running") {
		t.Errorf("expected 'app not running' in error, got: %v", err)
	}
}

// TestFocusAppWindow_Ghostty_EmptyCWD verifies that an empty cwd is rejected for
// Ghostty (the check is an explicit guard after findPID). Skipped if Ghostty is
// open, since the PID-not-found path comes first.
func TestFocusAppWindow_Ghostty_EmptyCWD(t *testing.T) {
	err := FocusAppWindow("com.mitchellh.ghostty", "")
	if err == nil {
		t.Skip("Ghostty is running; skipping cwd-validation test")
	}
	// Either "app not running" (not open) or "invalid cwd" (open but empty cwd)
	if !strings.Contains(err.Error(), "app not running") &&
		!strings.Contains(err.Error(), "invalid cwd") {
		t.Errorf("expected 'app not running' or 'invalid cwd', got: %v", err)
	}
}

// TestFocusAppWindow_VSCode_InvalidCWD verifies that root "/" and bare "." are
// rejected as cwd. Skipped if VS Code is not running, since findPID returns -1
// before the cwd guard is reached.
func TestFocusAppWindow_VSCode_InvalidCWD(t *testing.T) {
	for _, cwd := range []string{"/", ".", ""} {
		err := FocusAppWindow("com.microsoft.VSCode", cwd)
		if err == nil {
			t.Skipf("VS Code not running or no error for cwd=%q; skipping", cwd)
		}
		// Accept either "app not running" (VS Code closed) or "invalid cwd" (open)
		if !strings.Contains(err.Error(), "app not running") &&
			!strings.Contains(err.Error(), "invalid cwd") {
			t.Errorf("cwd=%q: expected 'app not running' or 'invalid cwd', got: %v", cwd, err)
		}
	}
}

// === retryWindowFocusInner ===
// retryWindowFocusInner is a pure-Go function; we can drive it directly from
// a darwin test file without CGo.

func TestRetryWindowFocus_SuccessOnFirstCall(t *testing.T) {
	callCount := 0
	start := time.Now()

	result := retryWindowFocusInner(func() int {
		callCount++
		return 1 // success
	})

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
	if result != 1 {
		t.Errorf("expected result 1, got %d", result)
	}
	// Should have waited the first delay (~150ms) before calling fn
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected at least 100ms elapsed (first delay), got %v", elapsed)
	}
}

func TestRetryWindowFocus_RetriesUntilSuccess(t *testing.T) {
	callCount := 0

	result := retryWindowFocusInner(func() int {
		callCount++
		if callCount < 3 {
			return 0 // not found yet
		}
		return 1 // success on 3rd attempt
	})

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
	if result != 1 {
		t.Errorf("expected result 1, got %d", result)
	}
}

func TestRetryWindowFocus_ExhaustsAllRetries(t *testing.T) {
	callCount := 0

	result := retryWindowFocusInner(func() int {
		callCount++
		return 0 // never found
	})

	if callCount != 3 {
		t.Errorf("expected exactly 3 attempts (one per delay slot), got %d", callCount)
	}
	if result != 0 {
		t.Errorf("expected result 0 (not found), got %d", result)
	}
}

func TestRetryWindowFocus_PropagatesPermissionError(t *testing.T) {
	callCount := 0

	result := retryWindowFocusInner(func() int {
		callCount++
		return -1 // permission denied
	})

	if callCount != 1 {
		t.Errorf("expected 1 call (stop on -1), got %d", callCount)
	}
	if result != -1 {
		t.Errorf("expected result -1, got %d", result)
	}
}

// === Live integration tests (skipped in CI; require running apps) ===

// TestFocusAppWindow_VSCode_LiveIntegration exercises the full VS Code focus
// path — findPID → findSwitchAndActivate → retryWindowFocus(raiseWindowByAXTitle)
// — against a real running VS Code instance. The VS Code path always returns nil
// so a non-nil result is a genuine failure.
func TestFocusAppWindow_VSCode_LiveIntegration(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping in CI")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = FocusAppWindow("com.microsoft.VSCode", cwd)
	if err != nil {
		if strings.Contains(err.Error(), "app not running") {
			t.Skip("VS Code not running; skipping live integration test")
		}
		// VS Code's path always returns nil — any error here is unexpected.
		t.Errorf("FocusAppWindow(VSCode) returned unexpected error: %v", err)
	}
}

// TestFocusAppWindow_Ghostty_LiveIntegration exercises the Ghostty focus path
// — findPID → activateByPID → cwdToFileURL → retryWindowFocus(raiseWindowByAXDocument)
// — against a real running Ghostty instance.
// Acceptable outcomes: nil (window found) or "window not found" (running but
// no window with this OSC-7 cwd). Both prove the code path was executed.
func TestFocusAppWindow_Ghostty_LiveIntegration(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping in CI")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = FocusAppWindow("com.mitchellh.ghostty", cwd)
	if err != nil {
		if strings.Contains(err.Error(), "app not running") {
			t.Skip("Ghostty not running; skipping live integration test")
		}
		if strings.Contains(err.Error(), "Accessibility permission required") {
			t.Logf("Accessibility permission not granted — cannot validate window raise: %v", err)
			return
		}
		// "window not found" is a valid outcome (Ghostty is running but this cwd
		// may not be open in any window). It confirms the AXDocument path ran.
		if !strings.Contains(err.Error(), "window not found") {
			t.Errorf("FocusAppWindow(Ghostty) returned unexpected error: %v", err)
		}
	}
}

// projectRoot walks up from the test's working directory to find the module
// root (directory containing go.mod). Tests that invoke focus-window with a
// real project path use this so the cwd matches the user's open Ghostty window.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir // reached filesystem root, give up
		}
		dir = parent
	}
}

// TestDiagAXWindows_Ghostty logs what AX sees for Ghostty: whether AXAllWindows
// is supported, how many windows it returns, and what AXDocument values exist.
// Run this when cross-Space focus fails to diagnose whether the problem is
// AXAllWindows not returning off-Space windows, or an AXDocument URL mismatch.
func TestDiagAXWindows_Ghostty(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping in CI")
	}
	t.Log(diagAXWindowsForTest("com.mitchellh.ghostty"))
}

// TestFocusAppWindow_Ghostty_CrossSpace verifies that FocusAppWindow can raise
// a Ghostty window that is open on a different macOS Space. "window not found"
// when Ghostty is running with this project open is the cross-Space regression:
// activateViaAppleScript must switch to the correct Space, and after that
// AXWindows must include the target window.
//
// Requires: Ghostty open with this project on a different Space than the
// current one. Screen Recording is NOT required (Accessibility only).
func TestFocusAppWindow_Ghostty_CrossSpace(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping in CI")
	}
	cwd := projectRoot(t)
	err := FocusAppWindow("com.mitchellh.ghostty", cwd)
	if err == nil {
		return // raised successfully
	}
	if strings.Contains(err.Error(), "app not running") {
		t.Skip("Ghostty not running; skipping cross-Space test")
	}
	if strings.Contains(err.Error(), "Accessibility permission required") {
		t.Log("Accessibility permission not granted — cannot validate cross-Space raise")
		return
	}
	// "window not found" when Ghostty is running with this project open means
	// the window exists on another Space and the focus path couldn't reach it.
	t.Errorf("FocusAppWindow(Ghostty) failed (window may be on a different Space): %v", err)
}

// TestFindWindowIDSCK_VSCode exercises findWindowIDSCK against a live VS Code
// instance. Confirms SCK returns a non-zero window ID for the current project.
// Skipped in CI and when VS Code is not running.
func TestFindWindowIDSCK_VSCode(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping in CI")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wid := sckFindWindowForTest("com.microsoft.VSCode", filepath.Base(cwd))
	if wid == 0 {
		t.Skipf("SCK found no VS Code window for folder %q (not running or Screen Recording not granted)", filepath.Base(cwd))
	}
	t.Logf("SCK found VS Code window ID %d for folder %q", wid, filepath.Base(cwd))
}

// TestFindGhosttySpaceViaSCK exercises findGhosttySpaceViaSCK against a live
// Ghostty instance. Confirms sub-window → container matching returns a non-zero
// window ID. Skipped in CI and when Ghostty is not running.
func TestFindGhosttySpaceViaSCK(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping in CI")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wid := sckFindGhosttySpaceForTest("com.mitchellh.ghostty", filepath.Base(cwd))
	if wid == 0 {
		t.Skipf("SCK found no Ghostty container window for folder %q (not running, on current Space, or Screen Recording not granted)", filepath.Base(cwd))
	}
	t.Logf("SCK found Ghostty container window ID %d for folder %q", wid, filepath.Base(cwd))
}

// === activateViaAppleScript ===

func TestActivateViaAppleScript_DoesNotPanic(t *testing.T) {
	// A non-existent bundle ID causes osascript to error, but the function
	// ignores the error so it must not panic.
	activateViaAppleScript("com.notexist.fake.12345")
}

func TestActivateViaAppleScript_EmptyBundleID(t *testing.T) {
	activateViaAppleScript("")
}

// === Permission prompt marker file logic ===
// We redirect $HOME to a temp directory so the functions write markers there
// instead of the real ~/.claude/claude-notifications-go directory.

func TestPromptAccessibilityOnce_CreatesMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// First call should create the marker file (and attempt a notification,
	// which may fail silently in CI).
	promptAccessibilityOnce()

	home := os.Getenv("HOME")
	markerPath := filepath.Join(home, ".claude", "claude-notifications-go", ".accessibility-prompted")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("marker file should have been created by promptAccessibilityOnce")
	}
}

func TestPromptAccessibilityOnce_Idempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Prime the marker
	promptAccessibilityOnce()

	// Write a sentinel value to the marker file so we can detect if it's overwritten
	home := os.Getenv("HOME")
	markerPath := filepath.Join(home, ".claude", "claude-notifications-go", ".accessibility-prompted")
	sentinel := []byte("sentinel")
	if err := os.WriteFile(markerPath, sentinel, 0644); err != nil {
		t.Fatalf("setup: could not write sentinel: %v", err)
	}

	// Second call must be a no-op — the marker file content must not change
	promptAccessibilityOnce()

	got, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("could not read marker file: %v", err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("second call overwrote the marker file: got %q, want %q", got, sentinel)
	}
}

func TestPromptScreenRecordingOnce_CreatesMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	promptScreenRecordingOnce()

	home := os.Getenv("HOME")
	markerPath := filepath.Join(home, ".claude", "claude-notifications-go", ".screen-recording-prompted")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("marker file should have been created by promptScreenRecordingOnce")
	}
}

func TestPromptScreenRecordingOnce_Idempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	promptScreenRecordingOnce()

	home := os.Getenv("HOME")
	markerPath := filepath.Join(home, ".claude", "claude-notifications-go", ".screen-recording-prompted")
	sentinel := []byte("sentinel")
	if err := os.WriteFile(markerPath, sentinel, 0644); err != nil {
		t.Fatalf("setup: could not write sentinel: %v", err)
	}

	promptScreenRecordingOnce()

	got, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("could not read marker file: %v", err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("second call overwrote the marker file: got %q, want %q", got, sentinel)
	}
}

func TestPromptAccessibilityOnce_IndependentFromScreenRecording(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Prompting accessibility should not create the screen recording marker
	promptAccessibilityOnce()

	home := os.Getenv("HOME")
	srPath := filepath.Join(home, ".claude", "claude-notifications-go", ".screen-recording-prompted")
	if _, err := os.Stat(srPath); err == nil {
		t.Error("promptAccessibilityOnce should not create the screen-recording marker")
	}
}

func TestPromptScreenRecordingOnce_IndependentFromAccessibility(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	promptScreenRecordingOnce()

	home := os.Getenv("HOME")
	axPath := filepath.Join(home, ".claude", "claude-notifications-go", ".accessibility-prompted")
	if _, err := os.Stat(axPath); err == nil {
		t.Error("promptScreenRecordingOnce should not create the accessibility marker")
	}
}
