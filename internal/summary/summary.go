package summary

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/belief/claude-notifications/internal/analyzer"
	"github.com/belief/claude-notifications/internal/config"
	"github.com/belief/claude-notifications/pkg/jsonl"
)

var (
	// Regex patterns for markdown cleanup
	headerPattern     = regexp.MustCompile(`^#+\s*`)
	bulletPattern     = regexp.MustCompile(`^[-*•]\s*`)
	backtickPattern   = regexp.MustCompile("`")
	multiSpacePattern = regexp.MustCompile(`\s+`)
	emojiPattern      = regexp.MustCompile(`^[\p{So}\p{Sk}]+\s*`)
)

// GenerateFromTranscript generates a status-specific summary from transcript
func GenerateFromTranscript(transcriptPath string, status analyzer.Status, cfg *config.Config) string {
	messages, err := jsonl.ParseFile(transcriptPath)
	if err != nil {
		return GetDefaultMessage(status, cfg)
	}

	if len(messages) == 0 {
		return GetDefaultMessage(status, cfg)
	}

	// Use status-specific generators
	switch status {
	case analyzer.StatusQuestion:
		return generateQuestionSummary(messages, cfg)
	case analyzer.StatusPlanReady:
		return generatePlanSummary(messages, cfg)
	case analyzer.StatusReviewComplete:
		return generateReviewSummary(messages, cfg)
	case analyzer.StatusTaskComplete:
		return generateTaskSummary(messages, cfg)
	default:
		return generateTaskSummary(messages, cfg)
	}
}

// generateQuestionSummary generates summary for question status
// Matches bash: lib/summarizer.sh lines 416-469
func generateQuestionSummary(messages []jsonl.Message, cfg *config.Config) string {
	// 1) Try to extract AskUserQuestion tool (with recency check)
	question, isRecent := extractAskUserQuestion(messages)
	if question != "" && isRecent {
		return truncateText(question, 150)
	}

	// 2) Fallback: look for recent textual question in assistant messages
	recentMessages := jsonl.GetLastAssistantMessages(messages, 8)
	texts := jsonl.ExtractTextFromMessages(recentMessages)

	// Find last line with "?"
	for i := len(texts) - 1; i >= 0; i-- {
		if strings.Contains(texts[i], "?") {
			return truncateText(texts[i], 150)
		}
	}

	// 3) Final fallback: generic prompt
	return "Claude needs your input to continue"
}

// generatePlanSummary generates summary for plan_ready status
// Matches bash: lib/summarizer.sh lines 471-492
func generatePlanSummary(messages []jsonl.Message, cfg *config.Config) string {
	// Extract plan from ExitPlanMode tool
	plan := extractExitPlanModePlan(messages)

	if plan != "" {
		// Get first line, clean markdown
		lines := strings.Split(plan, "\n")
		firstLine := ""
		for _, line := range lines {
			cleaned := CleanMarkdown(line)
			if strings.TrimSpace(cleaned) != "" {
				firstLine = cleaned
				break
			}
		}

		if firstLine != "" {
			return truncateText(firstLine, 150)
		}
	}

	return "Plan is ready for review"
}

// generateReviewSummary generates summary for review_complete status
// Matches bash: lib/summarizer.sh lines 494-521
func generateReviewSummary(messages []jsonl.Message, cfg *config.Config) string {
	// Look for review-related messages
	recentMessages := jsonl.GetLastAssistantMessages(messages, 5)
	texts := jsonl.ExtractTextFromMessages(recentMessages)
	combined := strings.Join(texts, " ")

	// Check for review keywords
	reviewKeywords := []string{"review", "анализ", "проверка", "analyzed", "analysis"}
	for _, keyword := range reviewKeywords {
		if strings.Contains(strings.ToLower(combined), keyword) {
			// Find the sentence containing the keyword
			for _, text := range texts {
				if strings.Contains(strings.ToLower(text), keyword) {
					return truncateText(text, 150)
				}
			}
		}
	}

	// Count Read tool usage
	tools := jsonl.ExtractTools(recentMessages)
	readCount := 0
	for _, tool := range tools {
		if tool.Name == "Read" {
			readCount++
		}
	}

	if readCount > 0 {
		noun := "file"
		if readCount != 1 {
			noun = "files"
		}
		return fmt.Sprintf("Reviewed %d %s", readCount, noun)
	}

	return "Code review completed"
}

