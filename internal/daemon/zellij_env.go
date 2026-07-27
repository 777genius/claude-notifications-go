// ABOUTME: Whether this process sits inside a zellij pane, decided from the environment.
// ABOUTME: Untagged so the notifier and the daemon answer the question the same way.
package daemon

import (
	"os"
	"strings"
)

// InZellij reports whether this process is running inside a zellij pane.
//
// $ZELLIJ is zellij's own marker, but it does not always survive: a process
// started outside the pane's shell — a background job, a supervisor, anything
// that rebuilds the environment — can inherit ZELLIJ_SESSION_NAME and
// ZELLIJ_PANE_ID without it. Those two name the pane on their own, which is
// everything a focus request needs, so accept them as sufficient rather than
// discarding a usable target for want of a marker that carries no extra
// information.
func InZellij() bool {
	if os.Getenv("ZELLIJ") != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("ZELLIJ_SESSION_NAME")) != "" &&
		strings.TrimSpace(os.Getenv("ZELLIJ_PANE_ID")) != ""
}
