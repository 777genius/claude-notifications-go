package hooks

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/dedup"
	"github.com/777genius/claude-notifications/internal/state"
	"github.com/777genius/claude-notifications/pkg/jsonl"
)

// === Mock Notifier ===

type mockNotifier struct {
	mu         sync.Mutex
	calls      []notificationCall
	shouldFail bool
}

type notificationCall struct {
	status  analyzer.Status
	message string
}

func (m *mockNotifier) SendDesktop(status analyzer.Status, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, notificationCall{
		status:  status,
		message: message,
	})

	if m.shouldFail {
		return errors.New("mock error")
	}
	return nil
}

func (m *mockNotifier) Close() error {
	return nil
}

func (m *mockNotifier) wasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls) > 0
}

func (m *mockNotifier) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockNotifier) lastCall() *notificationCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return &m.calls[len(m.calls)-1]
}

// === Mock Webhook ===

type mockWebhook struct {
	mu              sync.Mutex
	calls           []webhookCall
	shutdownCalled  bool
	shutdownTimeout time.Duration
}

type webhookCall struct {
	status    analyzer.Status
	message   string
	sessionID string
}

func (m *mockWebhook) SendAsync(status analyzer.Status, message, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, webhookCall{
		status:    status,
		message:   message,
		sessionID: sessionID,
	})
}

func (m *mockWebhook) Shutdown(timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
	m.shutdownTimeout = timeout
	return nil
}

func (m *mockWebhook) Send(status analyzer.Status, message, sessionID string) error {
	m.SendAsync(status, message, sessionID)
	return nil
}

func (m *mockWebhook) wasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls) > 0
}

func (m *mockWebhook) wasShutdownCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCalled
}

func (m *mockWebhook) getShutdownTimeout() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownTimeout
}

// === Test Helpers ===

func buildHookDataJSON(data HookData) io.Reader {
	b, _ := json.Marshal(data)
	return strings.NewReader(string(b))
}

func createTempTranscript(t *testing.T, messages []jsonl.Message) string {
	t.Helper()

	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "transcript.jsonl")

	f, err := os.Create(transcriptPath)
	if err != nil {
		t.Fatalf("failed to create transcript: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			t.Fatalf("failed to encode message: %v", err)
		}
	}

	return transcriptPath
}

func buildTranscriptWithTools(tools []string, textLength int) []jsonl.Message {
	var content []jsonl.Content

	// Add tools
	for _, tool := range tools {
		content = append(content, jsonl.Content{
			Type: "tool_use",
			Name: tool,
		})
	}

	// Add text
	text := strings.Repeat("a", textLength)
	content = append(content, jsonl.Content{
		Type: "text",
		Text: text,
	})

	return []jsonl.Message{
		{
			Type: "user",
			Message: jsonl.MessageContent{
				Role: "user",
				Content: []jsonl.Content{
					{Type: "text", Text: "Test request"},
				},
			},
			Timestamp: "2025-01-01T12:00:00Z",
		},
		{
			Type: "assistant",
			Message: jsonl.MessageContent{
				Role:    "assistant",
				Content: content,
			},
			Timestamp: "2025-01-01T12:00:01Z",
		},
	}
}

func newTestHandler(t *testing.T, cfg *config.Config) (*Handler, *mockNotifier, *mockWebhook) {
	t.Helper()

	// Clear CLAUDE_HOOK_JUDGE_MODE by default for all tests
	// This ensures tests don't accidentally get affected by judge mode
	// Tests that need judge mode set should call t.Setenv AFTER calling newTestHandler
	t.Setenv("CLAUDE_HOOK_JUDGE_MODE", "")

	mockNotif := &mockNotifier{}
	mockWH := &mockWebhook{}

	handler := &Handler{
		cfg:         cfg,
		dedupMgr:    dedup.NewManager(),
		stateMgr:    state.NewManager(),
		notifierSvc: mockNotif,
		webhookSvc:  mockWH,
		pluginRoot:  t.TempDir(),
	}

	return handler, mockNotif, mockWH
}

// === Integration Tests ===

func TestHandler_PreToolUse_ExitPlanMode(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"plan_ready": {Title: "Plan Ready"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID: "test-session-1",
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Error("expected notification to be sent")
	}

	call := mockNotif.lastCall()
	if call == nil {
		t.Fatal("no notification sent")
	}

	if call.status != analyzer.StatusPlanReady {
		t.Errorf("got status %v, want StatusPlanReady", call.status)
	}
}

