package analyzer

import "strings"

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
	if strings.HasSuffix(trimmed, "?") {
		return StatusQuestion
	}
	return StatusTaskComplete
}
