package notifier

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// isTimeSensitiveStatus returns true for statuses that should break through Focus Mode
func isTimeSensitiveStatus(status analyzer.Status) bool {
	switch status {
	case analyzer.StatusAPIError, analyzer.StatusAPIErrorOverloaded, analyzer.StatusSessionLimitReached:
		return true
	default:
		return false
	}
}

// SendDesktop sends a desktop notification using beeep (cross-platform)
// On macOS with clickToFocus enabled, uses terminal-notifier for click-to-focus support
// On Linux with clickToFocus enabled, uses background daemon for click-to-focus support
// cwd is the working directory of the project; used for window-specific focus. May be empty.
func (n *Notifier) SendDesktop(status analyzer.Status, message, sessionID, cwd string) error {
	// Send terminal bell for terminal tab indicators (e.g. Ghostty, tmux)
	if n.cfg.IsTerminalBellEnabled() {
		sendTerminalBell()
	}

	if !n.cfg.IsDesktopEnabled() {
		logging.Debug("Desktop notifications disabled, skipping")
		return nil
	}

	statusInfo, exists := n.cfg.GetStatusInfo(string(status))
	if !exists {
		return fmt.Errorf("unknown status: %s", status)
	}

	// Extract session name, git branch and folder name from message
	// Format: "[session-name|branch folder] actual message" or "[session-name folder] actual message"
	sessionName, gitBranch, cleanMessage := extractSessionInfo(message)

	// Build clean title (status only + session name)
	// Format: "✅ Completed [peak]" or "✅ Completed"
	title := statusInfo.Title
	if sessionName != "" {
		title = fmt.Sprintf("%s [%s]", title, sessionName)
	}

	// Build subtitle from branch and folder name
	// Format: "main · notification_plugin_go" or just folder name
	var subtitle string
	if gitBranch != "" {
		// gitBranch may contain "branch folder" (space-separated from hooks.go format)
		parts := strings.SplitN(gitBranch, " ", 2)
		if len(parts) == 2 {
			subtitle = fmt.Sprintf("%s \u00B7 %s", parts[0], parts[1])
		} else {
			subtitle = gitBranch
		}
	}

	timeSensitive := isTimeSensitiveStatus(status)

	// Get app icon path if configured
	appIcon := n.cfg.Notifications.Desktop.AppIcon
	if appIcon != "" && !platform.FileExists(appIcon) {
		logging.Warn("App icon not found: %s, using default", appIcon)
		appIcon = ""
	}

	// macOS: Try terminal-notifier for click-to-focus support
	if platform.IsMacOS() && n.cfg.Notifications.Desktop.ClickToFocus {
		if IsTerminalNotifierAvailable() {
			if err := n.sendWithTerminalNotifier(title, cleanMessage, subtitle, sessionID, timeSensitive, cwd); err != nil {
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

	// Linux: Try daemon for click-to-focus support
	if platform.IsLinux() && n.cfg.Notifications.Desktop.ClickToFocus {
		if err := sendLinuxNotification(title, cleanMessage, appIcon, n.cfg, cwd); err != nil {
			logging.Warn("Linux daemon notification failed, falling back to beeep: %v", err)
			// Fall through to beeep
		} else {
			logging.Debug("Desktop notification sent via Linux daemon: title=%s", title)
			n.playSoundAsync(statusInfo.Sound)
			return nil
		}
	}

	// Standard path: beeep (Windows, macOS fallback, Linux fallback)
	return n.sendWithBeeep(title, cleanMessage, appIcon, statusInfo.Sound)
}

// sendWithTerminalNotifier sends notification via terminal-notifier on macOS
// with click-to-focus support (clicking notification activates the terminal)
func (n *Notifier) sendWithTerminalNotifier(title, message, subtitle, sessionID string, timeSensitive bool, cwd string) error {
	notifierPath, err := GetTerminalNotifierPath()
	if err != nil {
		return fmt.Errorf("terminal-notifier not found: %w", err)
	}

	bundleID := GetTerminalBundleID(n.cfg.Notifications.Desktop.TerminalBundleID)

	var args []string
	if muxArgs, muxName := detectMultiplexerArgs(title, message, bundleID); muxArgs != nil {
		args = muxArgs
		logging.Debug("%s detected, using multiplexer-specific -execute", muxName)
	} else {
		if muxName != "" {
			logging.Debug("%s detected but target capture failed, falling back to -activate", muxName)
		}
		args = buildTerminalNotifierArgs(title, message, bundleID, cwd)
	}

	// Append shared options: subtitle, threadID, timeSensitive, nosound
	if subtitle != "" {
		args = append(args, "-subtitle", subtitle)
	}
	if sessionID != "" {
		args = append(args, "-threadID", sessionID)
	}
	if timeSensitive {
		args = append(args, "-timeSensitive")
	}
	// Always suppress sound in Swift — Go manages sound via audio player
	args = append(args, "-nosound")

	cmd := exec.Command(notifierPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("terminal-notifier error: %w, output: %s", err, string(output))
	}

	logging.Debug("terminal-notifier executed: bundleID=%s", bundleID)
	return nil
}

// buildTerminalNotifierArgs constructs command-line arguments for terminal-notifier.
// When cwd is provided, uses -execute with a focus script instead of -activate.
// Exported for testing purposes.
func buildTerminalNotifierArgs(title, message, bundleID, cwd string) []string {
	args := []string{
		"-title", title,
		"-message", message,
	}

	// Note: -sender option removed because it conflicts with -activate on macOS Sequoia (15.x)
	// Using -sender causes click-to-focus to stop working.
	if cwd != "" {
		if script := buildFocusScript(bundleID, cwd); script != "" {
			args = append(args, "-execute", script)
		} else {
			args = append(args, "-activate", bundleID)
		}
	} else {
		args = append(args, "-activate", bundleID)
	}

	// Add group ID to prevent notification stacking issues
	args = append(args, "-group", fmt.Sprintf("claude-notif-%d", time.Now().UnixNano()))

	return args
}

// buildFocusScript returns the shell command for -execute in terminal-notifier.
// For Ghostty: uses AXDocument attribute (OSC 7 CWD) via Accessibility API,
// falling back to plain app activation.
// For VS Code: invokes the binary's focus-window subcommand (CGo AXUIElement).
// For all other apps: uses AppleScript title search by folder name.
// Returns "" when cwd is empty or unusable (caller should use -activate instead).
func buildFocusScript(bundleID, cwd string) string {
	if cwd == "" {
		return ""
	}

	if isGhosttyBundleID(bundleID) {
		return buildGhosttyFocusScript(bundleID, cwd)
	}

	folderName := filepath.Base(cwd)
	if folderName == "" || folderName == "." || folderName == string(filepath.Separator) {
		return ""
	}

	if isVSCodeBundleID(bundleID) {
		// VS Code's AppleScript dictionary doesn't support window enumeration
		// (-1708), so AppleScript is not a viable fallback. Return "" to use
		// plain -activate if the binary path is unavailable.
		return buildVSCodeFocusScript(bundleID, cwd)
	}

	return buildAppleScriptFocusScript(bundleID, folderName)
}

// isVSCodeBundleID reports whether bundleID is VS Code or VS Code Insiders.
func isVSCodeBundleID(bundleID string) bool {
	return bundleID == "com.microsoft.VSCode" ||
		bundleID == "com.microsoft.VSCodeInsiders"
}

// isGhosttyBundleID reports whether bundleID is Ghostty.
func isGhosttyBundleID(bundleID string) bool {
	return bundleID == "com.mitchellh.ghostty"
}

// shellQuote wraps s in single quotes, escaping internal single quotes
// using the '\” technique (end quote, literal apostrophe, resume quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildVSCodeFocusScript builds the -execute script for VS Code.
// Invokes the binary's focus-window subcommand which activates VS Code,
// waits for AXWindows to populate, then raises the window matching cwd.
// Returns "" (causing -activate fallback) if os.Executable() fails.
func buildVSCodeFocusScript(bundleID, cwd string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return shellQuote(exe) + " focus-window " + shellQuote(bundleID) + " " + shellQuote(cwd)
}

// buildGhosttyFocusScript builds the -execute script for Ghostty.
// Invokes the binary's focus-window subcommand which activates Ghostty,
// waits for AXWindows to populate, then raises the window matching cwd via
// AXDocument (OSC 7 file:// URL). AXDocument is window-level only; tabs and
// split panes within a window are not individually addressable.
// Returns "" (causing -activate fallback) if cwd is empty or os.Executable() fails.
func buildGhosttyFocusScript(bundleID, cwd string) string {
	if cwd == "" {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return shellQuote(exe) + " focus-window " + shellQuote(bundleID) + " " + shellQuote(cwd)
}

// cwdToFileURL converts an absolute path to a file:// URL. Ghostty exposes the
// window CWD (set via OSC 7) as a file:// URL in the AXDocument attribute.
// Encodes spaces as %20 to match the format Ghostty uses.
func cwdToFileURL(cwd string) string {
	encoded := strings.NewReplacer(
		" ", "%20",
		"#", "%23",
		"?", "%3F",
		"%", "%25",
	).Replace(cwd)
	return "file://" + encoded + "/"
}

// buildAppleScriptFocusScript builds the -execute script that activates an app
// and raises the first window whose title contains folderName as a distinct
// component (delimited by " — " or " - ", or exact match). This avoids false
// matches like "app" matching "my-app".
func buildAppleScriptFocusScript(bundleID, folderName string) string {
	safeBundleID := sanitizeForAppleScript(bundleID)
	safeFolder := sanitizeForAppleScript(folderName)
	// AppleScript: check if name contains " — folder" or "folder — " or equals folder.
	// This matches titles like "file — folder — App" without substring false positives.
	return fmt.Sprintf(
		`osascript -e 'tell application id "%s"' -e 'activate' -e 'set _n to "%s"' -e 'set _d1 to " \u2014 " & _n' -e 'set _d2 to _n & " \u2014 "' -e 'set _d3 to " - " & _n' -e 'set _d4 to _n & " - "' -e 'repeat with w in windows' -e 'set _t to name of w' -e 'if _t = _n or _t contains _d1 or _t contains _d2 or _t contains _d3 or _t contains _d4 then' -e 'set index of w to 1' -e 'exit repeat' -e 'end if' -e 'end repeat' -e 'end tell'`,
		safeBundleID, safeFolder,
	)
}

// sanitizeForAppleScript escapes characters that would break AppleScript string
// literals or shell single-quote delimiters when embedded in a -execute command.
// Single quotes use the shell end-quote/apostrophe/resume-quote technique.
// Double quotes and backslashes are backslash-escaped for AppleScript.
func sanitizeForAppleScript(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\'':
			b.WriteString(`'\''`)
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SendQuickNotification sends a one-off notification without requiring a
// Notifier instance. Fallback chain: terminal-notifier → osascript.
// executeCmd is the shell command run when the user clicks the notification (may be empty).
func SendQuickNotification(title, message, executeCmd string) error {
	if notifierPath, err := GetTerminalNotifierPath(); err == nil {
		args := []string{
			"-title", title,
			"-message", message,
		}
		if executeCmd != "" {
			args = append(args, "-execute", executeCmd)
		}
		args = append(args,
			"-group", fmt.Sprintf("claude-quick-%d", time.Now().UnixNano()),
			"-nosound",
		)
		if output, err := exec.Command(notifierPath, args...).CombinedOutput(); err == nil {
			return nil
		} else {
			logging.Debug("terminal-notifier failed: %v, output: %s", err, string(output))
		}
	}

	// Fallback: osascript (no click action, just informational)
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return fmt.Errorf("all notification methods failed: %w", err)
	}
	return nil
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

// sendTerminalBell writes a BEL character to /dev/tty to trigger terminal
// tab indicators (e.g. Ghostty tab highlight, tmux window bell flag).
func sendTerminalBell() {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		logging.Debug("Could not open /dev/tty for bell: %v", err)
		return
	}
	defer f.Close()
	_, _ = f.Write([]byte("\a"))
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
