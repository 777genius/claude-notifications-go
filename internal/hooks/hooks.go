package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/dedup"
	"github.com/777genius/claude-notifications/internal/errorhandler"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/notifier"
	"github.com/777genius/claude-notifications/internal/platform"
	"github.com/777genius/claude-notifications/internal/sessionname"
	"github.com/777genius/claude-notifications/internal/state"
	"github.com/777genius/claude-notifications/internal/summary"
	"github.com/777genius/claude-notifications/internal/webhook"
)

// HookData represents the data received from Claude Code hooks
type HookData struct {
	TranscriptPath string `json:"transcript_path"`
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	ToolName       string `json:"tool_name,omitempty"`
	HookEventName  string `json:"hook_event_name,omitempty"`
}

// notifierInterface defines the interface for sending desktop notifications
type notifierInterface interface {
	SendDesktop(status analyzer.Status, message string) error
	Close() error
}

// webhookInterface defines the interface for sending webhook notifications
type webhookInterface interface {
	SendAsync(status analyzer.Status, message, sessionID string)
	Shutdown(timeout time.Duration) error
}

// Handler handles hook events
type Handler struct {
	cfg         *config.Config
	dedupMgr    *dedup.Manager
	stateMgr    *state.Manager
	notifierSvc notifierInterface
	webhookSvc  webhookInterface
	pluginRoot  string
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
	// Add panic recovery for robustness
	defer errorhandler.HandlePanic()

	// Skip notifications when running in background judge mode (e.g., double-shot-latte plugin)
	// The CLAUDE_HOOK_JUDGE_MODE env var is set by plugins that spawn background Claude instances
	// to evaluate context/decide on continuation - we don't want notifications from these
	if os.Getenv("CLAUDE_HOOK_JUDGE_MODE") == "true" {
		return nil
	}

	// Ensure notifier resources are cleaned up when function exits
	defer func() {
		if err := h.notifierSvc.Close(); err != nil {
			logging.Warn("Failed to close notifier: %v", err)
		}
	}()

	// Ensure webhook sender waits for in-flight requests before exit
	defer func() {
		if err := h.webhookSvc.Shutdown(5 * time.Second); err != nil {
			logging.Warn("Failed to shutdown webhook sender: %v", err)
		}
	}()

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

	// Phase 1: Early duplicate check (per hook event type)
	if h.dedupMgr.CheckEarlyDuplicate(hookData.SessionID, hookEvent) {
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
	case "Stop":
		// Analyze the transcript to determine status
		status, err = h.handleStopEvent(&hookData)
		if err != nil {
			return err
		}
		// Note: We don't delete session state here to preserve cooldown info
		// State files have TTL and will be cleaned up automatically
		defer h.cleanupOldLocks()
	case "SubagentStop":
		// Check config: should we notify on subagent completion?
		if !h.cfg.Notifications.NotifyOnSubagentStop {
			logging.Debug("SubagentStop: notifications disabled (config), skipping")
			return nil
		}
		// If enabled, handle like Stop
		logging.Debug("SubagentStop: notifications enabled (config), processing")
		status, err = h.handleStopEvent(&hookData)
		if err != nil {
			return err
		}
		defer h.cleanupOldLocks()
	default:
		return fmt.Errorf("unknown hook event: %s", hookEvent)
	}

	// If status is unknown, skip
	if status == analyzer.StatusUnknown {
		logging.Debug("Status is unknown, skipping notification")
		return nil
	}

	// Phase 2: Acquire lock before sending (per hook event type)
	acquired, err := h.dedupMgr.AcquireLock(hookData.SessionID, hookEvent)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		logging.Debug("Failed to acquire lock (duplicate), skipping")
		return nil
	}

	logging.Debug("Lock acquired, proceeding with notification")
	// Note: Lock is NOT released - it ages out naturally after 2s to prevent rapid duplicates

	// Check cooldown for question status BEFORE updating notification time
	if status == analyzer.StatusQuestion {
		logging.Debug("Checking question cooldown: cooldownSeconds=%d", h.cfg.Notifications.SuppressQuestionAfterAnyNotificationSeconds)

		// Load state to log its contents
		sessionState, stateErr := h.stateMgr.Load(hookData.SessionID)
		if stateErr != nil {
			logging.Warn("Failed to load state for logging: %v", stateErr)
		} else if sessionState != nil {
			logging.Debug("Session state: lastNotificationTime=%d, lastNotificationStatus=%s",
				sessionState.LastNotificationTime, sessionState.LastNotificationStatus)
		} else {
			logging.Debug("No session state found")
		}

		// First, check if we should suppress question after ANY notification (not just task_complete)
		suppressAfterAny, err := h.stateMgr.ShouldSuppressQuestionAfterAnyNotification(
			hookData.SessionID,
			h.cfg.Notifications.SuppressQuestionAfterAnyNotificationSeconds,
		)
		if err != nil {
			logging.Warn("Failed to check cooldown after any notification: %v", err)
		} else if suppressAfterAny {
			logging.Debug("Question suppressed due to recent notification from this session")
			// Lock will be released by defer
			return nil
		} else {
			logging.Debug("Question NOT suppressed (cooldown check passed)")
		}

		// Also check legacy cooldown after task_complete
		suppress, err := h.stateMgr.ShouldSuppressQuestion(
			hookData.SessionID,
			h.cfg.Notifications.SuppressQuestionAfterTaskCompleteSeconds,
		)
		if err != nil {
			logging.Warn("Failed to check cooldown: %v", err)
		} else if suppress {
			logging.Debug("Question suppressed due to cooldown after task complete")
			// Lock will be released by defer
			return nil
		}
	}

	// Update state (only for task_complete, PreToolUse already updated state)
	if status == analyzer.StatusTaskComplete {
		if err := h.stateMgr.UpdateTaskComplete(hookData.SessionID); err != nil {
			logging.Warn("Failed to update task complete state: %v", err)
		}
	}

	// Generate message
	message := h.generateMessage(&hookData, status)

	// Acquire content lock to prevent race between different hooks (Stop vs Notification)
	// This ensures only one process can check and update duplicate state at a time
	contentLockAcquired, err := h.dedupMgr.AcquireContentLock(hookData.SessionID)
	if err != nil {
		logging.Warn("Failed to acquire content lock: %v", err)
		// Error (not "lock busy") - continue without lock as fallback
	} else if !contentLockAcquired {
		// Lock is held by another process - it's already handling this notification
		logging.Debug("Content lock held by another process, skipping to prevent duplicate")
		return nil
	}

	// Release lock on exit if acquired
	defer func() {
		if contentLockAcquired {
			if err := h.dedupMgr.ReleaseContentLock(hookData.SessionID); err != nil {
				logging.Warn("Failed to release content lock: %v", err)
			}
		}
	}()

	// Check for duplicate message content (3 minutes = 180 seconds window)
	isDuplicate, err := h.stateMgr.IsDuplicateMessage(hookData.SessionID, message, 180)
	if err != nil {
		logging.Warn("Failed to check duplicate message: %v", err)
	} else if isDuplicate {
		logging.Debug("Duplicate message content detected within 3 minutes, skipping")
		return nil
	}

	// Update last notification time and message
	if err := h.stateMgr.UpdateLastNotification(hookData.SessionID, status, message); err != nil {
		logging.Warn("Failed to update last notification: %v", err)
	}

	// Send notifications
	h.sendNotifications(status, message, hookData.SessionID, hookData.CWD)

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

