//go:build linux

package daemon

import (
	"os"
	"strings"
	"testing"
)

func TestWriteKWinScript_RendersMatchers(t *testing.T) {
	path, cleanup, err := writeKWinScript("konsole", "my-project")
	if err != nil {
		t.Fatalf("writeKWinScript() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read rendered script: %v", err)
	}
	source := string(body)

	// The class comes from the same mapping kdotool uses, lowercased for the
	// case-insensitive compare the script does.
	if want := strings.ToLower(GetKdotoolClass("konsole")); !strings.Contains(source, want) {
		t.Errorf("rendered script is missing the window class %q", want)
	}
	if !strings.Contains(source, "my-project") {
		t.Error("rendered script is missing the folder name used for caption matching")
	}
	if !strings.Contains(source, kwinReplyService) {
		t.Error("rendered script does not call back, so a failed match would look like success")
	}
	// Plasma 5 compatibility: both API generations must be handled.
	for _, api := range []string{"windowList", "clientList", "activeWindow", "activeClient"} {
		if !strings.Contains(source, api) {
			t.Errorf("rendered script does not handle %s", api)
		}
	}
}

func TestWriteKWinScript_EscapesFolderName(t *testing.T) {
	// loadScript takes a path, so the folder name reaches the compositor as JS source
	// and has to survive quoting the same way the GNOME Shell Eval path does.
	path, cleanup, err := writeKWinScript("konsole", `evil'); workspace.slotWindowClose(); ('`)
	if err != nil {
		t.Fatalf("writeKWinScript() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read rendered script: %v", err)
	}

	if strings.Contains(string(body), `evil'); workspace`) {
		t.Errorf("folder name was interpolated unescaped:\n%s", body)
	}
	if !strings.Contains(string(body), `\'`) {
		t.Error("expected the quote in the folder name to be escaped")
	}
}

func TestWriteKWinScript_CleanupRemovesFile(t *testing.T) {
	path, cleanup, err := writeKWinScript("konsole", "")
	if err != nil {
		t.Fatalf("writeKWinScript() error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("script file should exist before cleanup: %v", err)
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup should remove the script file, stat gave: %v", err)
	}
}

// TestGetFocusMethods_KWinBeforeKdotool pins the ordering. kdotool drives the same
// KWin interface, so trying it first would make the fallback depend on a binary the
// user has to install for no added capability.
func TestGetFocusMethods_KWinBeforeKdotool(t *testing.T) {
	var kwin, kdotool = -1, -1
	for i, m := range GetFocusMethods() {
		switch m.Name {
		case "KWin script":
			kwin = i
		case "kdotool":
			kdotool = i
		}
	}

	if kwin < 0 {
		t.Fatal("KWin script is not in the focus method list")
	}
	if kdotool < 0 {
		t.Fatal("kdotool is not in the focus method list")
	}
	if kwin > kdotool {
		t.Errorf("KWin script runs at %d, after kdotool at %d", kwin, kdotool)
	}
}
