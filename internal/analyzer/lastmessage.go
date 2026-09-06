package analyzer

import "strings"

// maxErrorMessageLength bounds the error-pattern heuristic: long final
// messages are usually task summaries that merely mention an error word,
// while genuine failure reports from the host are short.
const maxErrorMessageLength = 300

// errorMessagePatterns are matched case-insensitively against a SHORT final
// assistant message. Patterns are failure phrasings, not bare words like
// "error"/"failed", to keep false positives on ordinary summaries low.
// This is a documented MVP heuristic (see ClassifyLastMessage).
var errorMessagePatterns = []struct {
	pattern string
	status  Status
}{
	{pattern: "session limit", status: StatusSessionLimitReached},
	{pattern: "usage limit", status: StatusSessionLimitReached},
	{pattern: "rate limit", status: StatusAPIErrorOverloaded},
	{pattern: "rate-limited", status: StatusAPIErrorOverloaded},
	{pattern: "quota exceeded", status: StatusAPIErrorOverloaded},
	{pattern: "overloaded", status: StatusAPIErrorOverloaded},
	{pattern: "too many requests", status: StatusAPIErrorOverloaded},
	{pattern: "authentication failed", status: StatusAPIError},
	{pattern: "unauthorized", status: StatusAPIError},
	{pattern: "invalid api key", status: StatusAPIError},
	{pattern: "api error", status: StatusAPIError},
	{pattern: "stream error", status: StatusAPIError},
	{pattern: "context window exceeded", status: StatusAPIError},
}

// detectMessageErrorStatus applies the error heuristic to a trimmed final
// message; StatusUnknown means "no error detected".
func detectMessageErrorStatus(trimmed string) Status {
	if len(trimmed) == 0 || len(trimmed) > maxErrorMessageLength {
		return StatusUnknown
	}
	lower := strings.ToLower(trimmed)
	for _, p := range errorMessagePatterns {
		if strings.Contains(lower, p.pattern) {
			return p.status
		}
	}
	return StatusUnknown
}

// ClassifyLastMessage derives a status from a host-provided final assistant
// message when no Claude-format transcript is available (the Codex path).
//
// This is a deliberate MVP simplification: Codex delivers the final message
// directly in the Stop payload and its rollout transcript format is not
// parsed, so the rich tool-based state machine above does not apply. An empty
// message still means the turn finished, so the default is task_complete,
// with a light question heuristic on top.
func ClassifyLastMessage(text string) Status {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return StatusTaskComplete
	}
	// Error reports take precedence: a short failure message must not be
	// announced as a completed task.
	if status := detectMessageErrorStatus(trimmed); status != StatusUnknown {
		return status
	}
	// Question marks across scripts: ASCII, fullwidth (CJK), and Arabic.
	// The Greek question mark shares its codepoint with the semicolon and
	// would misclassify ordinary text, so it stays out.
	for _, suffix := range []string{"?", "？", "؟"} {
		if strings.HasSuffix(trimmed, suffix) {
			return StatusQuestion
		}
	}
	return StatusTaskComplete
}
