//go:build linux

package notifier

import (
	"errors"
	"testing"
)

func TestParseWindowID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want uint64
		ok   bool
	}{
		{"decimal", "31457289", 31457289, true},
		{"hex", "0x1e00009", 0x1e00009, true},
		{"whitespace trimmed", "  12345 \n", 12345, true},
		{"empty", "", 0, false},
		{"not a number", "window", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWindowID(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseWindowID(%q) = (%d, %v), want (%d, %v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTerminalHasFocus_Linux(t *testing.T) {
	restore := activeWindowID
	defer func() { activeWindowID = restore }()

	// Set whenever the suite runs from Claude Code's own VS Code extension, where the
	// window ID is now deliberately dropped. Left in place, every case below would see
	// no window and report unfocused — green in CI, red for anyone running the tests
	// from the extension.
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "")

	t.Run("focused when active window matches WINDOWID", func(t *testing.T) {
		t.Setenv("WINDOWID", "0x1e00009")
		activeWindowID = func() (string, error) { return "31457289", nil } // == 0x1e00009
		if !terminalHasFocus("", "/repo") {
			t.Error("expected focus when the active window equals WINDOWID")
		}
	})

	t.Run("not focused when active window differs", func(t *testing.T) {
		t.Setenv("WINDOWID", "100")
		activeWindowID = func() (string, error) { return "200", nil }
		if terminalHasFocus("", "/repo") {
			t.Error("expected no focus when the active window differs")
		}
	})

	t.Run("unknown (notify) when WINDOWID is unset (e.g. Wayland)", func(t *testing.T) {
		t.Setenv("WINDOWID", "")
		activeWindowID = func() (string, error) { return "200", nil }
		if terminalHasFocus("", "/repo") {
			t.Error("expected no focus (deliver) when WINDOWID is unset")
		}
	})

	t.Run("unknown (notify) when the active-window query fails", func(t *testing.T) {
		t.Setenv("WINDOWID", "100")
		activeWindowID = func() (string, error) { return "", errors.New("xdotool missing") }
		if terminalHasFocus("", "/repo") {
			t.Error("expected no focus (deliver) when the query fails")
		}
	})
}

// TestTerminalHasFocus_VSCodeExtensionIgnoresInheritedWindow covers the other
// consumer of WINDOWID. Under the extension host the variable names the terminal
// that launched VS Code, so comparing against it would report "focused" whenever
// that terminal is in front — and notifyOnlyWhenUnfocused would then swallow a
// notification the user never saw.
func TestTerminalHasFocus_VSCodeExtensionIgnoresInheritedWindow(t *testing.T) {
	restore := activeWindowID
	defer func() { activeWindowID = restore }()

	// The inherited window is the active one: the trap this guards against.
	activeWindowID = func() (string, error) { return "96249407204176", nil }

	t.Setenv("WINDOWID", "96249407204176")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "claude-vscode")

	if terminalHasFocus("session", "/project") {
		t.Error("the extension host must not treat an inherited window as its own")
	}
}

// TestTerminalHasFocus_TerminalStillMatchesItsOwnWindow pins the case the check
// exists for: a terminal session comparing against the window it really owns.
func TestTerminalHasFocus_TerminalStillMatchesItsOwnWindow(t *testing.T) {
	restore := activeWindowID
	defer func() { activeWindowID = restore }()

	activeWindowID = func() (string, error) { return "96249407204176", nil }

	t.Setenv("WINDOWID", "96249407204176")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")

	if !terminalHasFocus("session", "/project") {
		t.Error("a terminal owning the active window should report focused")
	}
}
