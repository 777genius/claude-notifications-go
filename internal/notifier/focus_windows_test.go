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

	restoreFG, restoreSnap := foregroundProcessPID, processSnapshot
	defer func() { foregroundProcessPID, processSnapshot = restoreFG, restoreSnap }()

	t.Run("focused when foreground owns an ancestor", func(t *testing.T) {
		foregroundProcessPID = func() (uint32, bool) { return 77, true }
		processSnapshot = func() (map[uint32]uint32, error) {
			return map[uint32]uint32{self: 55, 55: 77, 77: 1}, nil
		}
		if !terminalHasFocus() {
			t.Error("expected focus when foreground PID is an ancestor")
		}
	})

	t.Run("not focused when foreground window unavailable", func(t *testing.T) {
		foregroundProcessPID = func() (uint32, bool) { return 0, false }
		processSnapshot = func() (map[uint32]uint32, error) {
			return map[uint32]uint32{self: 55}, nil
		}
		if terminalHasFocus() {
			t.Error("expected no focus when foreground PID is unavailable")
		}
	})

	t.Run("not focused when snapshot fails", func(t *testing.T) {
		foregroundProcessPID = func() (uint32, bool) { return 77, true }
		processSnapshot = func() (map[uint32]uint32, error) { return nil, errors.New("snapshot failed") }
		if terminalHasFocus() {
			t.Error("expected no focus when the process snapshot fails")
		}
	})
}