// generateTaskSummary generates summary for task_complete status
// Matches bash: lib/summarizer.sh lines 523-653
func generateTaskSummary(messages []jsonl.Message, cfg *config.Config) string {
	// Get recent assistant messages
	recentMessages := jsonl.GetLastAssistantMessages(messages, 5)
	if len(recentMessages) == 0 {
		return GetDefaultMessage(analyzer.StatusTaskComplete, cfg)
	}

	// Extract last assistant message text
	texts := jsonl.ExtractTextFromMessages(recentMessages)
	var lastMessage string
	if len(texts) > 0 {
		lastMessage = texts[len(texts)-1]
	}

	// Calculate duration and count tools
	duration := calculateDuration(messages)
	toolCounts := countToolsByType(messages)

	// Build actions string
	actions := buildActionsString(toolCounts, duration)

	// If we have both message and actions, combine them
	if lastMessage != "" {
		// Get first sentence
		firstSentence := extractFirstSentence(lastMessage)
		firstSentence = CleanMarkdown(firstSentence)

		if actions != "" {
			combined := firstSentence + ". " + actions
			return truncateText(combined, 150)
		}
		return truncateText(firstSentence, 150)
	}

	// Fallback: just actions or generic message
	if actions != "" {
		return actions
	}

	// Final fallback
	toolCount := 0
	for _, count := range toolCounts {
		toolCount += count
	}
	if toolCount > 0 {
		return fmt.Sprintf("Completed task with %d operations", toolCount)
	}

	return "Task completed successfully"
}

// extractAskUserQuestion extracts the last AskUserQuestion with recency check
// Returns (question, isRecent)
func extractAskUserQuestion(messages []jsonl.Message) (string, bool) {
	// Find last AskUserQuestion tool
	var questionText string
	var questionTimestamp string

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Type != "assistant" {
			continue
		}

		for _, content := range msg.Message.Content {
			if content.Type == "tool_use" && content.Name == "AskUserQuestion" {
				// Extract question from input.questions[0].question
				if questions, ok := content.Input["questions"].([]interface{}); ok && len(questions) > 0 {
					if q, ok := questions[0].(map[string]interface{}); ok {
						if qtext, ok := q["question"].(string); ok {
							questionText = qtext
							questionTimestamp = msg.Timestamp
							break
						}
					}
				}
			}
		}
		if questionText != "" {
			break
		}
	}

	if questionText == "" {
		return "", false
	}

	// Check recency (60s window)
	lastAssistantTS := jsonl.GetLastAssistantTimestamp(messages)
	if lastAssistantTS == "" || questionTimestamp == "" {
		return questionText, false
	}

	questionTime, err1 := time.Parse(time.RFC3339, questionTimestamp)
	lastTime, err2 := time.Parse(time.RFC3339, lastAssistantTS)

	if err1 != nil || err2 != nil {
		return questionText, false
	}

	// Check if question is within 60s of last assistant message
	age := lastTime.Sub(questionTime)
	isRecent := age >= 0 && age <= 60*time.Second

	return questionText, isRecent
}

// extractExitPlanModePlan extracts the plan text from ExitPlanMode tool
func extractExitPlanModePlan(messages []jsonl.Message) string {
	input := jsonl.ExtractToolInput(messages, "ExitPlanMode")
	if plan, ok := input["plan"].(string); ok {
		return plan
	}
	return ""
}

// calculateDuration calculates duration between last user and last assistant messages
func calculateDuration(messages []jsonl.Message) string {
	userTS := jsonl.GetLastUserTimestamp(messages)
	assistantTS := jsonl.GetLastAssistantTimestamp(messages)

	if userTS == "" || assistantTS == "" {
		return ""
	}

	userTime, err1 := time.Parse(time.RFC3339, userTS)
	assistantTime, err2 := time.Parse(time.RFC3339, assistantTS)

	if err1 != nil || err2 != nil {
		return ""
	}

	duration := assistantTime.Sub(userTime)
	if duration < 0 {
		return ""
	}

	return formatDuration(duration)
}

