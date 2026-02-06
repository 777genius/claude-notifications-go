//go:build darwin

package notifier

import (
	"os"
	"os/exec"
	"strings"

	"github.com/777genius/claude-notifications/internal/logging"
)

// TmuxPaneInfo contains the current tmux pane details for click-to-focus navigation
type TmuxPaneInfo struct {
	SessionName string // tmux session name
	WindowIndex string // window index within session
	PaneIndex   string // pane index within window
	Target      string // full target string like "session:0.1"
}

// GetCurrentTmuxPane detects if we're running inside tmux and returns the current pane info.
// Returns nil if not in a tmux session.
func GetCurrentTmuxPane() *TmuxPaneInfo {
	// Check if we're inside tmux
	if os.Getenv("TMUX") == "" {
		return nil
	}

	// Get the full pane target in format "session_name:window_index.pane_index"
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}:#{window_index}.#{pane_index}")
	output, err := cmd.Output()
	if err != nil {
		logging.Debug("Failed to get tmux pane info: %v", err)
		return nil
	}

	target := strings.TrimSpace(string(output))
	if target == "" {
		return nil
	}

	// Parse the target string
	// Format: "session_name:window_index.pane_index" (e.g., "main:0.2")
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 {
		logging.Debug("Unexpected tmux target format: %s", target)
		return nil
	}

	sessionName := parts[0]
	windowPane := parts[1]

	wpParts := strings.SplitN(windowPane, ".", 2)
	if len(wpParts) != 2 {
		logging.Debug("Unexpected tmux window.pane format: %s", windowPane)
		return nil
	}

	info := &TmuxPaneInfo{
		SessionName: sessionName,
		WindowIndex: wpParts[0],
		PaneIndex:   wpParts[1],
		Target:      target,
	}

	logging.Debug("Detected tmux pane: %s", target)
	return info
}

// BuildTmuxFocusCommand builds a shell command that will:
// 1. Switch to the correct tmux session/window/pane
// 2. Activate the terminal application
func BuildTmuxFocusCommand(pane *TmuxPaneInfo, terminalBundleID string) string {
	if pane == nil {
		return ""
	}

	// Use switch-client for attached sessions, or attach if detached
	// Also select the specific window and pane
	// Finally, bring the terminal app to front with 'open -b'
	cmd := "tmux switch-client -t '" + pane.Target + "' 2>/dev/null || " +
		"tmux select-window -t '" + pane.SessionName + ":" + pane.WindowIndex + "' 2>/dev/null; " +
		"tmux select-pane -t '" + pane.Target + "' 2>/dev/null; " +
		"open -b '" + terminalBundleID + "'"

	logging.Debug("Built tmux focus command: %s", cmd)
	return cmd
}
