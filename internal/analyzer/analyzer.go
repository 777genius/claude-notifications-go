package analyzer

import (
	"github.com/belief/claude-notifications/internal/config"
	"github.com/belief/claude-notifications/pkg/jsonl"
)

// Tool categories for state machine classification
var (
	ActiveTools   = []string{"Write", "Edit", "Bash", "NotebookEdit", "SlashCommand", "KillShell"}
	QuestionTools = []string{"AskUserQuestion"}
	PlanningTools = []string{"ExitPlanMode", "TodoWrite"}
	PassiveTools  = []string{"Read", "Grep", "Glob", "WebFetch", "WebSearch", "Task"}
)

// Status represents the current task status
type Status string

const (
	StatusTaskComplete   Status = "task_complete"
	StatusReviewComplete Status = "review_complete"
	StatusQuestion       Status = "question"
	StatusPlanReady      Status = "plan_ready"
	StatusUnknown        Status = "unknown"
)

// AnalyzeTranscript analyzes a transcript file and determines the current status
func AnalyzeTranscript(transcriptPath string, cfg *config.Config) (Status, error) {
	// Parse JSONL file
	messages, err := jsonl.ParseFile(transcriptPath)
	if err != nil {
		return StatusUnknown, err
	}

	// Find last user message timestamp
	// This ensures we only analyze tools from the CURRENT response,
	// not from previous user requests (avoids "ghost" ExitPlanMode problem)
	userTS := jsonl.GetLastUserTimestamp(messages)

	// Filter assistant messages AFTER last user message
	filteredMessages := jsonl.FilterMessagesAfterTimestamp(messages, userTS)

	if len(filteredMessages) == 0 {
		return StatusUnknown, nil
	}

	// Take last 15 messages (temporal window) from filtered set
	recentMessages := filteredMessages
	if len(filteredMessages) > 15 {
		recentMessages = filteredMessages[len(filteredMessages)-15:]
	}

	// Extract tools with positions
	tools := jsonl.ExtractTools(recentMessages)

	// STATE MACHINE LOGIC - tool-based detection only

	// 1. If we have tools, analyze them
	if len(tools) > 0 {
		lastTool := jsonl.GetLastTool(tools)

		// 1a. Last tool is ExitPlanMode → plan just created
		if lastTool == "ExitPlanMode" {
			return StatusPlanReady, nil
		}

		// 1b. Last tool is AskUserQuestion → waiting for user
		if lastTool == "AskUserQuestion" {
			return StatusQuestion, nil
		}

		// 1c. ExitPlanMode exists AND tools after it → plan executed
		exitPlanPos := jsonl.FindToolPosition(tools, "ExitPlanMode")
		if exitPlanPos >= 0 {
			toolsAfter := jsonl.CountToolsAfterPosition(tools, exitPlanPos)
			if toolsAfter > 0 {
				return StatusTaskComplete, nil
			}
		}

		// 1d. Last tool is active (Write/Edit/Bash) → work completed
		if contains(ActiveTools, lastTool) {
			return StatusTaskComplete, nil
		}

		// 1e. Any tool usage at all → likely task completed
		// (matches bash version: toolCount >= 1 → task_complete)
		return StatusTaskComplete, nil
	}

	// 2. No tools found → unknown (skip notification)
	return StatusUnknown, nil
}

// contains checks if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// GetStatusForPreToolUse determines status for PreToolUse hook
// This is called BEFORE tool execution, so we only have the tool name
func GetStatusForPreToolUse(toolName string) Status {
	if toolName == "ExitPlanMode" {
		return StatusPlanReady
	}
	if toolName == "AskUserQuestion" {
		return StatusQuestion
	}
	return StatusUnknown
}