// formatDuration formats duration into human-readable string
func formatDuration(d time.Duration) string {
	seconds := int(d.Seconds())

	if seconds < 60 {
		return fmt.Sprintf("Took %ds", seconds)
	}

	minutes := seconds / 60
	secs := seconds % 60

	if minutes < 60 {
		if secs > 0 {
			return fmt.Sprintf("Took %dm %ds", minutes, secs)
		}
		return fmt.Sprintf("Took %dm", minutes)
	}

	hours := minutes / 60
	mins := minutes % 60

	if mins > 0 {
		return fmt.Sprintf("Took %dh %dm", hours, mins)
	}
	return fmt.Sprintf("Took %dh", hours)
}

// countToolsByType counts tools since last user message
func countToolsByType(messages []jsonl.Message) map[string]int {
	counts := make(map[string]int)

	// Find last user timestamp
	userTS := jsonl.GetLastUserTimestamp(messages)
	var sinceTime time.Time
	if userTS != "" {
		if t, err := time.Parse(time.RFC3339, userTS); err == nil {
			sinceTime = t
		}
	}

	// Count tools after user message
	for _, msg := range messages {
		if msg.Type != "assistant" {
			continue
		}

		// Check if this message is after user message
		if !sinceTime.IsZero() && msg.Timestamp != "" {
			if msgTime, err := time.Parse(time.RFC3339, msg.Timestamp); err == nil {
				if msgTime.Before(sinceTime) {
					continue
				}
			}
		}

		for _, content := range msg.Message.Content {
			if content.Type == "tool_use" {
				counts[content.Name]++
			}
		}
	}

	return counts
}

// buildActionsString builds actions summary with tool counts and duration
func buildActionsString(toolCounts map[string]int, duration string) string {
	var parts []string

	// Write
	if count := toolCounts["Write"]; count > 0 {
		noun := "file"
		if count != 1 {
			noun = "files"
		}
		parts = append(parts, fmt.Sprintf("Created %d %s", count, noun))
	}

	// Edit
	if count := toolCounts["Edit"]; count > 0 {
		noun := "file"
		if count != 1 {
			noun = "files"
		}
		parts = append(parts, fmt.Sprintf("Edited %d %s", count, noun))
	}

	// Bash
	if count := toolCounts["Bash"]; count > 0 {
		noun := "command"
		if count != 1 {
			noun = "commands"
		}
		parts = append(parts, fmt.Sprintf("Ran %d %s", count, noun))
	}

	// Add duration at the end
	if duration != "" {
		parts = append(parts, duration)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, ". ")
}

// Helper functions

func extractFirstSentence(text string) string {
	// Find first sentence (ending with . ! or ?)
	for i, char := range text {
		if char == '.' || char == '!' || char == '?' {
			return strings.TrimSpace(text[:i])
		}
	}
	// No sentence ending found, return first 100 chars
	if len(text) > 100 {
		return text[:100]
	}
	return text
}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Try to truncate at word boundary
	truncated := text[:maxLen-3]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// CleanMarkdown cleans markdown formatting from text
func CleanMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove headers
		line = headerPattern.ReplaceAllString(line, "")

		// Remove bullet points
		line = bulletPattern.ReplaceAllString(line, "")

		// Remove backticks
		line = backtickPattern.ReplaceAllString(line, "")

		// Trim again
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	// Join and normalize whitespace
	result := strings.Join(cleaned, " ")
	result = multiSpacePattern.ReplaceAllString(result, " ")

	return strings.TrimSpace(result)
}

// GetDefaultMessage returns a default message for a status
func GetDefaultMessage(status analyzer.Status, cfg *config.Config) string {
	statusInfo, exists := cfg.GetStatusInfo(string(status))
	if !exists {
		return "Claude Code notification"
	}

	// Remove emoji from title for message
	title := statusInfo.Title
	title = strings.TrimSpace(emojiPattern.ReplaceAllString(title, ""))

	return title
}

// GenerateSimple generates a simple message based on status
func GenerateSimple(status analyzer.Status, cfg *config.Config) string {
	return GetDefaultMessage(status, cfg)
}
