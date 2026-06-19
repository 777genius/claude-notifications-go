package notifier

// IsTerminalFocused reports whether the terminal window running Claude Code
// currently has operating-system focus.
//
// It is deliberately conservative: it returns true ONLY when it can positively
// confirm that the terminal is the focused window. On any uncertainty — an OS
// API error, an unsupported platform or session (e.g. a Wayland compositor with
// no generic active-window query), or a terminal it cannot identify — it returns
// false so the caller still delivers the notification.
//
// This bias matters: a wrong "focused" result silently swallows a notification
// the user is waiting for, whereas a wrong "unfocused" result merely shows one
// extra banner. The failure mode is therefore always an extra notification,
// never a missing one.
func IsTerminalFocused() bool {
	return terminalHasFocus()
}