func TestHandler_PreToolUse_AskUserQuestion(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"question": {Title: "Question"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID: "test-session-2",
		ToolName:  "AskUserQuestion",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Error("expected notification to be sent")
	}

	call := mockNotif.lastCall()
	if call.status != analyzer.StatusQuestion {
		t.Errorf("got status %v, want StatusQuestion", call.status)
	}
}

func TestHandler_Stop_ReviewComplete(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"review_complete": {Title: "Review Complete"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	// Create transcript with Read tools + long text
	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Read", "Read", "Grep"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-session-3",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Error("expected notification to be sent")
	}

	call := mockNotif.lastCall()
	if call.status != analyzer.StatusReviewComplete {
		t.Errorf("got status %v, want StatusReviewComplete", call.status)
	}
}

func TestHandler_Stop_TaskComplete(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	// Create transcript with active tools (Write/Edit)
	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Read", "Edit", "Write"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-session-4",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Error("expected notification to be sent")
	}

	call := mockNotif.lastCall()
	if call.status != analyzer.StatusTaskComplete {
		t.Errorf("got status %v, want StatusTaskComplete", call.status)
	}
}

func TestHandler_Notification_SuppressedAfterExitPlanMode(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
			SuppressQuestionAfterAnyNotificationSeconds: 60, // 60s suppression window
		},
		Statuses: map[string]config.StatusInfo{
			"plan_ready": {Title: "Plan Ready"},
			"question":   {Title: "Question"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	// 1. Send PreToolUse ExitPlanMode (writes session state)
	hookData1 := buildHookDataJSON(HookData{
		SessionID: "test-session-5",
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData1)
	if err != nil {
		t.Fatalf("PreToolUse error: %v", err)
	}

	initialCalls := mockNotif.callCount()

	// 2. Send Notification hook within 60s (should be suppressed - same session!)
	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"ExitPlanMode"}, 300))

	hookData2 := buildHookDataJSON(HookData{
		SessionID:      "test-session-5", // Same session ID
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	time.Sleep(100 * time.Millisecond) // Small delay

	err = handler.HandleHook("Notification", hookData2)
	if err != nil {
		t.Fatalf("Notification error: %v", err)
	}

	// Should not send duplicate notification
	if mockNotif.callCount() > initialCalls {
		t.Error("Notification should be suppressed after recent ExitPlanMode")
	}
}

// === Deduplication Tests ===

func TestHandler_EarlyDuplicateCheck(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "same-session",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	// First call
	err := handler.HandleHook("Stop", hookData)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	firstCallCount := mockNotif.callCount()

	// Immediate second call (< 2s) should be suppressed by early duplicate check
	time.Sleep(50 * time.Millisecond)

	hookData2 := buildHookDataJSON(HookData{
		SessionID:      "same-session",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err = handler.HandleHook("Stop", hookData2)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	// Should not send duplicate
	if mockNotif.callCount() > firstCallCount {
		t.Error("Duplicate hook should be suppressed by early check")
	}
}

// === Cooldown Tests ===

func TestHandler_QuestionCooldownAfterTaskComplete(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop:                                  config.DesktopConfig{Enabled: true},
			SuppressQuestionAfterTaskCompleteSeconds: 3,
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
			"question":      {Title: "Question"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	// 1. Send task_complete
	transcriptTask := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData1 := buildHookDataJSON(HookData{
		SessionID:      "test-cooldown-1",
		TranscriptPath: transcriptTask,
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData1)
	if err != nil {
		t.Fatalf("task_complete error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Fatal("task_complete notification should be sent")
	}

	taskCallCount := mockNotif.callCount()

	// 2. Send question within cooldown (3s) - should be suppressed (same session!)
	// Wait to ensure state file is fully written and flushed
	time.Sleep(200 * time.Millisecond)

	hookData2 := buildHookDataJSON(HookData{
		SessionID: "test-cooldown-1", // Same session ID
		CWD:       "/test",
	})

	err = handler.HandleHook("Notification", hookData2)
	if err != nil {
		t.Fatalf("notification error: %v", err)
	}

	// Should be suppressed
	if mockNotif.callCount() > taskCallCount {
		t.Errorf("Question should be suppressed within cooldown window, got %d calls, expected %d",
			mockNotif.callCount(), taskCallCount)
	}

	// 3. Wait for cooldown to expire (3s total from task_complete)
	time.Sleep(3 * time.Second)

	// Use same session ID - cooldown should have expired now
	hookData3 := buildHookDataJSON(HookData{
		SessionID: "test-cooldown-1", // Same session - cooldown expired
		CWD:       "/test",
	})

	err = handler.HandleHook("Notification", hookData3)
	if err != nil {
		t.Fatalf("notification after cooldown error: %v", err)
	}

	// Should go through after cooldown expires
	if mockNotif.callCount() <= taskCallCount {
		t.Errorf("Question should be sent after cooldown expires, got %d calls, expected > %d",
			mockNotif.callCount(), taskCallCount)
	}
}

// === Error Handling Tests ===

func TestHandler_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler, _, _ := newTestHandler(t, cfg)

	err := handler.HandleHook("Stop", strings.NewReader("invalid json"))

	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse hook data") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHandler_MissingTranscriptFile(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, _, _ := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-session-9",
		TranscriptPath: "/nonexistent/path.jsonl",
		CWD:            "/test",
	})

	// Should handle gracefully (degrades, not fails)
	err := handler.HandleHook("Stop", hookData)

	if err != nil {
		t.Errorf("should handle missing file gracefully, got error: %v", err)
	}

	// May still send notification with default message
	// (depends on implementation - this is graceful degradation)
}

func TestHandler_EmptySessionID(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"plan_ready": {Title: "Plan Ready"},
		},
	}

	handler, _, _ := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID: "", // Empty
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	// Should handle gracefully (uses "unknown")
	err := handler.HandleHook("PreToolUse", hookData)

	if err != nil {
		t.Errorf("should handle empty session ID gracefully, got error: %v", err)
	}
}

// === Notification Disabled Tests ===

func TestHandler_NotificationsDisabled(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: false},
			Webhook: config.WebhookConfig{Enabled: false},
		},
	}

	handler, mockNotif, mockWH := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID: "test-session-10",
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should exit early without sending
	if mockNotif.wasCalled() {
		t.Error("should not send notification when disabled")
	}

	if mockWH.wasCalled() {
		t.Error("should not send webhook when disabled")
	}
}

