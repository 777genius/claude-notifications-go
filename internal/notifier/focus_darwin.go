//go:build darwin

package notifier

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// focusQueryTimeout bounds each lsappinfo call so a stalled subprocess can never
// hang notification dispatch on the hook path. A timeout surfaces as an error,
// which terminalHasFocus treats as "unfocused" and delivers the notification.
const focusQueryTimeout = 2 * time.Second

// frontmostBundleID and frontmostTerminalWindowMatches are seams so tests can
// stub the LaunchServices and exact-window queries.
var (
	frontmostBundleID              = defaultFrontmostBundleID
	frontmostTerminalWindowMatches = frontmostTerminalWindowMatchesCWD
)

// terminalHasFocus reports whether our terminal application is frontmost on macOS.
//
// It compares the bundle ID of the frontmost application (resolved through
// LaunchServices via lsappinfo, which - unlike System Events scripting - needs
// no Accessibility permission) against the bundle ID of the terminal running
// Claude Code. This is app-level rather than window-level focus, matching the
// NSWorkspace.frontmostApplication approach: if the terminal app is frontmost
// the user is looking at it.
func terminalHasFocus(sessionID, cwd string) bool {
	front, ok := frontmostBundleID()
	if !ok || front == "" {
		return false
	}
	// GetTerminalBundleID never returns empty: it falls back to com.apple.Terminal
	// when it cannot positively identify the terminal. Trusting that fallback would
	// break the fail-safe contract - an unidentifiable terminal could falsely
	// "match" a frontmost Terminal.app and swallow the notification - so we only
	// compare when the identity is positively known.
	if !terminalIdentityKnown() {
		return false
	}
	ours := GetTerminalBundleID("")
	if ours == "" {
		return false
	}
	if !strings.EqualFold(front, ours) {
		return false
	}

	if isGhosttyBundleID(ours) {
		info, err := ghosttyFrontmostTerminalInfoRunner()
		if err != nil {
			return false
		}
		if storedID := loadStoredGhosttyTerminalID(sessionID); storedID != "" && info.ID != storedID {
			return false
		}
		return ghosttyFrontmostTerminalMatchesSession(info, cwd)
	}

	return frontmostTerminalWindowMatches(ours, cwd)
}

// terminalIdentityKnown reports whether GetTerminalBundleID can positively
// identify the terminal (vs. landing on its com.apple.Terminal fallback). It
// mirrors that function's positive-identification sources.
func terminalIdentityKnown() bool {
	if os.Getenv("__CFBundleIdentifier") != "" {
		return true
	}
	if termProgram := os.Getenv("TERM_PROGRAM"); termProgram != "" {
		if _, ok := terminalBundleIDMap[termProgram]; ok {
			return true
		}
	}
	if IsTmux() && getBundleIDFromTmuxEnv() != "" {
		return true
	}
	return false
}

// defaultFrontmostBundleID returns the bundle ID of the frontmost application via
// LaunchServices. The second return value is false on any failure.
func defaultFrontmostBundleID() (string, bool) {
	// `lsappinfo front` prints the ASN (e.g. "ASN:0x0-0x12345:") of the frontmost
	// app; `lsappinfo info -only bundleid <asn>` then yields `"bundleID"="..."`.
	asnOut, err := runLsappinfo("front")
	if err != nil {
		return "", false
	}
	asn := strings.TrimSpace(string(asnOut))
	if asn == "" {
		return "", false
	}

	infoOut, err := runLsappinfo("info", "-only", "bundleid", asn)
	if err != nil {
		return "", false
	}
	return parseLsappinfoBundleID(string(infoOut))
}

// runLsappinfo runs lsappinfo with focusQueryTimeout so it cannot hang the hook.
func runLsappinfo(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), focusQueryTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "lsappinfo", args...).Output()
}

// parseLsappinfoBundleID extracts the bundle ID from `lsappinfo info -only bundleid`
// output, formatted as `"LSBundleID"="com.apple.Terminal"` (or `bundleID`). It
// returns the value between the final pair of quotes.
func parseLsappinfoBundleID(out string) (string, bool) {
	out = strings.TrimSpace(out)
	eq := strings.Index(out, "=")
	if eq < 0 {
		return "", false
	}
	value := strings.TrimSpace(out[eq+1:])
	value = strings.Trim(value, "\"")
	if value == "" {
		return "", false
	}
	return value, true
}
