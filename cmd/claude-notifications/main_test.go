package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPluginManifestVersion(t *testing.T) {
	pluginRoot := t.TempDir()
	manifestDir := filepath.Join(pluginRoot, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(`{"version":"9.99.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := readPluginManifestVersion(pluginRoot); got != "9.99.0" {
		t.Fatalf("readPluginManifestVersion() = %q, want %q", got, "9.99.0")
	}
}

func TestMaybeScheduleWindowsLazyUpdateOnMismatch(t *testing.T) {
	pluginRoot := setupLazyUpdateTestPlugin(t, "9.99.0")
	withIsolatedLazyUpdateGlobals(t)

	var called int
	var gotRoot string
	scheduleWindowsLazyUpdate = func(root string) error {
		called++
		gotRoot = root
		return nil
	}

	maybeScheduleWindowsLazyUpdate(pluginRoot)
	maybeScheduleWindowsLazyUpdate(pluginRoot)

	if called != 1 {
		t.Fatalf("scheduleWindowsLazyUpdate called %d times, want 1", called)
	}
	if gotRoot != pluginRoot {
		t.Fatalf("scheduleWindowsLazyUpdate root = %q, want %q", gotRoot, pluginRoot)
	}
}

func TestMaybeScheduleWindowsLazyUpdateSkipsMatchingVersion(t *testing.T) {
	pluginRoot := setupLazyUpdateTestPlugin(t, version)
	withIsolatedLazyUpdateGlobals(t)

	scheduleWindowsLazyUpdate = func(root string) error {
		t.Fatalf("scheduleWindowsLazyUpdate called for matching version at %s", root)
		return nil
	}

	maybeScheduleWindowsLazyUpdate(pluginRoot)
}

func TestMaybeScheduleWindowsLazyUpdateRetriesAfterScheduleFailure(t *testing.T) {
	pluginRoot := setupLazyUpdateTestPlugin(t, "9.99.0")
	withIsolatedLazyUpdateGlobals(t)

	var called int
	scheduleWindowsLazyUpdate = func(root string) error {
		called++
		return errors.New("boom")
	}

	maybeScheduleWindowsLazyUpdate(pluginRoot)
	maybeScheduleWindowsLazyUpdate(pluginRoot)

	if called != 2 {
		t.Fatalf("scheduleWindowsLazyUpdate called %d times, want 2", called)
	}
}

func TestLazyUpdateQuoting(t *testing.T) {
	if got, want := shellSingleQuoted(`C:/Users/O'Brien/bin/install.sh`), `'C:/Users/O'"'"'Brien/bin/install.sh'`; got != want {
		t.Fatalf("shellSingleQuoted() = %q, want %q", got, want)
	}
	if got, want := powershellSingleQuoted(`C:\Users\O'Brien\bash.exe`), `'C:\Users\O''Brien\bash.exe'`; got != want {
		t.Fatalf("powershellSingleQuoted() = %q, want %q", got, want)
	}
}

func TestNewPowerShellHookSetsUTF8OutputEncoding(t *testing.T) {
	hook := newPowerShellHook(`C:\Tools\claude-notifications.exe`, "Stop")

	for _, want := range []string{
		"$OutputEncoding = [System.Text.UTF8Encoding]::new($false)",
		`$input | & "C:\Tools\claude-notifications.exe" handle-hook Stop`,
	} {
		if !strings.Contains(hook.Command, want) {
			t.Fatalf("newPowerShellHook command = %q, want substring %q", hook.Command, want)
		}
	}
	if hook.Shell != "powershell" {
		t.Fatalf("newPowerShellHook shell = %q, want powershell", hook.Shell)
	}
}

func setupLazyUpdateTestPlugin(t *testing.T, pluginVersion string) string {
	t.Helper()

	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugin")
	for _, dir := range []string{
		filepath.Join(pluginRoot, ".claude-plugin"),
		filepath.Join(pluginRoot, "bin"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	manifest := []byte(`{"version":"` + pluginVersion + `"}`)
	if err := os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "bin", "install.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	return pluginRoot
}

func withIsolatedLazyUpdateGlobals(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, ".cache"))
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "LocalAppData"))

	oldGOOS := currentGOOS
	oldSchedule := scheduleWindowsLazyUpdate
	currentGOOS = "windows"

	t.Cleanup(func() {
		currentGOOS = oldGOOS
		scheduleWindowsLazyUpdate = oldSchedule
	})
}
