package notifier

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// IsZellij returns true if the current process is running inside a zellij session.
func IsZellij() bool {
	return os.Getenv("ZELLIJ") != ""
}

// getZellijPath returns the absolute path to the zellij binary.
// ClaudeNotifier.app runs without the user's PATH, so we need the full path.
func getZellijPath() string {
	if path, err := exec.LookPath("zellij"); err == nil {
		return path
	}
	return "zellij"
}

// GetZellijTabTarget returns the active tab name and session name for the current zellij session.
func GetZellijTabTarget() (tabName, sessionName string, err error) {
	sessionName = os.Getenv("ZELLIJ_SESSION_NAME")
	if sessionName == "" {
		return "", "", fmt.Errorf("ZELLIJ_SESSION_NAME not set")
	}

	zellijPath := getZellijPath()

	cmd := exec.Command(zellijPath, "action", "dump-layout")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to run zellij dump-layout: %w", err)
	}

	tabName = parseActiveTabName(string(output))
	if tabName == "" {
		return "", "", fmt.Errorf("could not determine active tab from zellij layout")
	}

	return tabName, sessionName, nil
}

// parseActiveTabName extracts the name of the focused tab from zellij dump-layout output (KDL format).
// It looks for lines matching: tab ... name="..." ... focus=true
func parseActiveTabName(layout string) string {
	for _, line := range strings.Split(layout, "\n") {
		trimmed := strings.TrimSpace(line)

		// Must be a top-level "tab" line (not "pane" or nested content)
		if !strings.HasPrefix(trimmed, "tab ") {
			continue
		}

		// Must have focus=true
		if !strings.Contains(trimmed, "focus=true") {
			continue
		}

		// Extract name="..."
		if name := extractKDLStringAttr(trimmed, "name"); name != "" {
			return name
		}
	}
	return ""
}

// extractKDLStringAttr extracts the value of a key="value" attribute from a KDL line.
func extractKDLStringAttr(line, key string) string {
	// Search for key="
	search := key + `="`
	idx := strings.Index(line, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	// Find closing quote
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}

// buildZellijNotifierArgs constructs command-line arguments for terminal-notifier
// when running inside zellij. Uses -activate (to focus the terminal app)
// and -execute (to switch to the correct zellij tab) on click.
func buildZellijNotifierArgs(title, message, tabName, sessionName, bundleID string) []string {
	return buildZellijActionNotifierArgs(title, message, sessionName, bundleID, "go-to-tab-name", tabName)
}

// buildZellijActionNotifierArgs assembles the terminal-notifier invocation whose
// -execute runs a single `zellij action` against an explicitly named session.
func buildZellijActionNotifierArgs(title, message, sessionName, bundleID, action, target string) []string {
	zellijPath := getZellijPath()

	executeCmd := fmt.Sprintf(
		"'%s' -s '%s' action %s '%s'",
		zellijPath, sessionName, action, target,
	)

	args := []string{
		"-title", title,
		"-message", message,
		"-activate", bundleID,
		"-execute", executeCmd,
	}

	// Add group ID to prevent notification stacking issues
	args = append(args, "-group", fmt.Sprintf("claude-notif-%d", time.Now().UnixNano()))

	return args
}
