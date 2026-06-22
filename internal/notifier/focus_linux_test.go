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
