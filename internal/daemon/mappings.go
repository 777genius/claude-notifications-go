package daemon

import (
	"os"
	"strings"
)

// escapeJS escapes a string for safe interpolation into JavaScript single-quoted strings.
// Prevents JS injection when values are passed to GNOME Shell.Eval.
func escapeJS(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\x00", `\x00`,
		"\u2028", `\u2028`,
		"\u2029", `\u2029`,
	)
	return r.Replace(s)
}

// GetAppID returns the .desktop app ID for a terminal name.
func GetAppID(terminalName string) string {
	switch strings.ToLower(terminalName) {
	case "code", "vscode", "visual studio code":
		return "code.desktop"
	case "gnome-terminal":
		return "org.gnome.Terminal.desktop"
	case "konsole":
		return "org.kde.konsole.desktop"
	case "alacritty":
		return "Alacritty.desktop"
	case "kitty":
		return "kitty.desktop"
	case "wezterm":
		return "org.wezfurlong.wezterm.desktop"
	case "tilix":
		return "com.gexperts.Tilix.desktop"
	case "terminator":
		return "terminator.desktop"
	default:
		return strings.ToLower(terminalName) + ".desktop"
	}
}

// GetWlrctlAppID returns the wlroots app_id for a terminal name.
func GetWlrctlAppID(terminalName string) string {
	switch strings.ToLower(terminalName) {
	case "code", "vscode", "visual studio code":
		return "code"
	case "alacritty":
		return "Alacritty"
	case "kitty":
		return "kitty"
	case "wezterm":
		return "org.wezfurlong.wezterm"
	case "gnome-terminal":
		return "org.gnome.Terminal"
	case "konsole":
		return "org.kde.konsole"
	default:
		return strings.ToLower(terminalName)
	}
}

// GetKdotoolClass returns the window class for kdotool search.
func GetKdotoolClass(terminalName string) string {
	switch strings.ToLower(terminalName) {
	case "code", "vscode", "visual studio code":
		return "code"
	case "alacritty":
		return "Alacritty"
	case "kitty":
		return "kitty"
	case "wezterm":
		return "org.wezfurlong.wezterm"
	case "gnome-terminal":
		return "gnome-terminal-server"
	case "konsole":
		return "konsole"
	default:
		return strings.ToLower(terminalName)
	}
}

// GetXdotoolClass returns the X11 WM_CLASS for xdotool search.
func GetXdotoolClass(terminalName string) string {
	switch strings.ToLower(terminalName) {
	case "code", "vscode", "visual studio code":
		return "Code"
	case "alacritty":
		return "Alacritty"
	case "kitty":
		return "kitty"
	case "wezterm":
		return "org.wezfurlong.wezterm"
	case "gnome-terminal":
		return "Gnome-terminal"
	case "konsole":
		return "konsole"
	case "xfce4-terminal":
		return "Xfce4-terminal"
	case "mate-terminal":
		return "Mate-terminal"
	case "tilix":
		return "Tilix"
	case "terminator":
		return "Terminator"
	default:
		return terminalName
	}
}

// GetSearchTerm returns a window title search term for a terminal name.
func GetSearchTerm(terminalName string) string {
	switch strings.ToLower(terminalName) {
	case "code", "vscode", "visual studio code":
		return "Visual Studio Code"
	case "gnome-terminal":
		return "Terminal"
	default:
		return terminalName
	}
}

// GetTerminalName detects the current terminal from environment variables.
func GetTerminalName() string {
	// Try TERM_PROGRAM first (set by many terminals)
	if termProg := os.Getenv("TERM_PROGRAM"); termProg != "" {
		return termProg
	}

	// Check VS Code indicators
	if os.Getenv("VSCODE_INJECTION") != "" || os.Getenv("VSCODE_GIT_IPC_HANDLE") != "" {
		return "Code"
	}

	// Check GNOME Terminal indicators
	if os.Getenv("GNOME_TERMINAL_SCREEN") != "" || os.Getenv("GNOME_TERMINAL_SERVICE") != "" {
		return "gnome-terminal"
	}

	// Fallback to generic terminal
	return "Terminal"
}
