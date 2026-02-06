package notifier

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/beeep"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/audio"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/errorhandler"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/platform"
)

// Notifier sends desktop notifications
type Notifier struct {
	cfg         *config.Config
	audioPlayer *audio.Player
	playerInit  sync.Once
	playerErr   error
	mu          sync.Mutex
	wg          sync.WaitGroup
	closing     bool // Prevents new sounds from being enqueued after Close() is called
}

// New creates a new notifier
func New(cfg *config.Config) *Notifier {
	return &Notifier{
		cfg: cfg,
	}
}

// SendDesktop sends a desktop notification using beeep (cross-platform)
// On macOS with clickToFocus enabled, uses terminal-notifier for click-to-focus support
func (n *Notifier) SendDesktop(status analyzer.Status, message string) error {
	if !n.cfg.IsDesktopEnabled() {
		logging.Debug("Desktop notifications disabled, skipping")
		return nil
	}

	statusInfo, exists := n.cfg.GetStatusInfo(string(status))
	if !exists {
		return fmt.Errorf("unknown status: %s", status)
	}

	// Extract session name and git branch from message
	// Format: "[session-name|branch] actual message" or "[session-name] actual message"
	sessionName, gitBranch, cleanMessage := extractSessionInfo(message)

	// Build proper title with session name and git branch
	// Format: "✅ Completed [brave-ocean] main" or "✅ Completed [brave-ocean]"
	title := statusInfo.Title
	if sessionName != "" {
		if gitBranch != "" {
			title = fmt.Sprintf("%s [%s] %s", title, sessionName, gitBranch)
		} else {
			title = fmt.Sprintf("%s [%s]", title, sessionName)
		}
	}

	// Get app icon path if configured
	appIcon := n.cfg.Notifications.Desktop.AppIcon
	if appIcon != "" && !platform.FileExists(appIcon) {
		logging.Warn("App icon not found: %s, using default", appIcon)
		appIcon = ""
	}

	// macOS: Try terminal-notifier for click-to-focus support
	if platform.IsMacOS() && n.cfg.Notifications.Desktop.ClickToFocus {
		if IsTerminalNotifierAvailable() {
			if err := n.sendWithTerminalNotifier(title, cleanMessage); err != nil {
				logging.Warn("terminal-notifier failed, falling back to beeep: %v", err)
				// Fall through to beeep
			} else {
				logging.Debug("Desktop notification sent via terminal-notifier: title=%s", title)
				n.playSoundAsync(statusInfo.Sound)
				return nil
			}
		} else {
			logging.Debug("terminal-notifier not available, using beeep (run /claude-notifications-go:notifications-init to enable click-to-focus)")
		}
	}

	// Standard path: beeep (Windows, Linux, macOS fallback)
	return n.sendWithBeeep(title, cleanMessage, appIcon, statusInfo.Sound)
}

// sendWithTerminalNotifier sends notification via terminal-notifier on macOS
// with click-to-focus support (clicking notification activates the terminal)
// When running inside tmux, clicking the notification will navigate to the exact pane
func (n *Notifier) sendWithTerminalNotifier(title, message string) error {
	notifierPath, err := GetTerminalNotifierPath()
	if err != nil {
		return fmt.Errorf("terminal-notifier not found: %w", err)
	}

	bundleID := GetTerminalBundleID(n.cfg.Notifications.Desktop.TerminalBundleID)
	
	// Check if we're running inside tmux
	tmuxPane := GetCurrentTmuxPane()
	
	var args []string
	if tmuxPane != nil && n.cfg.Notifications.Desktop.TmuxPaneFocus {
		// Use -execute to run tmux navigation command when clicked
		args = buildTerminalNotifierArgsWithTmux(title, message, bundleID, tmuxPane)
	} else {
		// Standard behavior: just activate the terminal app
		args = buildTerminalNotifierArgs(title, message, bundleID)
	}

	cmd := exec.Command(notifierPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("terminal-notifier error: %w, output: %s", err, string(output))
	}

	if tmuxPane != nil {
		logging.Debug("terminal-notifier executed: bundleID=%s, tmuxPane=%s", bundleID, tmuxPane.Target)
	} else {
		logging.Debug("terminal-notifier executed: bundleID=%s", bundleID)
	}
	return nil
}

// buildTerminalNotifierArgs constructs command-line arguments for terminal-notifier.
// Exported for testing purposes.
func buildTerminalNotifierArgs(title, message, bundleID string) []string {
	args := []string{
		"-title", title,
		"-message", message,
		"-activate", bundleID,
		// Note: -sender option removed because it conflicts with -activate on macOS Sequoia (15.x)
		// Using -sender causes click-to-focus to stop working.
		// Trade-off: no custom Claude icon, but click-to-focus works reliably.
	}

	// Add group ID to prevent notification stacking issues
	args = append(args, "-group", fmt.Sprintf("claude-notif-%d", time.Now().UnixNano()))

	return args
}

// buildTerminalNotifierArgsWithTmux constructs arguments that include tmux pane navigation.
// When the notification is clicked, it will switch to the exact tmux pane that triggered it.
func buildTerminalNotifierArgsWithTmux(title, message, bundleID string, pane *TmuxPaneInfo) []string {
	focusCmd := BuildTmuxFocusCommand(pane, bundleID)

	args := []string{
		"-title", title,
		"-message", message,
		"-execute", focusCmd,
		// Note: -execute is mutually exclusive with -activate
		// The focus command handles both tmux navigation AND terminal activation
	}

	// Add group ID to prevent notification stacking issues
	args = append(args, "-group", fmt.Sprintf("claude-notif-%d", time.Now().UnixNano()))

	return args
}

