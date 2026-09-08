package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This offline fixture never starts Codex or reads credentials. Runtime children
// receive an allowlist, not os.Environ. Builds alone use the Go build environment.
type setupE2E struct {
	root, home, bundle string
	env                []string
}

func e2eWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
}
func e2eRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func newSetupE2E(t *testing.T) setupE2E {
	t.Helper()
	f := setupE2E{root: t.TempDir()}
	f.home = filepath.Join(f.root, "home space")
	f.bundle = filepath.Join(f.root, "bundle space")
	for _, key := range []string{"HOME", "USERPROFILE", "CODEX_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "APPDATA", "LOCALAPPDATA", "TMPDIR", "TEMP", "TMP"} {
		p := filepath.Join(f.home, key)
		if key == "HOME" || key == "USERPROFILE" {
			p = f.home
		}
		if key == "CODEX_HOME" {
			p = filepath.Join(f.home, ".codex")
		}
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		f.env = append(f.env, key+"="+p)
	}
	f.env = append(f.env, "PATH="+os.Getenv("PATH"))
	if runtime.GOOS == "windows" {
		f.env = append(f.env, "SystemRoot="+os.Getenv("SystemRoot"), "ComSpec="+os.Getenv("ComSpec"))
	}
	for _, name := range []string{"hook-wrapper.sh", "codex-hook-wrapper.sh", "codex-hook-wrapper.cmd"} {
		e2eWrite(t, filepath.Join(f.bundle, "bin", name), e2eRead(t, filepath.Join(repoRoot(t), "bin", name)))
	}
	e2eWrite(t, filepath.Join(f.bundle, ".claude-plugin/plugin.json"), []byte(`{"name":"claude-notifications-go","version":"9.9.9"}`))
	for _, name := range []string{"settings.json", "claude-notifications-go/plugin-root", "claude-notifications-go/state.json", "claude-notifications-go/dedup.json"} {
		e2eWrite(t, filepath.Join(f.home, ".claude", name), []byte("synthetic Claude canary: "+name))
	}
	e2eWrite(t, filepath.Join(f.home, ".claude/claude-notifications-go/config.json"), []byte(`{"notifications":{"desktop":{"enabled":false},"webhook":{"enabled":false}}}`))
	return f
}
func (f setupE2E) run(t *testing.T, input, binary string, args ...string) (string, error) {
	t.Helper()
	return f.runAt(t, f.root, input, binary, args...)
}
func (f setupE2E) runAt(t *testing.T, dir, input, binary string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, binary, args...)
	configureE2EShell(c, binary, args)
	c.Dir = dir
	c.Env = f.env
	c.Stdin = strings.NewReader(input)
	c.WaitDelay = 2 * time.Second
	out, err := c.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("process timeout: %v", args)
	}
	return string(out), err
}