// === SubagentStop Tests ===

func TestHandler_SubagentStop_DisabledByDefault(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop:              config.DesktopConfig{Enabled: true},
			NotifyOnSubagentStop: false, // Default: no notifications for subagents
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-session-11",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err := handler.HandleHook("SubagentStop", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT send notification when disabled
	if mockNotif.wasCalled() {
		t.Error("expected NO notification for SubagentStop (disabled by default)")
	}
}

func TestHandler_SubagentStop_EnabledInConfig(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop:              config.DesktopConfig{Enabled: true},
			NotifyOnSubagentStop: true, // Explicitly enabled
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-session-12",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err := handler.HandleHook("SubagentStop", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should send notification when explicitly enabled
	if !mockNotif.wasCalled() {
		t.Error("expected notification for SubagentStop (explicitly enabled)")
	}
}

// === Unknown Hook Event ===

func TestHandler_UnknownHookEvent(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}
	handler, _, _ := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID: "test-session-12",
		CWD:       "/test",
	})

	err := handler.HandleHook("UnknownEvent", hookData)

	if err == nil {
		t.Fatal("expected error for unknown hook event")
	}

	if !strings.Contains(err.Error(), "unknown hook event") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// === Webhook Integration ===

func TestHandler_SendsWebhookWhenEnabled(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
			Webhook: config.WebhookConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, _, mockWH := newTestHandler(t, cfg)

	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-session-13",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // Webhook is async

	if !mockWH.wasCalled() {
		t.Error("expected webhook to be called when enabled")
	}
}

// === NewHandler Constructor Tests ===

