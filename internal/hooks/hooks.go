package hooks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/benchmark"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/dedup"
	"github.com/777genius/claude-notifications/internal/errorhandler"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/notifier"
	"github.com/777genius/claude-notifications/internal/platform"
	"github.com/777genius/claude-notifications/internal/sessionname"
	"github.com/777genius/claude-notifications/internal/state"
	"github.com/777genius/claude-notifications/internal/summary"
	"github.com/777genius/claude-notifications/internal/teamstate"
	"github.com/777genius/claude-notifications/internal/webhook"
	"github.com/777genius/claude-notifications/pkg/jsonl"
)

// HookData represents the data received from Claude Code hooks
type HookData struct {
	TranscriptPath string `json:"transcript_path"`
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	ToolName       string `json:"tool_name,omitempty"`
	HookEventName  string `json:"hook_event_name,omitempty"`
	// Team-related fields (present in TeammateIdle, TaskCreated, TaskCompleted hooks)
	TeamName     string `json:"team_name,omitempty"`
	TeammateName string `json:"teammate_name,omitempty"`
}

// notifierInterface defines the interface for sending desktop notifications
type notifierInterface interface {
	SendDesktop(status analyzer.Status, message, sessionID, cwd string) error
	Close() error
}

// webhookInterface defines the interface for sending webhook notifications
type webhookInterface interface {
	SendAsyncWithContext(sendCtx webhook.SendContext)
	Shutdown(timeout time.Duration) error
}

// Handler handles hook events
type Handler struct {
	cfg          *config.Config
	dedupMgr     *dedup.Manager
	stateMgr     *state.Manager
	teamStateMgr *teamstate.Manager
	notifierSvc  notifierInterface
	webhookSvc   webhookInterface
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
		cfg:          cfg,
		dedupMgr:     dedup.NewManager(),
		stateMgr:     state.NewManager(),
		teamStateMgr: teamstate.NewManager(""),
		notifierSvc:  notifier.New(cfg),
		webhookSvc:   webhook.New(cfg),
		pluginRoot:   pluginRoot,
	}, nil
}