func TestSetupCodexE2EDocumentedRelativeBundle(t *testing.T) {
	bin := buildCLIBinary(t)
	f := newSetupE2E(t)
	before := e2eSnapshot(t, f.root)
	out, err := f.runAt(t, f.bundle, "", bin, "setup-codex", "--plugin-root", ".", "--dry-run")
	if err != nil || !strings.Contains(out, "dry run") {
		t.Fatalf("documented dry run: %v %s", err, out)
	}
	if !reflect.DeepEqual(before, e2eSnapshot(t, f.root)) {
		t.Fatal("dry run changed sandbox")
	}
	for i := 0; i < 2; i++ {
		out, err = f.runAt(t, f.bundle, "", bin, "setup-codex", "--plugin-root", ".")
		if err != nil || !strings.Contains(out, "Codex notifications registered") {
			t.Fatalf("documented setup run %d: %v %s", i, err, out)
		}
	}
	hooks := e2eRead(t, filepath.Join(f.home, ".codex", "hooks.json"))
	if !strings.Contains(string(hooks), "codex-hook-wrapper") {
		t.Fatal("documented setup did not register hooks")
	}
	e2eRead(t, filepath.Join(f.home, ".codex", "claude-notifications-go", "bin", "codex-hook-wrapper.sh"))
}
func e2eSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		if i.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected symlink: %s", p)
		}
		if i.IsDir() {
			out[rel] = "dir"
		} else {
			out[rel] = string(e2eRead(t, p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestSetupCodexE2EReadOnlyAndMalformed(t *testing.T) {
	bin := buildCLIBinary(t)
	for _, mode := range []string{"--print", "--dry-run", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			f := newSetupE2E(t)
			args := []string{"setup-codex", "--plugin-root", f.bundle}
			if mode == "malformed" {
				e2eWrite(t, filepath.Join(f.home, ".codex/hooks.json"), []byte(`{"hooks": broken`))
			} else {
				args = append(args, mode)
			}
			before := e2eSnapshot(t, f.root)
			out, err := f.run(t, "", bin, args...)
			if mode == "malformed" {
				if err == nil || !strings.Contains(out, "setup-codex:") {
					t.Fatalf("expected diagnostic failure: %v %s", err, out)
				}
			} else if err != nil || out == "" {
				t.Fatalf("%v %s", err, out)
			}
			if !reflect.DeepEqual(before, e2eSnapshot(t, f.root)) {
				t.Fatal("read-only/failure path changed sandbox")
			}
		})
	}
}

func TestSetupCodexE2ERegistrationAndDelivery(t *testing.T) {
	bin := buildCLIBinary(t)
	f := newSetupE2E(t)
	received := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if e != nil {
			t.Error(e)
		}
		select {
		case received <- string(b):
		default:
			t.Error("unexpected extra delivery")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	cfg := fmt.Sprintf(`{"notifications":{"desktop":{"enabled":false},"webhook":{"enabled":true,"preset":"slack","url":%q}}}`, srv.URL)
	e2eWrite(t, filepath.Join(f.home, ".claude/claude-notifications-go/config.json"), []byte(cfg))
	claudeBefore := e2eSnapshot(t, filepath.Join(f.home, ".claude"))
	foreign := `{"note":"foreign canary","hooks":{"Stop":[{"matcher":"foreign","extra":17,"hooks":[{"type":"command","command":"foreign-command-never-executed","timeout":7,"custom":true}]}]}}`
	hooksPath := filepath.Join(f.home, ".codex/hooks.json")
	e2eWrite(t, hooksPath, []byte(foreign))
	// Explicit fixture version shim: only the launcher's version probe is faked.
	// All hook invocations forward unchanged to the real built binary. This is
	// registration/launcher proof, NOT proof that the shipped 1.41.0 passes 1.42.0.
	shimSource := `package main
import("os";"os/exec";"fmt")
func main(){if len(os.Args)==2&&os.Args[1]=="version"{fmt.Println("claude-notifications v9.9.9");return}; c:=exec.Command(os.Getenv("E2E_REAL_BINARY"),os.Args[1:]...);c.Stdin=os.Stdin;c.Stdout=os.Stdout;c.Stderr=os.Stderr;c.Env=os.Environ();if c.Run()!=nil{os.Exit(1)}}`
	src := filepath.Join(f.root, "version_fixture.go")
	e2eWrite(t, src, []byte(shimSource))
	name := "claude-notifications"
	if runtime.GOOS == "windows" {
		name = "claude-notifications-windows-" + runtime.GOARCH + ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(f.bundle, "bin", name), src)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("fixture build: %v %s", err, out)
	}
	f.env = append(f.env, "E2E_REAL_BINARY="+bin)
	for i := 0; i < 2; i++ {
		if out, err := f.run(t, "", bin, "setup-codex", "--plugin-root", f.bundle); err != nil {
			t.Fatalf("setup: %v %s", err, out)
		}
		if i == 0 {
			e2eWrite(t, filepath.Join(f.root, "first-hooks.json"), e2eRead(t, hooksPath))
		} else if string(e2eRead(t, hooksPath)) != string(e2eRead(t, filepath.Join(f.root, "first-hooks.json"))) {
			t.Fatal("registration not idempotent")
		}
	}
	var doc struct {
		Note  string `json:"note"`
		Hooks map[string][]struct {
			Matcher string           `json:"matcher"`
			Extra   int              `json:"extra"`
			Hooks   []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(e2eRead(t, hooksPath), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Note != "foreign canary" || len(doc.Hooks["Stop"]) != 2 {
		t.Fatalf("foreign metadata/group lost: %+v", doc)
	}
	g := doc.Hooks["Stop"][0]
	if g.Matcher != "foreign" || g.Extra != 17 || len(g.Hooks) != 1 || g.Hooks[0]["command"] != "foreign-command-never-executed" || g.Hooks[0]["custom"] != true || g.Hooks[0]["timeout"] != float64(7) {
		t.Fatal("foreign handler mutated")
	}
	for _, event := range []string{"Stop", "PermissionRequest", "PreToolUse", "SubagentStop"} {
		want := 1
		if event == "Stop" {
			want = 2
		}
		if len(doc.Hooks[event]) != want {
			t.Fatalf("%s registration count", event)
		}
	}
	commandKey := "command"
	if runtime.GOOS == "windows" {
		commandKey = "commandWindows"
	}
	command, ok := doc.Hooks["Stop"][1].Hooks[0][commandKey].(string)
	if !ok {
		t.Fatal("missing platform command")
	}
	for _, mode := range []string{"generated-launcher-version-fixture", "real-pipeline"} {
		marker := "sandbox-" + mode
		payload, _ := json.Marshal(map[string]any{"session_id": marker, "turn_id": marker, "cwd": f.root, "transcript_path": filepath.Join(f.root, "absent.jsonl"), "hook_event_name": "Stop", "last_assistant_message": "Completed " + marker, "stop_hook_active": false})
		var out string
		var err error
		if mode == "real-pipeline" {
			out, err = f.run(t, string(payload), bin, "handle-hook", "Stop", "--product", "codex")
		} else if runtime.GOOS == "windows" {
			out, err = f.run(t, string(payload), "cmd.exe", "/d", "/s", "/c", command)
		} else {
			out, err = f.run(t, string(payload), "sh", "-c", command)
		}
		if err != nil || out != "" {
			t.Fatalf("%s: %v %q", mode, err, out)
		}
		select {
		case body := <-received:
			if !strings.Contains(body, marker) {
				t.Fatalf("wrong payload: %s", body)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s produced no delivery (silent guard no-op is not success)", mode)
		}
	}
	if !reflect.DeepEqual(claudeBefore, e2eSnapshot(t, filepath.Join(f.home, ".claude"))) {
		t.Fatal("Codex setup/launch changed Claude files")
	}
}