func TestNewHandler_Success(t *testing.T) {
	// Create temp plugin root with valid config
	tmpDir := t.TempDir()

	// Create config directory and file (expected path: pluginRoot/config/config.json)
	configDir := filepath.Join(tmpDir, "config")
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config.json")
	configJSON := `{
		"notifications": {
			"desktop": {"enabled": true, "sound": true},
			"webhook": {"enabled": false}
		},
		"statuses": {
			"task_complete": {"title": "Task Complete"}
		}
	}`

	err = os.WriteFile(configPath, []byte(configJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create handler
	handler, err := NewHandler(tmpDir)

	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	if handler == nil {
		t.Fatal("handler is nil")
	}

	// Verify handler components
	if handler.cfg == nil {
		t.Error("handler.cfg is nil")
	}
	if handler.dedupMgr == nil {
		t.Error("handler.dedupMgr is nil")
	}
	if handler.stateMgr == nil {
		t.Error("handler.stateMgr is nil")
	}
	if handler.notifierSvc == nil {
		t.Error("handler.notifierSvc is nil")
	}
	if handler.webhookSvc == nil {
		t.Error("handler.webhookSvc is nil")
	}
	if handler.pluginRoot != tmpDir {
		t.Errorf("handler.pluginRoot = %s, want %s", handler.pluginRoot, tmpDir)
	}
}

func TestNewHandler_WithDefaultConfig(t *testing.T) {
	// Create empty plugin root (no config file)
	tmpDir := t.TempDir()

	// NewHandler should use default config
	handler, err := NewHandler(tmpDir)

	if err != nil {
		t.Fatalf("NewHandler with defaults failed: %v", err)
	}

	if handler == nil {
		t.Fatal("handler is nil")
	}

	// Verify default config was loaded
	if !handler.cfg.IsDesktopEnabled() {
		t.Error("expected desktop notifications enabled by default")
	}
}

func TestNewHandler_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config directory
	configDir := filepath.Join(tmpDir, "config")
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create invalid config (webhook enabled but no URL)
	configPath := filepath.Join(configDir, "config.json")
	configJSON := `{
		"notifications": {
			"webhook": {
				"enabled": true,
				"preset": "slack",
				"url": ""
			}
		}
	}`

	err = os.WriteFile(configPath, []byte(configJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// NewHandler should fail validation
	handler, err := NewHandler(tmpDir)

	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}

	if handler != nil {
		t.Error("expected handler to be nil on validation error")
	}

	if !strings.Contains(err.Error(), "invalid config") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewHandler_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config directory
	configDir := filepath.Join(tmpDir, "config")
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create malformed JSON config
	configPath := filepath.Join(configDir, "config.json")
	err = os.WriteFile(configPath, []byte("{ invalid json }"), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// NewHandler should fail to load config
	handler, err := NewHandler(tmpDir)

	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}

	if handler != nil {
		t.Error("expected handler to be nil on load error")
	}

	if !strings.Contains(err.Error(), "failed to load config") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewHandler_NonexistentPluginRoot(t *testing.T) {
	// Use nonexistent directory
	nonexistentDir := "/nonexistent/plugin/root/path"

	// NewHandler should still work (config will use defaults)
	handler, err := NewHandler(nonexistentDir)

	if err != nil {
		t.Fatalf("NewHandler with nonexistent root failed: %v", err)
	}

	if handler == nil {
		t.Fatal("handler is nil")
	}

	// Should use default config
	if !handler.cfg.IsDesktopEnabled() {
		t.Error("expected desktop notifications enabled by default")
	}
}

func TestNewHandler_EmptyPluginRoot(t *testing.T) {
	// Empty string as plugin root
	handler, err := NewHandler("")

	if err != nil {
		t.Fatalf("NewHandler with empty root failed: %v", err)
	}

	if handler == nil {
		t.Fatal("handler is nil")
	}

	// Should use default config
	if !handler.cfg.IsDesktopEnabled() {
		t.Error("expected desktop notifications enabled by default")
	}
}

// === Cleanup Tests ===

func TestCleanupOldLocks_Success(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, _, _ := newTestHandler(t, cfg)

	// Call cleanupOldLocks - should not panic
	handler.cleanupOldLocks()

	// Verify handler is still functional after cleanup
	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-after-cleanup",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData)
	if err != nil {
		t.Fatalf("Handler should work after cleanup: %v", err)
	}
}

func TestHandleStopEvent_EmptyTranscriptPath(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, _, _ := newTestHandler(t, cfg)

	// Send Stop hook with empty TranscriptPath
	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-empty-transcript",
		TranscriptPath: "", // Empty
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData)

	// Should handle gracefully (no error)
	if err != nil {
		t.Errorf("should handle empty transcript gracefully, got error: %v", err)
	}

	// May or may not send notification (depends on fallback behavior)
	// But should not crash
}

func TestHandleStopEvent_NonexistentTranscriptFile(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
		},
	}

	handler, _, _ := newTestHandler(t, cfg)

	// Send Stop hook with nonexistent transcript file
	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-nonexistent-transcript",
		TranscriptPath: "/nonexistent/path/transcript.jsonl",
		CWD:            "/test",
	})

	err := handler.HandleHook("Stop", hookData)

	// Should handle gracefully (no error, graceful degradation)
	if err != nil {
		t.Errorf("should handle nonexistent transcript gracefully, got error: %v", err)
	}
}

// === Background Judge Mode Tests (double-shot-latte compatibility) ===

