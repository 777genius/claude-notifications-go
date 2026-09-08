package codexsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBundle builds a minimal plugin bundle that Run accepts.
func fakeBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sounds"), 0o755); err != nil {
		t.Fatalf("mkdir sounds: %v", err)
	}
	files := map[string]string{
		"bin/codex-hook-wrapper.sh":  "#!/bin/sh\nexit 0\n",
		"bin/codex-hook-wrapper.cmd": "@echo off\r\nexit /b 0\r\n",
		"bin/hook-wrapper.sh":        "#!/bin/sh\nexit 0\n",
		"sounds/task-complete.mp3":   "not-really-audio",
		"config/config.json":         `{"notifications":{}}`,
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// Source-only directories that must NOT be copied into the install dir.
	if err := os.MkdirAll(filepath.Join(root, "internal", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "hooks", "hooks.go"), []byte("package hooks"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return root
}

func readHooks(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	return parsed
}

func TestRunCreatesRegistrationAndInstallCopy(t *testing.T) {
	bundle := fakeBundle(t)
	codexHome := t.TempDir()

	res, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Install copy carries runtime assets but not sources.
	for _, rel := range []string{"bin/codex-hook-wrapper.sh", "bin/codex-hook-wrapper.cmd", "sounds/task-complete.mp3"} {
		if _, err := os.Stat(filepath.Join(res.InstallDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("install copy missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(res.InstallDir, "internal")); !os.IsNotExist(err) {
		t.Errorf("install copy must not contain sources (err=%v)", err)
	}

	parsed := readHooks(t, res.HooksPath)
	hooks, ok := parsed["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks section missing: %v", parsed)
	}
	for _, event := range []string{"Stop", "SubagentStop", "PermissionRequest", "PreToolUse"} {
		if _, ok := hooks[event]; !ok {
			t.Errorf("event %s not registered", event)
		}
	}

	// The command must point at the stable install dir, never at the source
	// bundle: the trust hash covers the command string.
	raw, _ := json.Marshal(parsed)
	if strings.Contains(string(raw), bundle) {
		t.Error("hook command references the source bundle path instead of the stable install dir")
	}
	if !strings.Contains(string(raw), res.InstallDir) {
		t.Error("hook command does not reference the stable install dir")
	}
}

func TestRunIsIdempotentAndStable(t *testing.T) {
	bundle := fakeBundle(t)
	codexHome := t.TempDir()

	first, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle})
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	firstData, err := os.ReadFile(first.HooksPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	second, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle})
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	secondData, err := os.ReadFile(second.HooksPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Byte-identical output keeps the Codex trust hash valid across re-runs.
	if string(firstData) != string(secondData) {
		t.Fatalf("re-running setup changed hooks.json:\nfirst:\n%s\nsecond:\n%s", firstData, secondData)
	}
	if !second.Replaced {
		t.Error("second run should report replacing the previous registration")
	}

	parsed := readHooks(t, second.HooksPath)
	hooks := parsed["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop has %d groups after re-run, want 1 (no duplicates)", len(stop))
	}
}

func TestRunPreservesForeignHooks(t *testing.T) {
	bundle := fakeBundle(t)
	codexHome := t.TempDir()
	hooksPath := filepath.Join(codexHome, "hooks.json")

	existing := `{
  "description": "my setup",
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "/usr/local/bin/my-own-tool", "timeout": 5, "customKey": true}]}],
    "SessionEnd": [{"hooks": [{"type": "command", "command": "echo bye"}]}]
  }
}`
	if err := os.WriteFile(hooksPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("seed hooks: %v", err)
	}

	res, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.ForeignKept != 2 {
		t.Errorf("ForeignKept = %d, want 2", res.ForeignKept)
	}
	if res.BackupPath == "" {
		t.Error("existing file must be backed up")
	}
	if backup, err := os.ReadFile(res.BackupPath); err != nil || string(backup) != existing {
		t.Errorf("backup does not match the original file (err=%v)", err)
	}

	parsed := readHooks(t, hooksPath)
	if parsed["description"] != "my setup" {
		t.Errorf("unknown top-level key lost: %v", parsed["description"])
	}
	hooks := parsed["hooks"].(map[string]any)
	if _, ok := hooks["SessionEnd"]; !ok {
		t.Error("foreign event SessionEnd was dropped")
	}

	stop := hooks["Stop"].([]any)
	if len(stop) != 2 {
		t.Fatalf("Stop has %d groups, want 2 (foreign + ours)", len(stop))
	}
	foreign := stop[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if foreign["command"] != "/usr/local/bin/my-own-tool" {
		t.Errorf("foreign handler mangled: %v", foreign)
	}
	if foreign["customKey"] != true {
		t.Errorf("unknown handler key lost: %v", foreign)
	}
}

func TestRunRefusesCorruptedHooksFile(t *testing.T) {
	bundle := fakeBundle(t)
	codexHome := t.TempDir()
	hooksPath := filepath.Join(codexHome, "hooks.json")
	corrupted := "{not json"
	if err := os.WriteFile(hooksPath, []byte(corrupted), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle}); err == nil {
		t.Fatal("expected an error instead of overwriting an unparseable file")
	}
	after, err := os.ReadFile(hooksPath)
	if err != nil || string(after) != corrupted {
		t.Fatalf("the user's file was modified: %q (err=%v)", after, err)
	}
}

func TestRunRejectsNonBundle(t *testing.T) {
	if _, err := Run(Options{CodexHome: t.TempDir(), PluginRoot: t.TempDir()}); err == nil {
		t.Fatal("expected an error for a directory that is not a plugin bundle")
	}
	if _, err := Run(Options{CodexHome: t.TempDir()}); err == nil {
		t.Fatal("expected an error for an empty plugin root")
	}
}

func TestRunRejectsInstallingOntoItself(t *testing.T) {
	codexHome := t.TempDir()
	installDir := filepath.Join(codexHome, InstallDirName)
	if err := os.MkdirAll(filepath.Join(installDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "bin", "codex-hook-wrapper.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Run(Options{CodexHome: codexHome, PluginRoot: installDir}); err == nil {
		t.Fatal("expected an error when the source and destination are the same directory")
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	bundle := fakeBundle(t)
	codexHome := t.TempDir()

	res, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle, DryRun: true})
	if err != nil {
		t.Fatalf("Run(dry) error = %v", err)
	}
	if _, err := os.Stat(res.HooksPath); !os.IsNotExist(err) {
		t.Errorf("dry run created %s", res.HooksPath)
	}
	if _, err := os.Stat(res.InstallDir); !os.IsNotExist(err) {
		t.Errorf("dry run created %s", res.InstallDir)
	}
}

func TestHookCommandsCarryBothPlatforms(t *testing.T) {
	posix, windows := HookCommands(filepath.Join("/home/u/.codex", InstallDirName), "Stop")
	if !strings.HasPrefix(posix, "sh ") || !strings.Contains(posix, "codex-hook-wrapper.sh") ||
		!strings.HasSuffix(posix, "handle-hook Stop --product codex") {
		t.Errorf("posix command = %q", posix)
	}
	if !strings.HasPrefix(windows, "cmd.exe /d /s /c call ") || !strings.Contains(windows, "codex-hook-wrapper.cmd") {
		t.Errorf("windows command = %q", windows)
	}
}

func TestResolveCodexHomePrecedence(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join("/tmp", "env-codex"))
	got, err := ResolveCodexHome("/tmp/explicit")
	if err != nil || got != filepath.Clean("/tmp/explicit") {
		t.Fatalf("explicit override = %q, %v", got, err)
	}
	got, err = ResolveCodexHome("")
	if err != nil || got != filepath.Clean("/tmp/env-codex") {
		t.Fatalf("env override = %q, %v", got, err)
	}

	t.Setenv("CODEX_HOME", "")
	got, err = ResolveCodexHome("")
	if err != nil {
		t.Fatalf("home fallback error = %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".codex")) {
		t.Fatalf("home fallback = %q, want a path ending in .codex", got)
	}
}

func TestRenderHooksJSONMatchesWrittenFile(t *testing.T) {
	bundle := fakeBundle(t)
	codexHome := t.TempDir()

	res, err := Run(Options{CodexHome: codexHome, PluginRoot: bundle})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	written, err := os.ReadFile(res.HooksPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	rendered, err := RenderHooksJSON(res.InstallDir)
	if err != nil {
		t.Fatalf("RenderHooksJSON() error = %v", err)
	}
	if strings.TrimSpace(string(written)) != strings.TrimSpace(string(rendered)) {
		t.Errorf("--print output differs from the written file:\nwritten:\n%s\nprinted:\n%s", written, rendered)
	}
}
