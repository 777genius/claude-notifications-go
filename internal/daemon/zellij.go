//go:build linux

// ABOUTME: Zellij pane focus for Linux click-to-focus.
// ABOUTME: Targets the exact pane that produced the notification, across tabs.
package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// zellijActionTimeout bounds the CLI call. It runs on the D-Bus action callback,
// so a zellij server that stops answering would otherwise wedge the daemon's
// click handling for every later notification, not just this one.
const zellijActionTimeout = 5 * time.Second

// GetZellijFocusHints returns the zellij session and pane that produced the
// notification, or empty strings when not running under zellij.
//
// The pane is read from the environment rather than asked of zellij when the
// click arrives, because by then "the focused pane" is whatever the user is
// looking at — precisely the pane they are not trying to get back to.
func GetZellijFocusHints() (sessionName, paneID string) {
	if !InZellij() {
		return "", ""
	}
	return strings.TrimSpace(os.Getenv("ZELLIJ_SESSION_NAME")),
		strings.TrimSpace(os.Getenv("ZELLIJ_PANE_ID"))
}

// TryZellijPane focuses paneID inside sessionName, switching tabs if the pane
// lives in one that is not current.
//
// focus-pane-id arrived in zellij 0.44.1; older versions reject the subcommand
// and the caller degrades to a raised window with no pane switch.
func TryZellijPane(sessionName, paneID string) error {
	if sessionName == "" || paneID == "" {
		return fmt.Errorf("zellij session name or pane id missing")
	}
	if _, err := exec.LookPath("zellij"); err != nil {
		return fmt.Errorf("zellij not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), zellijActionTimeout)
	defer cancel()

	// -s names the session explicitly. The daemon is spawned by whichever hook
	// first needed it and then keeps that session's ZELLIJ_* variables for the
	// rest of its life, so letting zellij infer the session from the environment
	// would focus a pane in whichever session happened to start the daemon.
	cmd := exec.CommandContext(ctx, "zellij", "-s", sessionName, "action", "focus-pane-id", paneID)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if isZellijAlreadyFocused(string(output)) {
		return nil
	}
	return fmt.Errorf("zellij focus-pane-id failed: %w, output: %s", err, strings.TrimSpace(string(output)))
}

// isZellijAlreadyFocused reports whether zellij failed only because the pane was
// already focused, which is the outcome we wanted.
//
// This has to match the message rather than the exit status: zellij exits 2 both
// when the pane is already focused and when the pane does not exist at all.
func isZellijAlreadyFocused(output string) bool {
	return strings.Contains(strings.ToLower(output), "already focused")
}
