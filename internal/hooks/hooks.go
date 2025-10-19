package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/belief/claude-notifications/internal/analyzer"
	"github.com/belief/claude-notifications/internal/config"
	"github.com/belief/claude-notifications/internal/dedup"
	"github.com/belief/claude-notifications/internal/logging"
	"github.com/belief/claude-notifications/internal/notifier"
	"github.com/belief/claude-notifications/internal/platform"
	"github.com/belief/claude-notifications/internal/sessionname"
	"github.com/belief/claude-notifications/internal/state"
	"github.com/belief/claude-notifications/internal/summary"
	"github.com/belief/claude-notifications/internal/webhook"
)

// HookData represents the data received from Claude Code hooks
type HookData struct {
	TranscriptPath string `json:"transcript_path"`
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	ToolName       string `json:"tool_name,omitempty"`
	HookEventName  string `json:"hook_event_name,omitempty"`
}

// Handler handles hook events
type Handler struct {
	cfg          *config.Config
	dedupMgr     *dedup.Manager
	stateMgr     *state.Manager
	notifierSvc  *notifier.Notifier
	webhookSvc   *webhook.Sender
	pluginRoot   string
}

// NewHandler creates a new hook handler
func NewHandler(pluginRoot string) (*Handler, error) {
	// Load config
	cfg, err := config.LoadFromPluginRoot(pluginRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Handler{
		cfg:         cfg,
		dedupMgr:    dedup.NewManager(),
		stateMgr:    state.NewManager(),
		notifierSvc: notifier.New(cfg),
		webhookSvc:  webhook.New(cfg),
		pluginRoot:  pluginRoot,
	}, nil
}

// HandleHook handles a hook event
func (h *Handler) HandleHook(hookEvent string, input io.Reader) error {
	logging.SetPrefix(fmt.Sprintf("PID:%d", os.Getpid()))
	logging.Debug("=== Hook triggered: %s ===", hookEvent)

	// Parse hook data
	var hookData HookData
	if err := json.NewDecoder(input).Decode(&hookData); err != nil {
		return fmt.Errorf("failed to parse hook data: %w", err)
	}

	logging.Debug("Hook data: session=%s, transcript=%s, tool=%s",
		hookData.SessionID, hookData.TranscriptPath, hookData.ToolName)

	// Validate session ID
	if hookData.SessionID == "" {
		hookData.SessionID = "unknown"
		logging.Warn("Session ID is empty, using 'unknown'")
	}

	// Phase 1: Early duplicate check
	if h.dedupMgr.CheckEarlyDuplicate(hookEvent, hookData.SessionID) {
		logging.Debug("Early duplicate detected, skipping")
		return nil
	}

	// Check if any notification method is enabled
	if !h.cfg.IsAnyNotificationEnabled() {
		logging.Debug("All notifications disabled, exiting")
		return nil
	}

	// Determine status based on hook type
	var status analyzer.Status
	var err error

	switch hookEvent {
	case "PreToolUse":
		status = h.handlePreToolUse(&hookData)
	case "Notification":
		// Check session state first (60s TTL) to suppress duplicates after PreToolUse
		status, err = h.handleNotificationEvent(&hookData)
		if err != nil {
			return err
		}
	case "Stop", "SubagentStop":
		// Analyze the transcript to determine status
		status, err = h.handleStopEvent(&hookData)
		if err != nil {
			return err
		}
		// Cleanup session state only for Stop/SubagentStop
		defer h.cleanupSession(hookData.SessionID)
	default:
		return fmt.Errorf("unknown hook event: %s", hookEvent)
	}

	// If status is unknown, skip
	if status == analyzer.StatusUnknown {
		logging.Debug("Status is unknown, skipping notification")
		return nil
	}

	// Check cooldown for question status
	if status == analyzer.StatusQuestion {
		suppress, err := h.stateMgr.ShouldSuppressQuestion(
			hookData.SessionID,
			h.cfg.Notifications.SuppressQuestionAfterTaskCompleteSeconds,
		)
		if err != nil {
			logging.Warn("Failed to check cooldown: %v", err)
		} else if suppress {
			logging.Debug("Question suppressed due to cooldown")
			return nil
		}
	}

	// Phase 2: Acquire lock before sending
	acquired, err := h.dedupMgr.AcquireLock(hookEvent, hookData.SessionID)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		logging.Debug("Failed to acquire lock (duplicate), skipping")
		return nil
	}

	logging.Debug("Lock acquired, proceeding with notification")

	// Update state (only for task_complete, PreToolUse already updated state)
	if status == analyzer.StatusTaskComplete {
		if err := h.stateMgr.UpdateTaskComplete(hookData.SessionID); err != nil {
			logging.Warn("Failed to update task complete state: %v", err)
		}
	}

	// Generate message
	message := h.generateMessage(&hookData, status)

	// Send notifications
	h.sendNotifications(status, message, hookData.SessionID)

	logging.Debug("=== Hook completed: %s ===", hookEvent)
	return nil
}