// TestHandler_SkipsNotificationsInJudgeMode verifies that notifications are
// suppressed when CLAUDE_HOOK_JUDGE_MODE=true (set by double-shot-latte plugin
// when running background Claude instances for context evaluation)
func TestHandler_SkipsNotificationsInJudgeMode(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
			Webhook: config.WebhookConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"task_complete": {Title: "Task Complete"},
			"plan_ready":    {Title: "Plan Ready"},
			"question":      {Title: "Question"},
		},
	}

	handler, mockNotif, mockWH := newTestHandler(t, cfg)

	// Set the judge mode environment variable AFTER creating handler
	// (newTestHandler clears it by default, so we override here)
	t.Setenv("CLAUDE_HOOK_JUDGE_MODE", "true")

	// Test PreToolUse hook
	hookData1 := buildHookDataJSON(HookData{
		SessionID: "test-judge-mode-1",
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData1)
	if err != nil {
		t.Fatalf("PreToolUse error: %v", err)
	}

	if mockNotif.wasCalled() {
		t.Error("expected NO notification in judge mode (PreToolUse)")
	}

	// Test Stop hook
	transcriptPath := createTempTranscript(t,
		buildTranscriptWithTools([]string{"Write"}, 300))

	hookData2 := buildHookDataJSON(HookData{
		SessionID:      "test-judge-mode-2",
		TranscriptPath: transcriptPath,
		CWD:            "/test",
	})

	err = handler.HandleHook("Stop", hookData2)
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	if mockNotif.wasCalled() {
		t.Error("expected NO notification in judge mode (Stop)")
	}

	if mockWH.wasCalled() {
		t.Error("expected NO webhook in judge mode")
	}
}

// TestHandler_SendsNotificationsWhenJudgeModeNotSet verifies that notifications
// work normally when CLAUDE_HOOK_JUDGE_MODE is not set
func TestHandler_SendsNotificationsWhenJudgeModeNotSet(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"plan_ready": {Title: "Plan Ready"},
		},
	}

	// newTestHandler already clears CLAUDE_HOOK_JUDGE_MODE by default
	handler, mockNotif, _ := newTestHandler(t, cfg)

	hookData := buildHookDataJSON(HookData{
		SessionID: "test-no-judge-mode",
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData)
	if err != nil {
		t.Fatalf("PreToolUse error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Error("expected notification when judge mode is NOT set")
	}
}

// TestHandler_SendsNotificationsWhenJudgeModeFalse verifies that notifications
// work normally when CLAUDE_HOOK_JUDGE_MODE is set to something other than "true"
func TestHandler_SendsNotificationsWhenJudgeModeFalse(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: true},
		},
		Statuses: map[string]config.StatusInfo{
			"plan_ready": {Title: "Plan Ready"},
		},
	}

	handler, mockNotif, _ := newTestHandler(t, cfg)

	// Set env var to "false" AFTER handler creation - should NOT suppress notifications
	t.Setenv("CLAUDE_HOOK_JUDGE_MODE", "false")

	hookData := buildHookDataJSON(HookData{
		SessionID: "test-judge-mode-false",
		ToolName:  "ExitPlanMode",
		CWD:       "/test",
	})

	err := handler.HandleHook("PreToolUse", hookData)
	if err != nil {
		t.Fatalf("PreToolUse error: %v", err)
	}

	if !mockNotif.wasCalled() {
		t.Error("expected notification when judge mode is 'false'")
	}
}

// TestHandleHookCallsWebhookShutdown verifies that HandleHook calls
// webhookSvc.Shutdown() in defer to ensure graceful shutdown of async requests
func TestHandleHookCallsWebhookShutdown(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			Desktop: config.DesktopConfig{Enabled: false},
			Webhook: config.WebhookConfig{Enabled: true},
		},
	}

	handler, _, mockWH := newTestHandler(t, cfg)

	// Create transcript file with Stop event
	transcript := []jsonl.Message{
		{
			Type: "assistant",
			Message: jsonl.MessageContent{
				Role: "assistant",
				Content: []jsonl.Content{
					{Type: "text", Text: "Task completed"},
				},
			},
			Timestamp: "2025-01-01T12:00:00Z",
		},
	}
	transcriptFile := createTempTranscript(t, transcript)

	hookData := buildHookDataJSON(HookData{
		SessionID:      "test-shutdown-session",
		TranscriptPath: transcriptFile,
		CWD:            "/test",
	})

	// Call HandleHook - this should call Shutdown() in defer
	err := handler.HandleHook("Stop", hookData)
	if err != nil {
		t.Fatalf("HandleHook failed: %v", err)
	}

	// Verify that Shutdown was called
	if !mockWH.wasShutdownCalled() {
		t.Error("expected webhookSvc.Shutdown() to be called in defer, but it wasn't")
	}

	// Verify that Shutdown was called with correct timeout (5 seconds)
	expectedTimeout := 5 * time.Second
	actualTimeout := mockWH.getShutdownTimeout()
	if actualTimeout != expectedTimeout {
		t.Errorf("expected Shutdown timeout %v, got %v", expectedTimeout, actualTimeout)
	}
}
