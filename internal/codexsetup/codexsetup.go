// Package codexsetup registers this plugin's hooks with the Codex CLI.
//
// Codex does not load hooks declared by a plugin manifest (its plugin_hooks
// feature is removed), so notifications only run when the hooks are present
// in the user's hooks.json. This package performs that registration in Go so
// one implementation covers macOS, Linux, and Windows.
//
// The registered command must stay byte-identical across releases: Codex
// hashes the command string for its trust review, so a versioned path would
// force the user to re-approve the hook after every update. Setup therefore
// installs a self-contained copy of the plugin under a stable directory in
// the Codex home and points the hooks at that copy.
package codexsetup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// InstallDirName is the stable directory (inside the Codex home) that holds
// the plugin copy the hooks point at. Frozen: it is part of the hook command
// string and therefore of the Codex trust hash.
const InstallDirName = "claude-notifications-go"

// hookTimeoutSeconds matches the shipped hooks-codex.json contract.
const hookTimeoutSeconds = 30

// registeredEvents are the Codex events this plugin handles, in the order
// they are written. Kept in sync with hooks/hooks-codex.json.
var registeredEvents = []struct {
	event   string
	matcher string
}{
	{event: "PreToolUse", matcher: "^request_user_input$"},
	{event: "Stop"},
	{event: "SubagentStop"},
	{event: "PermissionRequest"},
}

// Options controls a setup run.
type Options struct {
	// CodexHome overrides the Codex home directory (default: $CODEX_HOME,
	// then <user home>/.codex).
	CodexHome string
	// PluginRoot is the bundle to install from (default: the running
	// binary's plugin root).
	PluginRoot string
	// DryRun reports what would change without touching the filesystem.
	DryRun bool
}

// Result describes what a setup run did (or would do).
type Result struct {
	CodexHome   string
	InstallDir  string
	HooksPath   string
	BackupPath  string
	Events      []string
	Replaced    bool // an earlier registration was updated in place
	ForeignKept int  // hook entries owned by other tools that were preserved
}

// hookHandler is one command handler in hooks.json.
type hookHandler struct {
	Type           string `json:"type"`
	Command        string `json:"command"`
	CommandWindows string `json:"commandWindows,omitempty"`
	Timeout        int    `json:"timeout,omitempty"`
	Async          bool   `json:"async,omitempty"`
	// Unknown keys from foreign handlers are preserved verbatim.
	extra map[string]json.RawMessage
}

func (h hookHandler) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	for k, v := range h.extra {
		out[k] = v
	}
	set := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		out[key] = raw
		return nil
	}
	if err := set("type", h.Type); err != nil {
		return nil, err
	}
	if err := set("command", h.Command); err != nil {
		return nil, err
	}
	if h.CommandWindows != "" {
		if err := set("commandWindows", h.CommandWindows); err != nil {
			return nil, err
		}
	}
	if h.Timeout != 0 {
		if err := set("timeout", h.Timeout); err != nil {
			return nil, err
		}
	}
	if h.Async {
		if err := set("async", h.Async); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

func (h *hookHandler) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	h.extra = map[string]json.RawMessage{}
	for k, v := range raw {
		switch k {
		case "type":
			_ = json.Unmarshal(v, &h.Type)
		case "command":
			_ = json.Unmarshal(v, &h.Command)
		case "commandWindows":
			_ = json.Unmarshal(v, &h.CommandWindows)
		case "timeout":
			_ = json.Unmarshal(v, &h.Timeout)
		case "async":
			_ = json.Unmarshal(v, &h.Async)
		default:
			h.extra[k] = v
		}
	}
	return nil
}

// hookGroup is one matcher group in hooks.json.
type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookHandler `json:"hooks"`
}

// hooksFile is the subset of hooks.json this package understands. Top-level
// keys other than "hooks" are preserved verbatim.
type hooksFile struct {
	Hooks map[string][]hookGroup
	extra map[string]json.RawMessage
}

func (f hooksFile) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	for k, v := range f.extra {
		out[k] = v
	}
	raw, err := json.Marshal(f.Hooks)
	if err != nil {
		return nil, err
	}
	out["hooks"] = raw
	return json.Marshal(out)
}

