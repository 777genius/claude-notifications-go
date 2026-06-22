//go:build windows

package notifier

import (
	"errors"
	"os"
	"testing"
)

func TestPidFocused(t *testing.T) {
	self := uint32(1000)
	tests := []struct {
		name      string
		fgPID     uint32
		pidToPPID map[uint32]uint32
		want      bool
	}{
		{
			name:      "foreground is a direct ancestor (the terminal)",
			fgPID:     42,
			pidToPPID: map[uint32]uint32{1000: 500, 500: 42, 42: 1},
			want:      true,
		},
		{
			name:      "foreground is the process itself",
			fgPID:     1000,
			pidToPPID: map[uint32]uint32{1000: 500},
			want:      true,
		},
		{
			name:      "foreground is unrelated",
			fgPID:     9999,
			pidToPPID: map[uint32]uint32{1000: 500, 500: 42, 42: 1},
			want:      false,
		},
		{
			name:      "zero foreground pid",
			fgPID:     0,
			pidToPPID: map[uint32]uint32{1000: 500},
			want:      false,
		},
		{
			name:      "broken chain (parent missing from snapshot)",
			fgPID:     42,
			pidToPPID: map[uint32]uint32{1000: 500},
			want:      false,
		},
		{
			name:      "cycle does not hang and does not match",
			fgPID:     7,
			pidToPPID: map[uint32]uint32{1000: 500, 500: 1000},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pidFocused(tt.fgPID, self, tt.pidToPPID); got != tt.want {
				t.Errorf("pidFocused() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTerminalHasFocus_Windows(t *testing.T) {
	self := uint32(os.Getpid())

	restoreFG, restoreSnap := foregroundWindowInfo, processSnapshot
	defer func() { foregroundWindowInfo, processSnapshot = restoreFG, restoreSnap }()

	t.Run("focused when foreground owns an ancestor and title matches cwd folder", func(t *testing.T) {
		foregroundWindowInfo = func() (focusedWindowInfo, bool) {
			return focusedWindowInfo{PID: 77, Title: "notification_plugin_go - PowerShell"}, true
		}
		processSnapshot = func() (map[uint32]uint32, error) {
			return map[uint32]uint32{self: 55, 55: 77, 77: 1}, nil
		}
		if !terminalHasFocus("", `C:\dev\notification_plugin_go`) {
			t.Error("expected focus when foreground PID is an ancestor")
		}
	})

	t.Run("not focused when ancestor process matches but title does not prove the project", func(t *testing.T) {
		foregroundWindowInfo = func() (focusedWindowInfo, bool) {
			return focusedWindowInfo{PID: 77, Title: "other-project - PowerShell"}, true
		}
		processSnapshot = func() (map[uint32]uint32, error) {
			return map[uint32]uint32{self: 55, 55: 77, 77: 1}, nil
		}
		if terminalHasFocus("", `C:\dev\notification_plugin_go`) {
			t.Error("expected no focus when the foreground terminal window title does not match this project")
		}
	})

	t.Run("not focused when foreground window unavailable", func(t *testing.T) {
		foregroundWindowInfo = func() (focusedWindowInfo, bool) { return focusedWindowInfo{}, false }
		processSnapshot = func() (map[uint32]uint32, error) {
			return map[uint32]uint32{self: 55}, nil
		}
		if terminalHasFocus("", `C:\dev\notification_plugin_go`) {
			t.Error("expected no focus when foreground PID is unavailable")
		}
	})

	t.Run("not focused when snapshot fails", func(t *testing.T) {
		foregroundWindowInfo = func() (focusedWindowInfo, bool) {
			return focusedWindowInfo{PID: 77, Title: "notification_plugin_go"}, true
		}
		processSnapshot = func() (map[uint32]uint32, error) { return nil, errors.New("snapshot failed") }
		if terminalHasFocus("", `C:\dev\notification_plugin_go`) {
			t.Error("expected no focus when the process snapshot fails")
		}
	})
}

func TestWindowTitleMatchesFolder_Windows(t *testing.T) {
	if !windowTitleMatchesFolder("notification_plugin_go - Windows Terminal", `C:\dev\notification_plugin_go`) {
		t.Fatal("expected title to match cwd folder")
	}
	if windowTitleMatchesFolder("other-project - Windows Terminal", `C:\dev\notification_plugin_go`) {
		t.Fatal("expected unrelated title not to match cwd folder")
	}
	if windowTitleMatchesFolder("notification_plugin_go - Windows Terminal", "") {
		t.Fatal("empty cwd must not be treated as focused")
	}
}