// handleNotificationEvent handles Notification hook
// Always returns StatusQuestion as per design: Notification hook is triggered
// when Claude needs user input (e.g., permission dialogs, questions)
func (h *Handler) handleNotificationEvent(hookData *HookData) (analyzer.Status, error) {
	logging.Debug("Notification event received → question status")
	return analyzer.StatusQuestion, nil
}

// handleStopEvent handles Stop/SubagentStop hooks
func (h *Handler) handleStopEvent(hookData *HookData) (analyzer.Status, error) {
	if hookData.TranscriptPath == "" {
		logging.Warn("Transcript path is empty, skipping notification")
		return analyzer.StatusUnknown, nil
	}

	if !platform.FileExists(hookData.TranscriptPath) {
		logging.Warn("Transcript file not found: %s", hookData.TranscriptPath)
		return analyzer.StatusUnknown, nil
	}

	status, err := analyzer.AnalyzeTranscript(hookData.TranscriptPath, h.cfg)
	if err != nil {
		logging.Error("Failed to analyze transcript: %v", err)
		return analyzer.StatusUnknown, nil
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
func (h *Handler) sendNotifications(status analyzer.Status, message, sessionID, cwd string) {
	// Add panic recovery to prevent notification failures from crashing the plugin
	defer errorhandler.HandlePanic()

	// Add session name, git branch and folder name to message
	sessionName := sessionname.GenerateSessionName(sessionID)
	gitBranch := platform.GetGitBranch(cwd)
	folderName := filepath.Base(cwd)

	// Format: "[session-name|branch folder] message" or "[session-name folder] message"
	var enhancedMessage string
	if gitBranch != "" {
		enhancedMessage = fmt.Sprintf("[%s|%s %s] %s", sessionName, gitBranch, folderName, message)
	} else {
		enhancedMessage = fmt.Sprintf("[%s %s] %s", sessionName, folderName, message)
	}

	logging.Debug("Session name: %s, git branch: %s, folder: %s", sessionName, gitBranch, folderName)

	// Send desktop notification
	if h.cfg.IsDesktopEnabled() {
		if err := h.notifierSvc.SendDesktop(status, enhancedMessage); err != nil {
			errorhandler.HandleError(err, "Failed to send desktop notification")
		}
	}

	// Send webhook notification (async)
	if h.cfg.IsWebhookEnabled() {
		h.webhookSvc.SendAsync(status, enhancedMessage, sessionID)
	}
}

// cleanupOldLocks cleans up old lock and state files but preserves session state for cooldown
func (h *Handler) cleanupOldLocks() {
	// Cleanup old locks (older than 60 seconds)
	if err := h.dedupMgr.Cleanup(60); err != nil {
		logging.Warn("Failed to cleanup old locks: %v", err)
	}

	// Cleanup old state files (older than 60 seconds)
	if err := h.stateMgr.Cleanup(60); err != nil {
		logging.Warn("Failed to cleanup old state files: %v", err)
	}
}