func (f *hooksFile) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.extra = map[string]json.RawMessage{}
	f.Hooks = map[string][]hookGroup{}
	for k, v := range raw {
		if k == "hooks" {
			if err := json.Unmarshal(v, &f.Hooks); err != nil {
				return fmt.Errorf("hooks section is not an object of event arrays: %w", err)
			}
			continue
		}
		f.extra[k] = v
	}
	return nil
}

// ResolveCodexHome returns the Codex home directory for this machine.
func ResolveCodexHome(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return filepath.Clean(override), nil
	}
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		return filepath.Clean(env), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// HookCommands renders the POSIX and Windows command strings for one event.
// installDir must be the stable install directory.
func HookCommands(installDir, event string) (posix string, windows string) {
	posixLauncher := filepath.ToSlash(filepath.Join(installDir, "bin", "codex-hook-wrapper.sh"))
	windowsLauncher := filepath.Join(installDir, "bin", "codex-hook-wrapper.cmd")
	posix = fmt.Sprintf("sh %q handle-hook %s --product codex", posixLauncher, event)
	windows = fmt.Sprintf("cmd.exe /d /s /c call %q handle-hook %s --product codex", windowsLauncher, event)
	return posix, windows
}

// ownsHandler reports whether a handler was registered by this plugin. The
// check is intentionally narrow: it must never claim a hook a user wrote by
// hand for a different tool.
func ownsHandler(h hookHandler) bool {
	for _, cmd := range []string{h.Command, h.CommandWindows} {
		if cmd == "" {
			continue
		}
		if strings.Contains(cmd, "codex-hook-wrapper") && strings.Contains(cmd, "--product codex") {
			return true
		}
	}
	return false
}

// Run performs (or simulates) the registration.
func Run(opts Options) (Result, error) {
	codexHome, err := ResolveCodexHome(opts.CodexHome)
	if err != nil {
		return Result{}, err
	}
	pluginRoot := filepath.Clean(opts.PluginRoot)
	if pluginRoot == "" || pluginRoot == "." {
		return Result{}, fmt.Errorf("plugin root is required")
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, "bin", "codex-hook-wrapper.sh")); err != nil {
		return Result{}, fmt.Errorf("plugin root %q does not look like a claude-notifications bundle: %w", pluginRoot, err)
	}

	installDir := filepath.Join(codexHome, InstallDirName)
	hooksPath := filepath.Join(codexHome, "hooks.json")

	result := Result{
		CodexHome:  codexHome,
		InstallDir: installDir,
		HooksPath:  hooksPath,
	}
	for _, e := range registeredEvents {
		result.Events = append(result.Events, e.event)
	}

	// Reject installing from the destination onto itself.
	if sameDir(pluginRoot, installDir) {
		return Result{}, fmt.Errorf("plugin root and install directory are the same (%q); run setup from the plugin bundle", installDir)
	}

	existing, err := readHooksFile(hooksPath)
	if err != nil {
		return Result{}, err
	}
	merged, replaced, foreign := mergeHooks(existing, installDir)
	result.Replaced = replaced
	result.ForeignKept = foreign

	if opts.DryRun {
		return result, nil
	}

	if err := copyBundle(pluginRoot, installDir); err != nil {
		return Result{}, fmt.Errorf("failed to install plugin copy: %w", err)
	}

	backup, err := writeHooksFile(hooksPath, merged)
	if err != nil {
		return Result{}, err
	}
	result.BackupPath = backup
	return result, nil
}

// RenderHooksJSON returns the hooks.json fragment this plugin registers, for
// users who prefer to merge it themselves.
func RenderHooksJSON(installDir string) ([]byte, error) {
	empty := hooksFile{Hooks: map[string][]hookGroup{}, extra: map[string]json.RawMessage{}}
	merged, _, _ := mergeHooks(empty, installDir)
	return json.MarshalIndent(merged, "", "  ")
}

