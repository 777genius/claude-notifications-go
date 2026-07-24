//go:build windows

package winfocus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFocusHandlerExecutablePrefersSibling(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "claude-notifications-windows-amd64.exe")
	sibling := filepath.Join(dir, "claude-notifications-windows-amd64-focus.exe")

	if err := os.WriteFile(sibling, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	if got := focusHandlerExecutable(exe); got != sibling {
		t.Errorf("focusHandlerExecutable(%q) = %q, want %q", exe, got, sibling)
	}
}

func TestFocusHandlerExecutableFallsBackWhenSiblingMissing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "claude-notifications-windows-amd64.exe")

	if got := focusHandlerExecutable(exe); got != exe {
		t.Errorf("focusHandlerExecutable(%q) = %q, want %q (fallback to self)", exe, got, exe)
	}
}

func TestWindowForPIDPrefersFolderMatchOverForegroundHint(t *testing.T) {
	// Two windows share one PID (e.g. Windows Terminal's shared "monarch"
	// process hosting one window per project). The foreground hint points at
	// the wrong project's window; the folder-title match must win anyway.
	const sharedPID = 42
	list := []winInfo{
		{hwnd: 1, pid: sharedPID, title: "project-b - Windows Terminal"},
		{hwnd: 2, pid: sharedPID, title: "project-a - Windows Terminal"},
	}

	got := windowForPID(list, sharedPID, "project-a", 1 /* fg hint wrongly points at window 1 */)
	if got != 2 {
		t.Errorf("windowForPID = %d, want 2 (folder match should beat the foreground hint)", got)
	}
}

func TestWindowForPIDFallsBackToHintWhenNoFolderMatch(t *testing.T) {
	const pid = 7
	list := []winInfo{
		{hwnd: 10, pid: pid, title: "unrelated title"},
		{hwnd: 11, pid: pid, title: "also unrelated"},
	}

	got := windowForPID(list, pid, "project-a", 11)
	if got != 11 {
		t.Errorf("windowForPID = %d, want 11 (hint) when no window title matches the folder", got)
	}
}

func TestWindowForPIDFallsBackToFirstTitledWindow(t *testing.T) {
	const pid = 9
	list := []winInfo{
		{hwnd: 20, pid: pid, title: ""},
		{hwnd: 21, pid: pid, title: "some title"},
	}

	got := windowForPID(list, pid, "", 0)
	if got != 21 {
		t.Errorf("windowForPID = %d, want 21 (first titled window)", got)
	}
}
