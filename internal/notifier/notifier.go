package notifier

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/gen2brain/beeep"

	"github.com/belief/claude-notifications/internal/analyzer"
	"github.com/belief/claude-notifications/internal/config"
	"github.com/belief/claude-notifications/internal/logging"
	"github.com/belief/claude-notifications/internal/platform"
)

// Notifier sends desktop notifications
type Notifier struct {
	cfg *config.Config
}

// New creates a new notifier
func New(cfg *config.Config) *Notifier {
	return &Notifier{
		cfg: cfg,
	}
}

// SendDesktop sends a desktop notification using beeep (cross-platform)
func (n *Notifier) SendDesktop(status analyzer.Status, message string) error {
	if !n.cfg.IsDesktopEnabled() {
		logging.Debug("Desktop notifications disabled, skipping")
		return nil
	}

	statusInfo, exists := n.cfg.GetStatusInfo(string(status))
	if !exists {
		return fmt.Errorf("unknown status: %s", status)
	}

	// Extract session name from message (format: "[session-name] actual message")
	sessionName, cleanMessage := extractSessionName(message)

	// Build proper title with session name
	title := statusInfo.Title
	if sessionName != "" {
		title = fmt.Sprintf("%s [%s]", title, sessionName)
	}

	// Get app icon path if configured
	appIcon := n.cfg.Notifications.Desktop.AppIcon
	if appIcon != "" && !platform.FileExists(appIcon) {
		logging.Warn("App icon not found: %s, using default", appIcon)
		appIcon = ""
	}

	// Set unique AppName to prevent notification grouping/replacement
	// Each notification gets a unique group ID based on timestamp
	originalAppName := beeep.AppName
	beeep.AppName = fmt.Sprintf("claude-notif-%d", time.Now().UnixNano())
	defer func() {
		beeep.AppName = originalAppName
	}()

	// Send notification using beeep with proper title and clean message
	if err := beeep.Notify(title, cleanMessage, appIcon); err != nil {
		logging.Error("Failed to send desktop notification: %v", err)
		return err
	}

	logging.Debug("Desktop notification sent via beeep: title=%s", title)

	// Play sound if enabled (platform-specific, as beeep doesn't support custom sounds)
	if n.cfg.Notifications.Desktop.Sound && statusInfo.Sound != "" {
		go n.playSound(statusInfo.Sound)
	}

	return nil
}

// playSound plays a sound file (platform-specific)
func (n *Notifier) playSound(soundPath string) {
	if !platform.FileExists(soundPath) {
		logging.Warn("Sound file not found: %s", soundPath)
		return
	}

	var cmd *exec.Cmd

	switch platform.OS() {
	case "macos":
		// Use afplay on macOS
		cmd = exec.Command("afplay", soundPath)
	case "linux":
		// Try paplay (PulseAudio) or aplay (ALSA) on Linux
		if _, err := exec.LookPath("paplay"); err == nil {
			cmd = exec.Command("paplay", soundPath)
		} else if _, err := exec.LookPath("aplay"); err == nil {
			cmd = exec.Command("aplay", soundPath)
		} else {
			logging.Warn("No sound player found on Linux (paplay or aplay)")
			return
		}
	case "windows":
		// Use PowerShell to play sound on Windows
		// Escape quotes in path to prevent command injection
		escapedPath := strings.ReplaceAll(soundPath, `"`, `\"`)
		cmd = exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`(New-Object Media.SoundPlayer "%s").PlaySync()`, escapedPath))
	default:
		logging.Warn("Sound playback not supported on this platform")
		return
	}

	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		logging.Error("Failed to play sound: %v", err)
		return
	}

	// Set timeout for sound playback
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		logging.Debug("Sound played: %s", soundPath)
	case <-time.After(5 * time.Second):
		// Timeout
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			logging.Warn("Sound playback timed out")
		}
	}
}

// extractSessionName extracts session name from message with format "[session-name] message"
// Returns session name and clean message without the prefix
func extractSessionName(message string) (string, string) {
	message = strings.TrimSpace(message)

	// Check if message starts with [
	if !strings.HasPrefix(message, "[") {
		return "", message
	}

	// Find closing bracket
	closingIdx := strings.Index(message, "]")
	if closingIdx == -1 {
		return "", message
	}

	// Extract session name (without brackets)
	sessionName := message[1:closingIdx]

	// Extract clean message (everything after "] ")
	cleanMessage := strings.TrimSpace(message[closingIdx+1:])

	return sessionName, cleanMessage
}
