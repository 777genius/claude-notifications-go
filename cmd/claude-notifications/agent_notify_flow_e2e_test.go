//go:build linux || darwin

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/777genius/agent-notifications/internal/agentnotify"
	"github.com/777genius/agent-notifications/internal/agentnotify/clientsetup"
	"github.com/777genius/agent-notifications/internal/agentnotify/journal"
	notifymcp "github.com/777genius/agent-notifications/internal/agentnotify/mcp"
	"github.com/777genius/agent-notifications/internal/agentnotify/origin"
	"github.com/777genius/agent-notifications/internal/agentnotify/portable"
	"github.com/777genius/agent-notifications/internal/agentnotify/portablesetup"
	notifysetup "github.com/777genius/agent-notifications/internal/agentnotify/setup"
	"github.com/777genius/agent-notifications/internal/installruntime"
	"github.com/777genius/agent-notifications/internal/notification"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAgentNotifyIsolatedInstallFlowE2E(t *testing.T) {
	proveConfigurePrimaryOrder(t, "claude")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f, request, deps := configureFixture(t)
	request.Provider = "codex"
	if _, err := configureNotifications(ctx, request, deps); err != nil {
		t.Fatal(err)
	}
	request.Provider = "claude"
	request.Route = nil
	if _, err := configureNotifications(ctx, request, deps); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(request.CodexHome, "config.toml"), filepath.Join(f.root, ".claude.json")} {
		if !strings.Contains(setupCommandRead(t, path), f.command) {
			t.Fatal("primary command missing after both client orders", path)
		}
	}
	if setupCommandRead(t, filepath.Join(f.root, ".claude", "claude-notifications-go", "config.json")) != setupCommandRead(t, f.global) {
		t.Fatal("global restrictions changed")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(f.root, "package source with spaces")
	probe := filepath.Join(pkg, "bin", "probe")
	files := map[string][]byte{
		"plugin.json":                  []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"agent-notify","version":"1.0.0"}`),
		"mcp.json":                     []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"agent-notify":{"type":"stdio","command":"./bin/probe","args":[],"env":{}}}}`),
		"skills/agent-notify/SKILL.md": []byte("---\nname: agent-notify\ndescription: Isolated flow fixture\n---\n"),
		"bin/probe":                    binary,
	}
	for rel, data := range files {
		path := filepath.Join(pkg, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0600)
		if rel == "bin/probe" {
			mode = 0700
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := installruntime.Commit(ctx, installruntime.Request{
		ControlRoot: f.control, RuntimeRoot: f.runtime, Owner: "existing-installer", ConsumerID: "existing",
		Files: []installruntime.File{{Path: filepath.Join(f.runtime, "primary"), Data: binary, Mode: 0700}},
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := installruntime.ReadInstalledSnapshot(f.control)
	if err != nil {
		t.Fatal(err)
	}
	uapRoot := filepath.Join(f.root, "uap")
	mat, err := portablesetup.NewMaterializer(portablesetup.UAPRoots{
		StateFile: filepath.Join(uapRoot, "state", "state-v2.json"), LockFile: filepath.Join(uapRoot, "state", "mutation.lock"),
		OperationsDir: filepath.Join(uapRoot, "state", "operations"), PluginDataBase: filepath.Join(uapRoot, "plugin data"),
		ManagedRoot: filepath.Join(uapRoot, "managed"), HelperExecutable: probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(f.root, "home", "codex-portable")
	if err := os.MkdirAll(filepath.Join(f.root, "scope"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	codexConfig := filepath.Join(request.CodexHome, "config.toml")
	skillDest := filepath.Join(request.CodexHome, "skills", "agent-notify", "SKILL.md")
	identity := portablesetup.Identity{
		InstallationID: "00000000-0000-4000-8000-000000000009", ComponentID: snap.Ledger.ID, Owner: "existing-installer",
		ScopeRoot: filepath.Join(f.root, "scope"), ControlRoot: f.control, GlobalConfig: f.global,
		RuntimeRoot: f.runtime, Primary: "primary",
	}
	discovery := portablesetup.Discovery{
		ConfigPath: codexConfig, Command: f.command,
		Skill: &clientsetup.SkillProjection{
			SourcePath: filepath.Join(f.runtime, "skills", "agent-notify", "SKILL.md"), DestinationPath: skillDest,
		},
	}
	got, err := mat.Install(ctx, portablesetup.MaterializeRequest{
		Identity: identity, Integration: portable.Codex, ExpectedGeneration: snap.Ledger.Generation, PackageRoot: pkg,
		ClientConfigRoot: config, ClientExecutable: probe, Discovery: discovery, OperationID: "flow-e2e-codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := mat.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var mcpPath string
	for _, installation := range state.Installations {
		for _, client := range installation.Clients {
			mcpPath = filepath.Join(client.TargetLocator, ".mcp.json")
		}
	}
	body, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	var projected struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if json.Unmarshal(body, &projected) != nil {
		t.Fatalf("projected MCP is not JSON: %s", body)
	}
	command, args, env := projected.Command, projected.Args, projected.Env
	if command == "" {
		for _, server := range projected.MCPServers {
			command, args, env = server.Command, server.Args, server.Env
		}
	}
	if command == "" || !strings.Contains(strings.Join(args, "\x00"), "portable-launch") {
		t.Fatalf("UAP projection missing portable-launch: %s", body)
	}
	if strings.Contains(setupCommandRead(t, codexConfig), f.command) {
		t.Fatal("owned Codex MCP survived portable forward")
	}
	if _, err := os.Stat(skillDest); !os.IsNotExist(err) {
		t.Fatal("owned Codex skill survived portable forward")
	}
	if !strings.Contains(setupCommandRead(t, filepath.Join(f.root, ".claude.json")), f.command) {
		t.Fatal("Claude registration lost during Codex portable forward")
	}
	pluginRoot := env["PLUGIN_ROOT"]
	if pluginRoot == "" {
		pluginRoot = pkg
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = []string{"PLUGIN_DATA=" + got.DataRoot, "PLUGIN_ROOT=" + pluginRoot}
	cmd.Dir = filepath.Dir(mcpPath)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	defer func() {
		if !done {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	reader := bufio.NewReader(out)
	exchange := func(request string, wantID int) []byte {
		t.Helper()
		if _, err := fmt.Fprintln(in, request); err != nil {
			t.Fatal(err)
		}
		line, err := reader.ReadBytes('\n')
		if err != nil || !bytes.Contains(line, []byte(`"id":`+fmt.Sprint(wantID))) || bytes.Contains(line, []byte(`"error"`)) {
			t.Fatalf("projected protocol failed id=%d: %v stderr=%s line=%s command=%q args=%q env=%q mcp=%s", wantID, err, diagnostics.String(), line, command, args, cmd.Env, body)
		}
		return line
	}
	exchange(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"foreign-client-label","version":"1"}}}`, 1)
	if _, err := fmt.Fprintln(in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); err != nil {
		t.Fatal(err)
	}
	listed := exchange(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, 2)
	if !bytes.Contains(listed, []byte(`"notify"`)) || !bytes.Contains(listed, []byte(`"notification_status"`)) {
		t.Fatalf("projected tools/list missing notify: %s", listed)
	}
	statusLine := exchange(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"notification_status","_meta":{"threadId":"flow-thread-A","cwd":"/tmp/spoof-cwd","clientInfo":"foreign-client-label"},"arguments":{}}}`, 3)
	if bytes.Contains(statusLine, []byte(`"isError":true`)) || bytes.Contains(statusLine, []byte("/tmp/spoof-cwd")) || bytes.Contains(statusLine, []byte("foreign-client-label")) {
		t.Fatalf("status used cwd/clientInfo: %s", statusLine)
	}
	if !bytes.Contains(statusLine, []byte(`"context_reason":"available"`)) || !bytes.Contains(statusLine, []byte(`"dedup_scope":"session"`)) {
		t.Fatalf("status origin was not per-call threadId: %s", statusLine)
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = in.Close()
	_ = cmd.Wait()
	done = true
	cwd := cmd.Dir
	if err := os.RemoveAll(cwd); err != nil {
		t.Fatal(err)
	}
	proveNotifyTargetsAfterCwdGone(t, cwd)

	if native := nativeFlowApp(t); native != "" {
		sourceA := native
		change, err := installruntime.StageNative(ctx, f.control, sourceA)
		if err != nil {
			t.Fatal(err)
		}
		change.After.DecoderFloor = 1
		pathA := change.After.Path
		if _, err := installruntime.Commit(ctx, installruntime.Request{ControlRoot: f.control, RuntimeRoot: f.runtime, Owner: "existing-installer", ConsumerID: "hooks", Native: change}); err != nil {
			t.Fatal(err)
		}
		sendQueuedNativeFromActive(t, ctx, f.control, filepath.Join(f.root, "native-spool"), pathA)
		next := nativeFlowApp(t)
		change, err = installruntime.StageNative(ctx, f.control, next)
		if err != nil {
			t.Fatal(err)
		}
		change.After.DecoderFloor = 1
		if change.After.Path == pathA {
			t.Fatal("native update reused callback identity")
		}
		pathB := change.After.Path
		if _, err := installruntime.Commit(ctx, installruntime.Request{ControlRoot: f.control, RuntimeRoot: f.runtime, Owner: "existing-installer", ConsumerID: "hooks", Native: change}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(pathA); err != nil {
			t.Fatal("queued-callback generation A disappeared during flow E2E")
		}
		beforeRollback, err := installruntime.ReadInstalledSnapshot(f.control)
		if err != nil || beforeRollback.Ledger.Native == nil {
			t.Fatal(err)
		}
		publishedBefore := len(beforeRollback.Ledger.Native.Published)
		exeA := filepath.Join(pathA, "Contents", "MacOS", "terminal-notifier-modern")
		capOut, err := exec.CommandContext(ctx, exeA, "--capabilities-json").CombinedOutput()
		if err != nil {
			t.Fatalf("generation A dead after update: %s %v", capOut, err)
		}
		launchGenerationAfterColdStart(t, ctx, pathA)
		rollback, err := installruntime.StageNative(ctx, f.control, sourceA)
		if err != nil {
			t.Fatal(err)
		}
		rollback.After.DecoderFloor = 1
		if rollback.After.Path != pathA {
			t.Fatal("rollback restage of A assigned a new callback identity")
		}
		ledger, err := installruntime.Commit(ctx, installruntime.Request{ControlRoot: f.control, RuntimeRoot: f.runtime, Owner: "existing-installer", ConsumerID: "hooks", Native: rollback})
		if err != nil {
			t.Fatal(err)
		}
		if ledger.Native == nil || ledger.Native.Path != pathA {
			t.Fatal("active native path did not roll back to A")
		}
		if _, err := os.Stat(pathB); err != nil {
			t.Fatal("generation B deleted during rollback")
		}
		if len(ledger.Native.Published) != publishedBefore {
			t.Fatalf("rollback mutated published inventory %d -> %d", publishedBefore, len(ledger.Native.Published))
		}
		launchGenerationAfterColdStart(t, ctx, pathA)
		gen := ledger.Generation
		if _, err := installruntime.Commit(ctx, installruntime.Request{ControlRoot: f.control, RuntimeRoot: f.runtime, Owner: "existing-installer", ConsumerID: "hooks", ExpectedGeneration: &gen, RetireNative: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(pathA); err != nil {
			t.Fatal("retire removed published generation A")
		}
		if _, err := os.Stat(pathB); err != nil {
			t.Fatal("retire removed published generation B")
		}
	}
	snap, err = installruntime.ReadInstalledSnapshot(f.control)
	if err != nil {
		t.Fatal(err)
	}
	if err := mat.Remove(ctx, portablesetup.MaterializeRequest{
		Identity: identity, Integration: portable.Codex, ExpectedGeneration: snap.Ledger.Generation,
		ClientConfigRoot: config, ClientExecutable: probe, Discovery: discovery, OperationID: "flow-e2e-codex-remove",
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(setupCommandRead(t, codexConfig), f.command) {
		t.Fatal("owned Codex MCP not restored after portable reverse")
	}
	if setupCommandRead(t, skillDest) != "canonical test skill" {
		t.Fatal("owned Codex skill not restored after portable reverse")
	}
	claudePath := filepath.Join(f.root, ".claude.json")
	if !strings.Contains(setupCommandRead(t, claudePath), f.command) {
		t.Fatal("Claude registration lost during Codex portable reverse")
	}
	if setupCommandRead(t, filepath.Join(f.root, ".claude", "claude-notifications-go", "config.json")) != setupCommandRead(t, f.global) {
		t.Fatal("global restrictions changed after portable reverse")
	}
	name, err := got.Filename()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := portable.Acquire(ctx, got.DataRoot, name); err == nil {
		t.Fatal("locator survived portable reverse")
	}
	hook := setupCommandRead(t, filepath.Join(f.runtime, "hook"))
	global := setupCommandRead(t, f.global)
	codex := setupCommandRead(t, codexConfig)
	claude := setupCommandRead(t, claudePath)
	snap, err = installruntime.ReadInstalledSnapshot(f.control)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := notifysetup.Apply(ctx, notifysetup.Options{
		ControlRoot: f.control, RuntimeRoot: f.runtime, Owner: "existing-installer", ConsumerID: "hooks",
	}, notifysetup.Request{ExpectedGeneration: snap.Ledger.Generation, Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if setupCommandRead(t, filepath.Join(f.runtime, "hook")) != hook || setupCommandRead(t, f.global) != global || setupCommandRead(t, codexConfig) != codex || setupCommandRead(t, claudePath) != claude {
		t.Fatal("disable mutated hooks, global config, or client registrations")
	}
}

func proveConfigurePrimaryOrder(t *testing.T, first string) {
	t.Helper()
	f, request, deps := configureFixture(t)
	secondary := filepath.Join(f.root, "secondary")
	_, err := installruntime.Commit(setupCommandContext(t), installruntime.Request{ControlRoot: f.control, RuntimeRoot: secondary, Owner: "existing-installer", ConsumerID: "secondary", Consumer: installruntime.Consumer{Commands: []string{"secondary hook"}}, Files: []installruntime.File{{Path: filepath.Join(secondary, "bin", "claude-notifications"), Data: []byte("inert secondary " + installruntime.WriterProtocolMarker), Mode: 0700}}})
	if err != nil {
		t.Fatal(err)
	}
	deps.BundleRoot = secondary
	request.Provider = first
	if _, err = configureNotifications(setupCommandContext(t), request, deps); err != nil {
		t.Fatal(err)
	}
	request.Route = nil
	if first == "codex" {
		request.Provider = "claude"
	} else {
		request.Provider = "codex"
	}
	if _, err = configureNotifications(setupCommandContext(t), request, deps); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(f.root, ".claude.json"), filepath.Join(request.CodexHome, "config.toml")} {
		raw := setupCommandRead(t, p)
		if !strings.Contains(raw, f.command) || strings.Contains(raw, secondary) {
			t.Fatal("secondary command selected", raw)
		}
	}
}

type flowNotifySpy struct {
	mu    sync.Mutex
	calls []notification.Request
}

func (*flowNotifySpy) CheckReadiness(_ context.Context, r notification.Request) notification.Readiness {
	return notification.Readiness{CorrelationID: r.CorrelationID, Status: "ready", Reason: "ready", Navigation: notification.NavigationResult{Capability: "available", Precision: "chat_id", Scope: "local_current_profile"}}
}

func (p *flowNotifySpy) Deliver(_ context.Context, r notification.Request) notification.Receipt {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, r)
	return notification.Receipt{CorrelationID: r.CorrelationID, Status: "submitted", Reason: "os_accepted", Backend: "fixture", Navigation: notification.NavigationResult{Capability: "available", Precision: "chat_id", Scope: "local_current_profile"}}
}

type flowStatus struct{}

func (flowStatus) Status(context.Context) (notifymcp.Status, error) {
	return notifymcp.Status{Enabled: true, Configuration: "enabled", Capability: "available"}, nil
}

func proveNotifyTargetsAfterCwdGone(t *testing.T, cwd string) {
	t.Helper()
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Fatal("mcp cwd still present")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSetup()
	store, err := journal.Initialize(setupCtx, journal.Options{Root: root, Clock: journal.ClockFunc(func() journal.Sample { return journal.Sample{Boot: "test", Seconds: 100, Available: true} })})
	if err != nil {
		t.Fatal(err)
	}
	spy := &flowNotifySpy{}
	service, err := agentnotify.New(agentnotify.Dependencies{
		Admission: store,
		Clock:     agentnotify.ClockFunc(func() notification.Deadline { return notification.Deadline{BootID: "test", NotAfter: 100} }),
		Policy: agentnotify.PolicyFunc(func(context.Context, origin.Context) (agentnotify.Policy, error) {
			return agentnotify.Policy{Rates: journal.RatePolicy{SessionPerMinute: 6, RuntimePerMinute: 30, Burst: 6}, Delivery: notification.PolicySnapshot{Valid: true, ExplicitEnabled: true, DesktopEnabled: true, ClickToFocus: true}, Route: origin.RoutePolicy{LocalRouting: true, AllowUnknownCaller: true, ApplicationPath: "/fixture/Codex.app", TeamID: "fixture"}}, nil
		}),
		Target: agentnotify.TargetFunc(func(_ context.Context, o origin.Context, p origin.RoutePolicy) (origin.Target, error) {
			return origin.ResolveCodex(o, p), nil
		}),
		Readiness: spy, Delivery: spy,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server, peer := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- notifymcp.Run(ctx, server, notifymcp.Options{
			Backend: service, Status: flowStatus{},
			Clock:       agentnotify.ClockFunc(func() notification.Deadline { return notification.Deadline{BootID: "test", NotAfter: 100} }),
			AdapterKind: "codex",
		})
	}()
	client := sdk.NewClient(&sdk.Implementation{Name: "flow-e2e", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: peer, Writer: peer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = peer.Close()
		_ = session.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("mcp Run failed to join")
		}
	}()
	call := func(thread, body string) agentnotify.Receipt {
		t.Helper()
		result, e := session.CallTool(ctx, &sdk.CallToolParams{Name: "notify", Meta: sdk.Meta{"threadId": thread, "cwd": cwd}, Arguments: map[string]any{"title": "--help [important]", "body": body, "category": "attention", "request_id": "flow-" + thread}})
		if e != nil {
			t.Fatal(e)
		}
		text, ok := result.Content[0].(*sdk.TextContent)
		if !ok {
			t.Fatal("missing receipt")
		}
		var receipt agentnotify.Receipt
		if json.Unmarshal([]byte(text.Text), &receipt) != nil {
			t.Fatal(text.Text)
		}
		return receipt
	}
	firstA := call("flow-thread-A", "literal A")
	firstB := call("flow-thread-B", "literal B")
	if firstA.Status != "submitted" || firstB.Status != "submitted" || firstA.TrackingID == firstB.TrackingID {
		t.Fatalf("A/B not distinct submissions: %+v %+v", firstA, firstB)
	}
	replayA := call("flow-thread-A", "literal A")
	if !replayA.Replayed || replayA.TrackingID != firstA.TrackingID {
		t.Fatalf("replay created a new effect: %+v / %+v", firstA, replayA)
	}
	proveConcurrentSameCwdNotifies(t, service, cwd)
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.calls) != 4 {
		t.Fatalf("duplicate or missing native effects: %d", len(spy.calls))
	}
	seen := map[string]bool{}
	for _, req := range spy.calls {
		seen[req.Target.ThreadID] = true
		if req.Target.ApplicationPath != "/fixture/Codex.app" {
			t.Fatal("application path lost")
		}
		if req.Content.Title != "--help [important]" {
			t.Fatal("literal payload altered")
		}
	}
	if !seen["flow-thread-A"] || !seen["flow-thread-B"] || !seen["flow-conc-A"] || !seen["flow-conc-B"] {
		t.Fatalf("source target not from threadId: %+v", spy.calls)
	}
}

func proveConcurrentSameCwdNotifies(t *testing.T, service *agentnotify.Service, cwd string) {
	t.Helper()
	var wg sync.WaitGroup
	for _, thread := range []string{"flow-conc-A", "flow-conc-B"} {
		thread := thread
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			server, peer := net.Pipe()
			done := make(chan error, 1)
			go func() {
				done <- notifymcp.Run(ctx, server, notifymcp.Options{
					Backend: service, Status: flowStatus{},
					Clock: agentnotify.ClockFunc(func() notification.Deadline {
						return notification.Deadline{BootID: "test", NotAfter: 100}
					}),
					AdapterKind: "codex",
				})
			}()
			client := sdk.NewClient(&sdk.Implementation{Name: "flow-e2e-conc", Version: "1"}, nil)
			session, err := client.Connect(ctx, &sdk.IOTransport{Reader: peer, Writer: peer}, nil)
			if err != nil {
				t.Errorf("concurrent connect %s: %v", thread, err)
				return
			}
			result, err := session.CallTool(ctx, &sdk.CallToolParams{
				Name: "notify",
				Meta: sdk.Meta{"threadId": thread, "cwd": cwd},
				Arguments: map[string]any{
					"title": "--help [important]", "body": "concurrent " + thread,
					"category": "attention", "request_id": "conc-" + thread,
				},
			})
			_ = session.Close()
			_ = peer.Close()
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Errorf("concurrent mcp %s failed to join", thread)
			}
			if err != nil || result == nil || result.IsError {
				t.Errorf("concurrent notify %s: %v %+v", thread, err, result)
				return
			}
			text, ok := result.Content[0].(*sdk.TextContent)
			if !ok {
				t.Errorf("concurrent %s missing receipt", thread)
				return
			}
			var receipt agentnotify.Receipt
			if json.Unmarshal([]byte(text.Text), &receipt) != nil || receipt.Status != "submitted" {
				t.Errorf("concurrent %s receipt: %s", thread, text.Text)
			}
		}()
	}
	wg.Wait()
}

func nativeFlowApp(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("missing caller")
	}
	repo := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	bin := filepath.Join(repo, "swift-notifier/.build/arm64-apple-macosx/release/terminal-notifier-modern")
	plist := filepath.Join(repo, "swift-notifier/Resources/Info.plist")
	if _, err := os.Stat(bin); err != nil {
		if runtime.GOOS == "darwin" {
			t.Fatal("exact-head helper is required for isolated flow E2E")
		}
		return ""
	}
	root := filepath.Join(t.TempDir(), "ClaudeNotifier.app")
	macOS := filepath.Join(root, "Contents", "MacOS")
	if err := os.MkdirAll(macOS, 0755); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(macOS, "terminal-notifier-modern"), body, 0755); err != nil {
		t.Fatal(err)
	}
	info, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	bundleID := "com.agentnotify.test.flow"
	if os.Getenv("AGENT_NOTIFY_DARWIN_E2E") == "1" {
		bundleID = "com.claude.desktop.notifier"
	}
	info = bytes.Replace(info, []byte("com.claude.desktop.notifier"), []byte(bundleID), 1)
	if err := os.WriteFile(filepath.Join(root, "Contents", "Info.plist"), info, 0644); err != nil {
		t.Fatal(err)
	}
	marker := []byte(t.Name() + time.Now().String())
	if err := os.MkdirAll(filepath.Join(root, "Contents", "Resources"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Contents", "Resources", "generation.marker"), marker, 0644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("codesign", "--force", "--sign", "-", "--timestamp=none", "--identifier", bundleID, root).CombinedOutput(); err != nil {
			t.Fatalf("codesign generation helper: %s %v", out, err)
		}
	}
	return root
}