func mergeHooks(existing hooksFile, installDir string) (hooksFile, bool, int) {
	out := hooksFile{Hooks: map[string][]hookGroup{}, extra: map[string]json.RawMessage{}}
	for k, v := range existing.extra {
		out.extra[k] = v
	}
	replaced := false
	foreign := 0

	// Copy every event, dropping only handlers this plugin owns; foreign
	// handlers (and foreign groups) survive untouched.
	for event, groups := range existing.Hooks {
		var kept []hookGroup
		for _, g := range groups {
			var keptHandlers []hookHandler
			for _, h := range g.Hooks {
				if ownsHandler(h) {
					replaced = true
					continue
				}
				keptHandlers = append(keptHandlers, h)
				foreign++
			}
			if len(keptHandlers) == 0 {
				continue
			}
			g.Hooks = keptHandlers
			kept = append(kept, g)
		}
		if len(kept) > 0 {
			out.Hooks[event] = kept
		}
	}

	for _, spec := range registeredEvents {
		posix, windows := HookCommands(installDir, spec.event)
		// Deliberately synchronous. Measured against Codex v0.152.0: an
		// `async: true` handler never runs under `codex exec`, which exits as
		// soon as the turn ends, so the notification is silently lost. The
		// hook is fast and fail-open, and the timeout caps the worst case.
		group := hookGroup{
			Matcher: spec.matcher,
			Hooks: []hookHandler{{
				Type:           "command",
				Command:        posix,
				CommandWindows: windows,
				Timeout:        hookTimeoutSeconds,
			}},
		}
		out.Hooks[spec.event] = append(out.Hooks[spec.event], group)
	}
	return out, replaced, foreign
}

func readHooksFile(path string) (hooksFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return hooksFile{Hooks: map[string][]hookGroup{}, extra: map[string]json.RawMessage{}}, nil
	}
	if err != nil {
		return hooksFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return hooksFile{Hooks: map[string][]hookGroup{}, extra: map[string]json.RawMessage{}}, nil
	}
	var parsed hooksFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		// Never overwrite a file we cannot understand: the user would lose
		// their own hooks.
		return hooksFile{}, fmt.Errorf("%s exists but is not valid JSON (%w); fix or move it, then run setup again", path, err)
	}
	if parsed.Hooks == nil {
		parsed.Hooks = map[string][]hookGroup{}
	}
	return parsed, nil
}

// writeHooksFile writes atomically, backing up any existing file first.
func writeHooksFile(path string, content hooksFile) (string, error) {
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	backup := ""
	if prev, err := os.ReadFile(path); err == nil {
		backup = path + ".backup"
		if err := os.WriteFile(backup, prev, 0o600); err != nil {
			return "", fmt.Errorf("cannot write backup %s: %w", backup, err)
		}
	}

	tmp, err := os.CreateTemp(dir, "hooks-*.json.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	return backup, nil
}

// skippedBundleEntries are top-level bundle paths the Codex install does not
// need; skipping them keeps the copy small and avoids shipping sources.
var skippedBundleEntries = map[string]bool{
	".git":           true,
	".github":        true,
	"cmd":            true,
	"internal":       true,
	"pkg":            true,
	"docs":           true,
	"tests":          true,
	"testdata":       true,
	"swift-notifier": true,
}

func copyBundle(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if skippedBundleEntries[entry.Name()] {
			continue
		}
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		// Resolve symlinks (the shipped binary is one) so the install copy
		// never depends on the source tree.
		resolved, err := os.Stat(src)
		if err != nil {
			return nil // dangling link (wrong-platform binary): skip
		}
		if resolved.IsDir() {
			return copyDir(src, dst)
		}
		return copyFile(src, dst, resolved.Mode())
	case info.IsDir():
		return copyDir(src, dst)
	default:
		return copyFile(src, dst, info.Mode())
	}
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Replace rather than truncate: the destination may be a running binary.
	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.Abs(a)
	rb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ra, rb)
	}
	return ra == rb
}

// SortedEvents returns the registered event names in a stable order, for
// human-facing output.
func SortedEvents() []string {
	out := make([]string, 0, len(registeredEvents))
	for _, e := range registeredEvents {
		out = append(out, e.event)
	}
	sort.Strings(out)
	return out
}
