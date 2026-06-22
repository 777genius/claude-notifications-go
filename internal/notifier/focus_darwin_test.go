//go:build darwin

package notifier

import (
	"testing"
)

func TestParseLsappinfoBundleID(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
		ok   bool
	}{
		{"quoted key and value", `"LSBundleID"="com.apple.Terminal"`, "com.apple.Terminal", true},
		{"bare key", `bundleID="com.googlecode.iterm2"`, "com.googlecode.iterm2", true},
		{"trailing newline", "\"LSBundleID\"=\"com.microsoft.VSCode\"\n", "com.microsoft.VSCode", true},
		{"no equals", "com.apple.Terminal", "", false},
		{"empty value", `"LSBundleID"=""`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseLsappinfoBundleID(tt.out)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseLsappinfoBundleID(%q) = (%q, %v), want (%q, %v)", tt.out, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTerminalHasFocus_Darwin(t *testing.T) {
	restoreFrontmost := frontmostBundleID
	restoreWindowMatch := frontmostTerminalWindowMatches
	defer func() {
		frontmostBundleID = restoreFrontmost
		frontmostTerminalWindowMatches = restoreWindowMatch
	}()

	// Pin our terminal identity positively so the comparison is deterministic
	// regardless of the host. Apple_Terminal maps to com.apple.Terminal, which is
	// also a positively-known identity (not the unidentifiable fallback).
	t.Setenv("__CFBundleIdentifier", "")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")

	t.Run("focused when frontmost app is our terminal", func(t *testing.T) {
		frontmostBundleID = func() (string, bool) { return "com.apple.Terminal", true }
		frontmostTerminalWindowMatches = func(bundleID, cwd string) bool {
			return bundleID == "com.apple.Terminal" && cwd == "/repo/my-project"
		}
		if !terminalHasFocus("session", "/repo/my-project") {
			t.Error("expected focus when the frontmost app matches our terminal bundle ID")
		}
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		frontmostBundleID = func() (string, bool) { return "COM.APPLE.TERMINAL", true }
		frontmostTerminalWindowMatches = func(bundleID, cwd string) bool { return true }
		if !terminalHasFocus("session", "/repo/my-project") {
			t.Error("expected a case-insensitive bundle ID match")
		}
	})

	t.Run("not focused when a different app is frontmost", func(t *testing.T) {
		frontmostBundleID = func() (string, bool) { return "com.google.Chrome", true }
		frontmostTerminalWindowMatches = func(bundleID, cwd string) bool { return true }
		if terminalHasFocus("session", "/repo/my-project") {
			t.Error("expected no focus when a different app is frontmost")
		}
	})

	t.Run("unknown (notify) when the query fails", func(t *testing.T) {
		frontmostBundleID = func() (string, bool) { return "", false }
		frontmostTerminalWindowMatches = func(bundleID, cwd string) bool { return true }
		if terminalHasFocus("session", "/repo/my-project") {
			t.Error("expected no focus (deliver) when the frontmost query fails")
		}
	})

	t.Run("unknown when terminal app matches but exact window does not", func(t *testing.T) {
		frontmostBundleID = func() (string, bool) { return "com.apple.Terminal", true }
		frontmostTerminalWindowMatches = func(bundleID, cwd string) bool { return false }
		if terminalHasFocus("session", "/repo/my-project") {
			t.Error("expected no focus (deliver) when the exact terminal window cannot be proven")
		}
	})
}

// TestTerminalHasFocus_Darwin_UnidentifiableTerminal verifies the fail-safe
// contract: when the terminal cannot be positively identified, GetTerminalBundleID
// falls back to com.apple.Terminal, but focus must still report unknown (false) so
// a frontmost Terminal.app does not falsely suppress the notification.
func TestTerminalHasFocus_Darwin_UnidentifiableTerminal(t *testing.T) {
	restoreFrontmost := frontmostBundleID
	restoreWindowMatch := frontmostTerminalWindowMatches
	defer func() {
		frontmostBundleID = restoreFrontmost
		frontmostTerminalWindowMatches = restoreWindowMatch
	}()

	// No positive identity: no __CFBundleIdentifier, unmapped TERM_PROGRAM.
	t.Setenv("__CFBundleIdentifier", "")
	t.Setenv("TERM_PROGRAM", "")

	frontmostBundleID = func() (string, bool) { return "com.apple.Terminal", true }
	frontmostTerminalWindowMatches = func(bundleID, cwd string) bool { return true }
	if terminalHasFocus("session", "/repo/my-project") {
		t.Error("expected no focus (deliver) when the terminal identity is unknown, even if Terminal.app is frontmost")
	}
}