// sendWithBeeep sends notification via beeep (cross-platform)
func (n *Notifier) sendWithBeeep(title, message, appIcon, sound string) error {
	// Platform-specific AppName handling:
	// - Windows: Use fixed AppName to prevent registry pollution. Each unique AppName
	//   creates a persistent entry in HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\
	//   CurrentVersion\Notifications\Settings\ that is never cleaned up.
	//   See: https://github.com/777genius/claude-notifications-go/issues/4
	// - macOS/Linux: Use unique AppName to prevent notification grouping/replacement,
	//   allowing multiple notifications to be displayed simultaneously.
	originalAppName := beeep.AppName
	if platform.IsWindows() {
		beeep.AppName = "Claude Code Notifications"
	} else {
		beeep.AppName = fmt.Sprintf("claude-notif-%d", time.Now().UnixNano())
	}
	defer func() {
		beeep.AppName = originalAppName
	}()

	// Send notification using beeep with proper title and clean message
	if err := beeep.Notify(title, message, appIcon); err != nil {
		logging.Error("Failed to send desktop notification: %v", err)
		return err
	}

	logging.Debug("Desktop notification sent via beeep: title=%s", title)

	n.playSoundAsync(sound)
	return nil
}

// playSoundAsync plays sound asynchronously if enabled
func (n *Notifier) playSoundAsync(sound string) {
	if n.cfg.Notifications.Desktop.Sound && sound != "" {
		// Check if notifier is closing to prevent WaitGroup race
		n.mu.Lock()
		if n.closing {
			n.mu.Unlock()
			logging.Debug("Skipping sound playback: notifier is closing")
			return
		}
		n.wg.Add(1)
		n.mu.Unlock()

		// Use SafeGo to protect against panics in sound playback goroutine
		errorhandler.SafeGo(func() {
			defer n.wg.Done()
			n.playSound(sound)
		})
	}
}

// initPlayer initializes the audio player once
func (n *Notifier) initPlayer() error {
	n.playerInit.Do(func() {
		deviceName := n.cfg.Notifications.Desktop.AudioDevice
		volume := n.cfg.Notifications.Desktop.Volume

		player, err := audio.NewPlayer(deviceName, volume)
		if err != nil {
			n.playerErr = err
			logging.Error("Failed to initialize audio player: %v", err)
			return
		}

		n.audioPlayer = player

		if deviceName != "" {
			logging.Debug("Audio player initialized with device: %s, volume: %.0f%%", deviceName, volume*100)
		} else {
			logging.Debug("Audio player initialized with default device, volume: %.0f%%", volume*100)
		}
	})

	return n.playerErr
}

// playSound plays a sound file using the audio module
func (n *Notifier) playSound(soundPath string) {
	if !platform.FileExists(soundPath) {
		logging.Warn("Sound file not found: %s", soundPath)
		return
	}

	// Initialize player once
	if err := n.initPlayer(); err != nil {
		logging.Error("Failed to initialize audio player: %v", err)
		return
	}

	// Play sound
	if err := n.audioPlayer.Play(soundPath); err != nil {
		logging.Error("Failed to play sound %s: %v", soundPath, err)
		return
	}

	volume := n.cfg.Notifications.Desktop.Volume
	logging.Debug("Sound played successfully: %s (volume: %.0f%%)", soundPath, volume*100)
}

// Close waits for all sounds to finish playing and cleans up resources
func (n *Notifier) Close() error {
	// Set closing flag to prevent new sounds from being enqueued
	n.mu.Lock()
	n.closing = true
	n.mu.Unlock()

	// Wait for all sounds to finish
	n.wg.Wait()

	// Close audio player if it was initialized
	n.mu.Lock()
	if n.audioPlayer != nil {
		if err := n.audioPlayer.Close(); err != nil {
			logging.Warn("Failed to close audio player: %v", err)
		}
		n.audioPlayer = nil
		logging.Debug("Audio player closed")
	}
	n.mu.Unlock()

	return nil
}

// extractSessionInfo extracts session name and git branch from message
// Format: "[session-name|branch] message" or "[session-name] message"
// Returns session name, git branch (may be empty), and clean message
func extractSessionInfo(message string) (sessionName, gitBranch, cleanMessage string) {
	message = strings.TrimSpace(message)

	// Check if message starts with [
	if !strings.HasPrefix(message, "[") {
		return "", "", message
	}

	// Find closing bracket
	closingIdx := strings.Index(message, "]")
	if closingIdx == -1 {
		return "", "", message
	}

	// Extract content inside brackets
	bracketContent := message[1:closingIdx]

	// Check if there's a pipe separator for git branch
	if pipeIdx := strings.Index(bracketContent, "|"); pipeIdx != -1 {
		sessionName = bracketContent[:pipeIdx]
		gitBranch = bracketContent[pipeIdx+1:]
	} else {
		sessionName = bracketContent
		gitBranch = ""
	}

	// Extract clean message (everything after "] ")
	cleanMessage = strings.TrimSpace(message[closingIdx+1:])

	return sessionName, gitBranch, cleanMessage
}