// HandleHook handles a hook event
func (h *Handler) HandleHook(hookEvent string, input io.Reader) error {
	// Benchmark instrumentation (enabled via config debug.benchmark)
	bench := benchmark.New(h.cfg.IsBenchmarkEnabled(), logging.Info)
	bench.Start("hook.total")
	defer func() {
		bench.Elapsed("hook.total")
		bench.Report()
	}()

	// Add panic recovery for robustness
	defer errorhandler.HandlePanic()

	// Skip notifications when running in background judge mode (e.g., double-shot-latte plugin)
	// The CLAUDE_HOOK_JUDGE_MODE env var is set by plugins that spawn background Claude instances
	// to evaluate context/decide on continuation - we don't want notifications from these
	// Can be disabled via config: "respectJudgeMode": false
	if h.cfg.ShouldRespectJudgeMode() && os.Getenv("CLAUDE_HOOK_JUDGE_MODE") == "true" {
		return nil
	}

	// Ensure notifier resources are cleaned up when function exits
	defer func() {
		bench.Start("notifier.close")
		if err := h.notifierSvc.Close(); err != nil {
			logging.Warn("Failed to close notifier: %v", err)
		}
		bench.Elapsed("notifier.close")
	}()

	// Ensure webhook sender waits for in-flight requests before exit
	defer func() {
		bench.Start("webhook.shutdown")
		if err := h.webhookSvc.Shutdown(5 * time.Second); err != nil {
			logging.Warn("Failed to shutdown webhook sender: %v", err)
		}
		bench.Elapsed("webhook.shutdown")
	}()

	logging.SetPrefix(fmt.Sprintf("PID:%d", os.Getpid()))
	logging.Debug("=== Hook triggered: %s ===", hookEvent)

	// Parse hook data
	bench.Start("stdin.parse")
	var hookData HookData
	if err := json.NewDecoder(skipUTF8BOM(input)).Decode(&hookData); err != nil {
		return fmt.Errorf("failed to parse hook data: %w", err)
	}
	bench.Elapsed("stdin.parse")

	logging.Debug("Hook data: session=%s, transcript=%s, tool=%s",
		hookData.SessionID, hookData.TranscriptPath, hookData.ToolName)

	// Validate session ID
	if hookData.SessionID == "" {
		hookData.SessionID = "unknown"
		logging.Warn("Session ID is empty, using 'unknown'")
	}

	if h.cfg.Notifications.Desktop.ClickToFocus && (hookEvent == "PreToolUse" || hookEvent == "Notification") {
		notifier.MaybeCaptureGhosttyTerminalID(
			h.cfg.Notifications.Desktop.TerminalBundleID,
			hookData.SessionID,
			hookData.CWD,
		)
	}

	// Phase 1: Early duplicate check (per hook event type)
	bench.Start("dedup.early_check")
	if h.dedupMgr.CheckEarlyDuplicate(hookData.SessionID, hookEvent) {
		bench.Elapsed("dedup.early_check")
		logging.Debug("Early duplicate detected, skipping")
		return nil
	}
	bench.Elapsed("dedup.early_check")

	// Check if any notification method is enabled
	if !h.cfg.IsAnyNotificationEnabled() {
		logging.Debug("All notifications disabled, exiting")
		return nil
	}

	// Determine status based on hook type
	var status analyzer.Status
	var parsedMessages []jsonl.Message // reused by generateMessage to avoid double I/O
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
		// A Stop event is the MAIN agent finishing, so suppress only when its
		// transcript_path actually points at a subagent/teammate transcript
		// (.../subagents/...). Note: on current Claude Code the Stop hook receives
		// the parent session transcript, so this rarely matches — kept as a
		// forward-compatible guard for transcripts that are routed differently.
		if h.cfg.ShouldSuppressForSubagents() && isSubagentTranscript(hookData.TranscriptPath) {
			logging.Debug("Stop: subagent transcript detected (%s), suppressing (config: suppressForSubagents)", hookData.TranscriptPath)
			return nil
		}

		// Team mode: check if this session is a team lead and suppress if needed
		if h.cfg.GetTeamMode() == "wait-all" {
			if teamInfo := h.teamStateMgr.DetectTeamLead(hookData.SessionID); teamInfo != nil {
				logging.Debug("Stop: team lead detected for team %q (members: %d), checking team state",
					teamInfo.TeamName, len(teamInfo.Members))

				// Record that the lead has stopped
				if err := h.teamStateMgr.RecordLeadStopped(teamInfo.TeamName); err != nil {
					logging.Warn("Stop: failed to record lead stopped: %v", err)
				}

				// Check if all teammates are already idle
				allIdle, err := h.teamStateMgr.CheckAllIdle(teamInfo.TeamName, teamInfo.Members)
				if err != nil {
					logging.Warn("Stop: failed to check team idle state: %v", err)
				}

				if !allIdle {
					// Not all teammates idle yet — suppress notification, wait for TeammateIdle
					logging.Debug("Stop: team %q has active teammates, suppressing notification", teamInfo.TeamName)
					return nil
				}

				// All teammates are idle — proceed with notification and mark as notified
				logging.Debug("Stop: team %q all teammates idle, sending notification", teamInfo.TeamName)
				if err := h.teamStateMgr.MarkNotified(teamInfo.TeamName); err != nil {
					logging.Warn("Stop: failed to mark team notified: %v", err)
				}
			}
		} else if h.cfg.GetTeamMode() == "never" {
			if teamInfo := h.teamStateMgr.DetectTeamLead(hookData.SessionID); teamInfo != nil {
				logging.Debug("Stop: team mode is 'never', suppressing for team %q", teamInfo.TeamName)
				return nil
			}
		}
		// teamMode "always" or not a team lead: fall through to normal processing

		// Analyze the transcript to determine status
		bench.Start("stop.analyze")
		status, parsedMessages, err = h.handleStopEvent(&hookData)
		bench.Elapsed("stop.analyze")
		if err != nil {
			return err
		}
		// Note: We don't delete session state here to preserve cooldown info
		// State files have TTL and will be cleaned up automatically
		defer h.cleanupOldLocks()
	case "SubagentStop":
		// A SubagentStop event always denotes a subagent (Task tool) finishing,
		// so the event type itself — not the transcript path — is the reliable
		// subagent signal. Claude Code passes the PARENT session transcript_path
		// to this hook (e.g. .../<session>.jsonl), NOT the subagent's
		// .../<session>/subagents/agent-*.jsonl file, so isSubagentTranscript()
		// never matches here. Suppress by the event so suppressForSubagents works
		// as a safety net regardless of notifyOnSubagentStop.
		if h.cfg.ShouldSuppressForSubagents() {
			logging.Debug("SubagentStop: suppressing subagent notification (config: suppressForSubagents)")
			return nil
		}
		// Not globally suppressed — honor the explicit opt-in flag.
		if !h.cfg.Notifications.NotifyOnSubagentStop {
			logging.Debug("SubagentStop: notifications disabled (config: notifyOnSubagentStop), skipping")
			return nil
		}
		// Opted in and not suppressed: handle like Stop.
		logging.Debug("SubagentStop: notifications enabled (config), processing")
		bench.Start("stop.analyze")
		status, parsedMessages, err = h.handleStopEvent(&hookData)
		bench.Elapsed("stop.analyze")
		if err != nil {
			return err
		}
		defer h.cleanupOldLocks()
	case "TeammateIdle":
		return h.handleTeammateIdle(&hookData)
	default:
		return fmt.Errorf("unknown hook event: %s", hookEvent)
	}

	// If status is unknown, skip
	if status == analyzer.StatusUnknown {
		logging.Debug("Status is unknown, skipping notification")
		return nil
	}

	// Check suppress-filters before any state mutations (dedup lock, cooldowns)
	bench.Start("git.branch")
	{
		gitBranch := platform.GetGitBranch(hookData.CWD)
		bench.Elapsed("git.branch")
		folderName := filepath.Base(hookData.CWD)
		if h.cfg.ShouldFilter(string(status), gitBranch, folderName) {
			logging.Debug("Notification suppressed by filter: status=%s branch=%q folder=%s", status, gitBranch, folderName)
			return nil
		}
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
		logging.Debug("Checking question cooldown: cooldownSeconds=%d", h.cfg.GetSuppressQuestionAfterAnyNotificationSeconds())

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
			h.cfg.GetSuppressQuestionAfterAnyNotificationSeconds(),
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
			h.cfg.GetSuppressQuestionAfterTaskCompleteSeconds(),
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
	bench.Start("message.generate")
	body, actions := h.generateMessage(&hookData, status, parsedMessages)
	message := joinMessageParts(body, actions)
	bench.Elapsed("message.generate")

	// Acquire content lock to prevent race between different hooks (Stop vs Notification)
	// This ensures only one process can check and update duplicate state at a time
	contentLockAcquired, err := h.dedupMgr.AcquireContentLock(hookData.SessionID)
	if err != nil {
		logging.Warn("Failed to acquire content lock: %v", err)
		// Error (not "lock busy") - continue without lock as fallback
	} else if !contentLockAcquired {
		// Lock is held by another process - it's already handling this notification
		logging.Warn("Content lock held by another process: session=%s hook=%s (notification skipped)", hookData.SessionID, hookEvent)
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
	bench.Start("notify.send")
	h.sendNotifications(status, body, actions, hookData.SessionID, hookData.CWD)
	bench.Elapsed("notify.send")

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

// handleTeammateIdle handles the TeammateIdle hook event.
// Records the teammate as idle, checks if all teammates are idle + lead stopped,
// and sends a notification when both conditions are met.
func (h *Handler) handleTeammateIdle(hookData *HookData) error {
	if hookData.TeamName == "" || hookData.TeammateName == "" {
		logging.Debug("TeammateIdle: missing team_name or teammate_name, skipping")
		return nil
	}

	teamMode := h.cfg.GetTeamMode()
	if teamMode != "wait-all" {
		logging.Debug("TeammateIdle: teamMode=%q, skipping (only active in wait-all mode)", teamMode)
		return nil
	}

	// Dedup: prevent rapid duplicate TeammateIdle events for the same teammate
	dedupKey := hookData.SessionID + "-" + hookData.TeammateName
	if h.dedupMgr.CheckEarlyDuplicate(dedupKey, "TeammateIdle") {
		logging.Debug("TeammateIdle: duplicate for %q, skipping", hookData.TeammateName)
		return nil
	}

	logging.Debug("TeammateIdle: teammate=%q team=%q", hookData.TeammateName, hookData.TeamName)

	// Get team info to know all expected members
	teamInfo := h.teamStateMgr.DetectTeamByName(hookData.TeamName)
	if teamInfo == nil {
		logging.Debug("TeammateIdle: team %q config not found, skipping", hookData.TeamName)
		return nil
	}

	// Record this teammate as idle
	if err := h.teamStateMgr.RecordTeammateIdle(hookData.TeamName, hookData.TeammateName); err != nil {
		logging.Warn("TeammateIdle: failed to record idle state: %v", err)
		return nil
	}

	// Check if all conditions are met: lead stopped + all teammates idle
	allIdle, err := h.teamStateMgr.CheckAllIdle(hookData.TeamName, teamInfo.Members)
	if err != nil {
		logging.Warn("TeammateIdle: failed to check team idle state: %v", err)
		return nil
	}

	if !allIdle {
		logging.Debug("TeammateIdle: not all conditions met yet for team %q", hookData.TeamName)
		return nil
	}

	// All conditions met — send notification
	logging.Debug("TeammateIdle: all teammates idle + lead stopped for team %q, sending notification", hookData.TeamName)

	if err := h.teamStateMgr.MarkNotified(hookData.TeamName); err != nil {
		logging.Warn("TeammateIdle: failed to mark team notified: %v", err)
	}

	status := analyzer.StatusTaskComplete
	body := fmt.Sprintf("Team %q: all teammates finished work", hookData.TeamName)

	h.sendNotifications(status, body, "", hookData.SessionID, hookData.CWD)

	logging.Debug("=== Hook completed: TeammateIdle (team notification sent) ===")
	return nil
}

func skipUTF8BOM(input io.Reader) io.Reader {
	reader := bufio.NewReader(input)
	prefix, err := reader.Peek(3)
	if err == nil && bytes.Equal(prefix, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = reader.Discard(3)
	}
	return reader
}

// handleStopEvent handles Stop/SubagentStop hooks.
// Returns the parsed messages alongside the status so callers can reuse them
// (e.g., for summary generation) without re-reading the transcript file.
func (h *Handler) handleStopEvent(hookData *HookData) (analyzer.Status, []jsonl.Message, error) {
	if hookData.TranscriptPath == "" {
		logging.Warn("Transcript path is empty, skipping notification")
		return analyzer.StatusUnknown, nil, nil
	}

	if !platform.FileExists(hookData.TranscriptPath) {
		logging.Warn("Transcript file not found: %s", hookData.TranscriptPath)
		return analyzer.StatusUnknown, nil, nil
	}

	status, messages, err := analyzer.AnalyzeTranscriptWithMessages(hookData.TranscriptPath, h.cfg)
	if err != nil {
		logging.Error("Failed to analyze transcript: %v", err)
		return analyzer.StatusUnknown, nil, nil
	}

	logging.Debug("Analyzed status: %s", status)
	return status, messages, nil
}

// generateMessage generates a notification body and action summary.
// If messages are provided (from handleStopEvent), uses them directly to avoid re-reading the transcript.
func (h *Handler) generateMessage(hookData *HookData, status analyzer.Status, messages []jsonl.Message) (body, actions string) {
	// Use pre-parsed messages if available (eliminates ~234ms double I/O)
	if len(messages) > 0 {
		body, actions = summary.GenerateFromMessagesStructured(messages, status, h.cfg)
	} else if hookData.TranscriptPath != "" && platform.FileExists(hookData.TranscriptPath) {
		// Fallback: read transcript from file (for non-Stop hooks)
		if parsed, err := jsonl.ParseFile(hookData.TranscriptPath); err == nil {
			body, actions = summary.GenerateFromMessagesStructured(parsed, status, h.cfg)
		}
	}

	if body == "" {
		body = summary.GenerateSimple(status, h.cfg)
	}
	return body, actions
}

// joinMessageParts mirrors summary.appendActions: joins body and actions with a
// single space when actions is non-empty.
func joinMessageParts(body, actions string) string {
	if actions == "" {
		return body
	}
	return body + " " + actions
}

// sendNotifications sends desktop and webhook notifications.
//
// body is the summary text (no metadata prefix, no action segments).
// actions is the formatted action summary (e.g. "📝 1 new  ▶ 2 cmds  ⏱ 41s") or "".
func (h *Handler) sendNotifications(status analyzer.Status, body, actions, sessionID, cwd string) {
	// Add panic recovery to prevent notification failures from crashing the plugin
	defer errorhandler.HandlePanic()

	sessionName := sessionname.GenerateSessionLabel(sessionID)
	gitBranch := platform.GetGitBranch(cwd)
	folderName := filepath.Base(cwd)

	joined := joinMessageParts(body, actions)

	// Format: "[sessionname|branch folder] message" or "[sessionname folder] message"
	var enhancedMessage string
	if gitBranch != "" {
		enhancedMessage = fmt.Sprintf("[%s|%s %s] %s", sessionName, gitBranch, folderName, joined)
	} else {
		enhancedMessage = fmt.Sprintf("[%s %s] %s", sessionName, folderName, joined)
	}

	logging.Debug("Session name: %s, git branch: %s, folder: %s", sessionName, gitBranch, folderName)

	statusStr := string(status)

	// Send desktop notification (check per-status enabled)
	if h.cfg.IsStatusDesktopEnabled(statusStr) {
		if err := h.notifierSvc.SendDesktop(status, enhancedMessage, sessionID, cwd); err != nil {
			h.maybeEmitDesktopPermissionGuidance(err)
			errorhandler.HandleError(err, "Failed to send desktop notification")
		}
	} else {
		logging.Debug("Desktop notification disabled for status: %s", statusStr)
	}

	// Send webhook notification (async, check per-status enabled)
	if h.cfg.IsStatusWebhookEnabled(statusStr) {
		h.webhookSvc.SendAsyncWithContext(webhook.SendContext{
			Status:        status,
			Message:       enhancedMessage,
			SessionID:     sessionID,
			CWD:           cwd,
			SessionName:   sessionName,
			GitBranch:     gitBranch,
			Folder:        folderName,
			RawBody:       body,
			ActionSummary: actions,
		})
	} else {
		logging.Debug("Webhook notification disabled for status: %s", statusStr)
	}
}

// isSubagentTranscript checks if the transcript path indicates a subagent session.
// Claude Code stores subagent transcripts in paths containing /subagents/ segment.
func isSubagentTranscript(transcriptPath string) bool {
	// Normalize path separators for cross-platform compatibility
	normalized := filepath.ToSlash(transcriptPath)
	return strings.Contains(normalized, "/subagents/")
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

func (h *Handler) maybeEmitDesktopPermissionGuidance(err error) {
	if !platform.IsMacOS() {
		return
	}

	var permissionErr *notifier.NotificationPermissionDeniedError
	if !errors.As(err, &permissionErr) {
		return
	}

	if !h.shouldEmitPermissionGuidance() {
		return
	}

	message := "[claude-notifications] macOS is blocking ClaudeNotifier notifications. Open System Settings > Notifications > Claude Notifier and enable notifications. This can happen after older ad-hoc installs or stale notification permissions."
	fmt.Printf("{\"systemMessage\":%q}\n", message)
}

func (h *Handler) shouldEmitPermissionGuidance() bool {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		return true
	}

	stampDir := filepath.Join(cacheDir, "claude-notifications-go")
	stampPath := filepath.Join(stampDir, "macos-notification-permission-reminder")

	if info, err := os.Stat(stampPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return false
		}
	}

	if err := os.MkdirAll(stampDir, 0o755); err != nil {
		return true
	}
	if err := os.WriteFile(stampPath, []byte(time.Now().Format(time.RFC3339)), 0o644); err != nil {
		return true
	}

	return true
}
