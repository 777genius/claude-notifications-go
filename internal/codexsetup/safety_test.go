package codexsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestForeignEmptyAndMatcherPreserved(t *testing.T) {
	for _, input := range []string{
		`{"hooks":{"Other":[],"Null":null,"Stop":[{"matcher":"","hooks":[]},{"matcher":{"future":true},"hooks":null},{"annotation":true},null]}}`,
	} {
		var before hooksFile
		if err := json.Unmarshal([]byte(input), &before); err != nil {
			t.Fatal(err)
		}
		merged, _, _ := mergeHooks(before, t.TempDir())
		encoded, err := json.Marshal(merged)
		if err != nil {
			t.Fatal(err)
		}
		var original, actual map[string]any
		json.Unmarshal([]byte(input), &original)
		json.Unmarshal(encoded, &actual)
		oldHooks := original["hooks"].(map[string]any)
		newHooks := actual["hooks"].(map[string]any)
		for event, groups := range oldHooks {
			got := newHooks[event]
			if event == "Stop" {
				got = got.([]any)[:len(groups.([]any))]
			}
			if !reflect.DeepEqual(groups, got) {
				t.Fatalf("%s: %v became %v", event, groups, got)
			}
		}
	}
}

// Lazy download and version refresh must remain available in the stable copy.
func TestCopyPreservesUpdaterAssets(t *testing.T) {
	src, dst := fakeBundle(t), t.TempDir()
	files := map[string]string{
		".claude-plugin/plugin.json":  `{"version":"1.42.0"}`,
		".claude-plugin/private-note": "not a runtime asset",
		"bin/install.sh":              "#!/bin/sh\nexit 0\n",
		"bin/bootstrap.sh":            "not used by the Codex launcher",
	}
	for name, content := range files {
		path := filepath.Join(src, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyBundle(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude-plugin/plugin.json", "bin/install.sh"} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name)))
		if err != nil || string(got) != files[name] {
			t.Fatalf("runtime asset %s missing or altered: %v", name, err)
		}
	}
	for _, name := range []string{".claude-plugin/private-note", "bin/bootstrap.sh"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("unexpected asset %s: %v", name, err)
		}
	}
}

func TestCopyFailureLeavesLiveAssets(t *testing.T) {
	src, dst := fakeBundle(t), t.TempDir()
	if err := copyBundle(src, dst); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dst, "bin", "codex-hook-wrapper.sh")
	before, _ := os.ReadFile(live)
	os.WriteFile(filepath.Join(src, "bin", "codex-hook-wrapper.sh"), []byte("new"), 0700)
	if err := os.Symlink("missing", filepath.Join(src, "sounds", "broken")); err != nil {
		t.Skip(err)
	}
	if err := copyBundle(src, dst); err == nil {
		t.Fatal("expected staging failure")
	}
	after, _ := os.ReadFile(live)
	if string(before) != string(after) {
		t.Fatal("live bundle changed on copy failure")
	}
}

func TestCopyRejectsOverlapAndAliases(t *testing.T) {
	src := fakeBundle(t)
	for _, dst := range []string{src, filepath.Join(src, "nested"), filepath.Dir(src)} {
		if err := copyBundle(src, dst); err == nil {
			t.Fatalf("accepted overlap %s", dst)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(src, alias); err != nil {
		t.Skip(err)
	}
	if err := copyBundle(src, alias); err == nil {
		t.Fatal("accepted alias")
	}
	if err := copyBundle(src, filepath.Join(alias, "new")); err == nil {
		t.Fatal("accepted nested alias")
	}
}

func TestRuntimeSelectionAndRollback(t *testing.T) {
	src, dst := fakeBundle(t), t.TempDir()
	for _, name := range []string{".env", "notification-debug.log", ".claude/worktrees/secret", "bin/private-token"} {
		path := filepath.Join(src, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte("secret fixture"), 0600)
	}
	if err := copyBundle(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", "notification-debug.log", ".claude", "bin/private-token"} {
		if _, err := os.Lstat(filepath.Join(dst, name)); !os.IsNotExist(err) {
			t.Fatalf("copied %s", name)
		}
	}
	path := filepath.Join(dst, "bin", "codex-hook-wrapper.sh")
	old, _ := os.ReadFile(path)
	os.WriteFile(filepath.Join(src, "bin", "codex-hook-wrapper.sh"), []byte("replacement"), 0700)
	rollback, finish, err := stageBundle(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != string(old) {
		t.Fatal("rollback lost original")
	}
}

func TestSetupLockPreservesHooks(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "hooks.json")
	original := `{"hooks":{}}`
	os.WriteFile(path, []byte(original), 0600)
	os.WriteFile(filepath.Join(home, ".claude-notifications-setup.lock"), nil, 0600)
	if _, err := Run(Options{CodexHome: home, PluginRoot: fakeBundle(t)}); err == nil {
		t.Fatal("concurrent setup accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != original {
		t.Fatal("hooks changed")
	}
}

func TestRelativeCodexHomeIsAbsolute(t *testing.T) {
	t.Setenv("CODEX_HOME", "relative-fixture")
	for _, override := range []string{"", "explicit-fixture"} {
		got, err := ResolveCodexHome(override)
		if err != nil || !filepath.IsAbs(got) {
			t.Fatalf("%q: %v", got, err)
		}
	}
}

func TestHooksSnapshotRejectsLostUpdate(t *testing.T) {
	for _, existed := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "hooks.json")
		expected := []byte(nil)
		if existed {
			expected = []byte(`{"hooks":{}}`)
		}
		changed := []byte(`{"hooks":{},"foreign":"new edit"}`)
		os.WriteFile(path, changed, 0600)
		_, err := writeHooksFile(path, hooksFile{Hooks: map[string][]hookGroup{}}, expected, existed)
		if err == nil {
			t.Fatal("overwrote concurrent edit")
		}
		actual, _ := os.ReadFile(path)
		if string(actual) != string(changed) {
			t.Fatal("foreign edit lost")
		}
	}
}

func TestSymlinkAssetsCannotEscapeOrRecurse(t *testing.T) {
	for _, target := range []string{"..", filepath.Join(t.TempDir(), "private-fixture")} {
		src := fakeBundle(t)
		if filepath.IsAbs(target) {
			os.WriteFile(target, []byte("fixture"), 0600)
		}
		if err := os.Symlink(target, filepath.Join(src, "sounds", "link")); err != nil {
			t.Skip(err)
		}
		if err := copyBundle(src, t.TempDir()); err == nil {
			t.Fatal("unsafe symlink copied")
		}
	}
}

func TestLegacyGeneratedRegistrationMigrates(t *testing.T) {
	dir := t.TempDir()
	posix, _ := HookCommands(dir, "Stop")
	h := hookHandler{Type: "command", Command: posix, CommandWindows: "cmd.exe /d /s /c call " + windowsQuote(filepath.Join(dir, "bin", "codex-hook-wrapper.cmd")) + " handle-hook Stop --product codex"}
	if !ownsHandler(h, dir, "Stop") {
		t.Fatal("legacy registration not recognized")
	}
	if ownsHandler(h, t.TempDir(), "Stop") {
		t.Fatal("foreign location claimed")
	}
}
