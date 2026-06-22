//go:build !windows && !darwin && !linux

package notifier

// terminalHasFocus is unsupported on this platform, so focus is reported as
// unknown (false) and notifications are always delivered.
func terminalHasFocus(_, _ string) bool {
	return false
}
