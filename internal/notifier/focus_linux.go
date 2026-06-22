//go:build linux

package notifier

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// activeWindowQueryTimeout bounds the xdotool call so a stalled subprocess can
// never hang notification dispatch on the hook path. A timeout surfaces as an
// error, which terminalHasFocus treats as "unfocused" and delivers the notification.
const activeWindowQueryTimeout = 2 * time.Second

// activeWindowID is a seam so tests can stub the X11 active-window query.
var activeWindowID = defaultActiveWindowID

// terminalHasFocus reports whether the terminal window is the X11 active window.
//
// It compares the window manager's active window (_NET_ACTIVE_WINDOW, read via
// xdotool) against $WINDOWID, which X11 terminals export for their own window.
// When $WINDOWID is unset - typically under Wayland, where there is no portable
// active-window query - focus is treated as unknown and the notification is
// delivered. Class-based matching is intentionally avoided: two terminal windows
// share a class, so it cannot tell "the window Claude runs in" from "another
// terminal", and a false match would swallow the notification.
func terminalHasFocus(_, _ string) bool {
	ours, ok := parseWindowID(os.Getenv("WINDOWID"))
	if !ok {
		return false // Wayland, or a terminal that does not export WINDOWID
	}
	activeRaw, err := activeWindowID()
	if err != nil {
		return false
	}
	active, ok := parseWindowID(activeRaw)
	if !ok {
		return false
	}
	return ours == active
}

// defaultActiveWindowID returns the X11 active window ID via xdotool, bounded by
// activeWindowQueryTimeout so a stalled subprocess cannot hang the hook.
func defaultActiveWindowID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), activeWindowQueryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xdotool", "getactivewindow").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// parseWindowID normalizes an X11 window ID (decimal from $WINDOWID, or
// hexadecimal like 0x1e00007 from some tools) to a comparable integer. It
// returns false when the input is empty or not a valid number.
func parseWindowID(raw string) (uint64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 0, 64) // base 0 honors a 0x prefix
	if err != nil {
		return 0, false
	}
	return id, true
}
