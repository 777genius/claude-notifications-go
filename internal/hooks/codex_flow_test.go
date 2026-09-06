package hooks

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/codexsource"
	"github.com/777genius/claude-notifications/internal/config"
)

func codexTestConfig() *config.Config {
	return &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete":      {Title: "Completed"},
			"question":           {Title: "Question"},
			"permission_request": {Title: "Permission Request"},
		},
	}
}

func newCodexTestHandler(t *testing.T, decoded codexsource.Decoded) (*Handler, *mockNotifier, *mockWebhook) {
	t.Helper()
	handler, mockNotif, mockWH := newTestHandler(t, codexTestConfig())
	handler.product = ProductCodex
	handler.source = CodexSource{DecodeFn: stubCodexDecode(decoded)}
	return handler, mockNotif, mockWH
}

func codexStopData(session, turn, message string, continuation bool) *codexsource.StopData {
	return &codexsource.StopData{
		SessionID:            session,
		TurnID:               turn,
		TranscriptPath:       "/rollout.jsonl",
		CWD:                  "/proj",
		HookEventName:        "Stop",
		Model:                "gpt-5.6-sol",
		PermissionMode:       "bypassPermissions",
		StopHookActive:       continuation,
		LastAssistantMessage: message,
	}
}

func TestCodexFlowStopNotifies(t *testing.T) {
	handler, mockNotif, _ := newCodexTestHandler(t, codexsource.Decoded{
		Stop: codexStopData("codex-flow-stop-1", "turn-1", "All tests pass.", false),
	})

	if err := handler.HandleHook("Stop", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("HandleHook() error = %v", err)
	}

	call := mockNotif.lastCall()
	if call == nil {
		t.Fatal("expected notification")
	}
	if call.status != analyzer.StatusTaskComplete {
		t.Errorf("status = %v, want task_complete", call.status)
	}
	if !strings.Contains(call.message, "All tests pass.") {
		t.Errorf("message %q does not contain the assistant message", call.message)
	}
}

func TestCodexFlowStopQuestion(t *testing.T) {
	handler, mockNotif, _ := newCodexTestHandler(t, codexsource.Decoded{
		Stop: codexStopData("codex-flow-question-1", "turn-1", "Should I continue?", false),
	})

	if err := handler.HandleHook("Stop", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("HandleHook() error = %v", err)
	}

	call := mockNotif.lastCall()
	if call == nil {
		t.Fatal("expected notification")
	}
	if call.status != analyzer.StatusQuestion {
		t.Errorf("status = %v, want question", call.status)
	}
}

func TestCodexFlowContinuationSuppressed(t *testing.T) {
	handler, mockNotif, mockWH := newCodexTestHandler(t, codexsource.Decoded{
		Stop: codexStopData("codex-flow-cont-1", "turn-1", "OK", true),
	})

	if err := handler.HandleHook("Stop", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("HandleHook() error = %v", err)
	}

	if mockNotif.wasCalled() || mockWH.wasCalled() {
		t.Fatal("continuation turn must not notify")
	}
}

func TestCodexFlowSubagentStopSkipped(t *testing.T) {
	handler, mockNotif, mockWH := newCodexTestHandler(t, codexsource.Decoded{
		SubagentStop: &codexsource.SubagentStopData{
			Stop:    *codexStopData("codex-flow-sub-1", "turn-1", "done", false),
			AgentID: "a1",
		},
	})

	if err := handler.HandleHook("SubagentStop", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("HandleHook() error = %v", err)
	}

	if mockNotif.wasCalled() || mockWH.wasCalled() {
		t.Fatal("codex SubagentStop delivery is out of scope and must not notify")
	}
}

func TestCodexFlowPermissionRequest(t *testing.T) {
	handler, mockNotif, _ := newCodexTestHandler(t, codexsource.Decoded{
		PermissionRequest: &codexsource.PermissionRequestData{
			SessionID:     "codex-flow-perm-1",
			TurnID:        "turn-1",
			CWD:           "/proj",
			HookEventName: "PermissionRequest",
			ToolName:      "shell",
			ToolInput:     []byte(`{"command":["rm","-rf","secret"],"api_key":"sk-XYZ"}`),
		},
	})

	if err := handler.HandleHook("PermissionRequest", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("HandleHook() error = %v", err)
	}

	call := mockNotif.lastCall()
	if call == nil {
		t.Fatal("expected notification")
	}
	if call.status != analyzer.StatusPermissionRequest {
		t.Errorf("status = %v, want permission_request", call.status)
	}
	if !strings.Contains(call.message, "shell") {
		t.Errorf("message %q must include the tool identity", call.message)
	}
	// ToolInput is never projected into the body.
	for _, secret := range []string{"sk-XYZ", "rm -rf", "api_key"} {
		if strings.Contains(call.message, secret) {
			t.Errorf("message %q leaks tool input %q", call.message, secret)
		}
	}
}

func TestCodexFlowTurnScopedDedup(t *testing.T) {
	session := "codex-flow-dedup-1"

	// Same turn twice: the second run must be deduplicated.
	handler, mockNotif, _ := newCodexTestHandler(t, codexsource.Decoded{
		Stop: codexStopData(session, "turn-A", "First.", false),
	})
	if err := handler.HandleHook("Stop", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("first HandleHook() error = %v", err)
	}
	if err := handler.HandleHook("Stop", strings.NewReader(`{}`)); err != nil {
		t.Fatalf("second HandleHook() error = %v", err)
	}
	if got := mockNotif.callCount(); got != 1 {
		t.Fatalf("same-turn duplicate delivered %d notifications, want 1", got)
	}
}

func TestCodexFlowUnsupportedEventFails(t *testing.T) {
	handler, _, _ := newCodexTestHandler(t, codexsource.Decoded{})
	handler.source = CodexSource{DecodeFn: func(_ context.Context, publicEvent string, _ []byte) (codexsource.Decoded, error) {
		return codexsource.Decoded{}, fmt.Errorf("unsupported codex event %q", publicEvent)
	}}

	err := handler.HandleHook("PreToolUse", strings.NewReader(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported codex event") {
		t.Fatalf("error = %v, want unsupported event", err)
	}
}