// handlePreToolUse handles PreToolUse hook
func (h *Handler) handlePreToolUse(hookData *HookData) analyzer.Status {
	logging.Debug("PreToolUse: tool_name='%s'", hookData.ToolName)

	status := analyzer.GetStatusForPreToolUse(hookData.ToolName)

	// Write session state BEFORE returning (prevents race with Notification hook)
	// This matches bash version behavior: state is written BEFORE notification is sent
	if status == analyzer.StatusPlanReady || status == analyzer.StatusQuestion {
		if err := h.stateMgr.UpdateInteractiveTool(hookData.SessionID, hookData.ToolName, hookData.CWD); err != nil {
			logging.Warn("Failed to update interactive tool state: %v", err)
		} else {
			logging.Debug("PreToolUse: session state written (tool=%s)", hookData.ToolName)
		}
	}

	return status
}

// handleNotificationEvent handles Notification hook with duplicate protection
func (h *Handler) handleNotificationEvent(hookData *HookData) (analyzer.Status, error) {
	logging.Debug("Notification event received; duplicate protection with session state + transcript")

	// 1) Try session state first (written by PreToolUse). TTL = 60 seconds.
	state, err := h.stateMgr.Load(hookData.SessionID)
	if err != nil {
		logging.Warn("Failed to load session state: %v", err)
	}

	if state != nil {
		age := platform.CurrentTimestamp() - state.LastTimestamp
		logging.Debug("Notification: session state found (tool=%s, age=%ds)", state.LastInteractiveTool, age)

		if age < 60 {
			if state.LastInteractiveTool == "ExitPlanMode" {
				logging.Debug("Notification suppressed by session state: recent ExitPlanMode (<60s)")
				return analyzer.StatusUnknown, nil
			}
			if state.LastInteractiveTool == "AskUserQuestion" {
				// PreToolUse already sent a 'question' notification; suppress Notification duplicate
				logging.Debug("Notification suppressed by session state: recent AskUserQuestion (<60s)")
				return analyzer.StatusUnknown, nil
			}
		}
	}

	// 2) Fallback: analyze transcript (temporal window) to infer state
	if hookData.TranscriptPath != "" && platform.FileExists(hookData.TranscriptPath) {
		status, err := analyzer.AnalyzeTranscript(hookData.TranscriptPath, h.cfg)
		if err != nil {
			logging.Error("Failed to analyze transcript: %v", err)
			return analyzer.StatusQuestion, nil
		}
		logging.Debug("Notification: status by transcript = %s", status)

		if status == analyzer.StatusPlanReady {
			logging.Debug("Notification suppressed: ExitPlanMode is last tool (plan already notified)")
			return analyzer.StatusUnknown, nil
		}
		if status == analyzer.StatusQuestion {
			return analyzer.StatusQuestion, nil
		}
		// Return other statuses as-is
		return status, nil
	}

	// 3) Final fallback: treat as generic question
	logging.Debug("Notification fallback → question status")
	return analyzer.StatusQuestion, nil
}

// handleStopEvent handles Stop/SubagentStop hooks
func (h *Handler) handleStopEvent(hookData *HookData) (analyzer.Status, error) {
	if hookData.TranscriptPath == "" {
		logging.Warn("Transcript path is empty, using default status")
		return analyzer.StatusTaskComplete, nil
	}

	if !platform.FileExists(hookData.TranscriptPath) {
		logging.Warn("Transcript file not found: %s", hookData.TranscriptPath)
		return analyzer.StatusTaskComplete, nil
	}

	status, err := analyzer.AnalyzeTranscript(hookData.TranscriptPath, h.cfg)
	if err != nil {
		logging.Error("Failed to analyze transcript: %v", err)
		return analyzer.StatusTaskComplete, nil
	}

	logging.Debug("Analyzed status: %s", status)
	return status, nil
}

// generateMessage generates a notification message
func (h *Handler) generateMessage(hookData *HookData, status analyzer.Status) string {
	if hookData.TranscriptPath != "" && platform.FileExists(hookData.TranscriptPath) {
		msg := summary.GenerateFromTranscript(hookData.TranscriptPath, status, h.cfg)
		if msg != "" {
			return msg
		}
	}

	return summary.GenerateSimple(status, h.cfg)
}

// sendNotifications sends desktop and webhook notifications
func (h *Handler) sendNotifications(status analyzer.Status, message, sessionID string) {
	// Add session name to message (like bash version: "[bold-cat]")
	sessionName := sessionname.GenerateSessionName(sessionID)
	enhancedMessage := fmt.Sprintf("[%s] %s", sessionName, message)

	logging.Debug("Session name: %s", sessionName)

	// Send desktop notification
	if h.cfg.IsDesktopEnabled() {
		if err := h.notifierSvc.SendDesktop(status, enhancedMessage); err != nil {
			logging.Error("Failed to send desktop notification: %v", err)
		}
	}

	// Send webhook notification (async)
	if h.cfg.IsWebhookEnabled() {
		h.webhookSvc.SendAsync(status, enhancedMessage, sessionID)
	}
}

// cleanupSession cleans up session-related files
func (h *Handler) cleanupSession(sessionID string) {
	// Delete session state
	if err := h.stateMgr.Delete(sessionID); err != nil {
		logging.Warn("Failed to delete session state: %v", err)
	}

	// Cleanup old locks (older than 60 seconds)
	if err := h.dedupMgr.Cleanup(60); err != nil {
		logging.Warn("Failed to cleanup old locks: %v", err)
	}

	// Cleanup old state files (older than 60 seconds)
	if err := h.stateMgr.Cleanup(60); err != nil {
		logging.Warn("Failed to cleanup old state files: %v", err)
	}
}
