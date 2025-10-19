package webhook

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
)

func TestSlackFormatterFormat(t *testing.T) {
	formatter := &SlackFormatter{}
	statusInfo := config.StatusInfo{
		Title: "Task Complete",
	}

	result, err := formatter.Format(
		analyzer.StatusTaskComplete,
		"The task has been completed successfully",
		"session-123",
		statusInfo,
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify structure
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("Result should be a map")
	}

	attachments, ok := resultMap["attachments"].([]map[string]interface{})
	if !ok || len(attachments) == 0 {
		t.Fatal("Should have attachments array")
	}

	attachment := attachments[0]

	// Check color
	color, ok := attachment["color"].(string)
	if !ok || color != "#28a745" {
		t.Errorf("Expected green color #28a745, got %v", color)
	}

	// Check title
	title, ok := attachment["title"].(string)
	if !ok || title != "Task Complete" {
		t.Errorf("Expected title 'Task Complete', got %v", title)
	}

	// Check text
	text, ok := attachment["text"].(string)
	if !ok || text != "The task has been completed successfully" {
		t.Errorf("Expected message text, got %v", text)
	}

	// Check footer contains session ID
	footer, ok := attachment["footer"].(string)
	if !ok || !strings.Contains(footer, "session-123") {
		t.Errorf("Footer should contain session ID, got %v", footer)
	}

	// Verify it's valid JSON
	data, err := json.Marshal(result)
	if err != nil {
		t.Errorf("Result should be JSON-serializable: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSON data should not be empty")
	}
}

func TestSlackFormatterColors(t *testing.T) {
	formatter := &SlackFormatter{}
	statusInfo := config.StatusInfo{Title: "Test"}

	tests := []struct {
		status        analyzer.Status
		expectedColor string
	}{
		{analyzer.StatusTaskComplete, "#28a745"},
		{analyzer.StatusReviewComplete, "#17a2b8"},
		{analyzer.StatusQuestion, "#ffc107"},
		{analyzer.StatusPlanReady, "#007bff"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result, err := formatter.Format(tt.status, "test", "session-1", statusInfo)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			resultMap := result.(map[string]interface{})
			attachments := resultMap["attachments"].([]map[string]interface{})
			color := attachments[0]["color"].(string)

			if color != tt.expectedColor {
				t.Errorf("Expected color %s for %s, got %s", tt.expectedColor, tt.status, color)
			}
		})
	}
}

func TestDiscordFormatterFormat(t *testing.T) {
	formatter := &DiscordFormatter{}
	statusInfo := config.StatusInfo{
		Title: "Question",
	}

	result, err := formatter.Format(
		analyzer.StatusQuestion,
		"What should we do next?",
		"session-456",
		statusInfo,
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify structure
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("Result should be a map")
	}

	// Check username
	username, ok := resultMap["username"].(string)
	if !ok || username != "Claude Code" {
		t.Errorf("Expected username 'Claude Code', got %v", username)
	}

	// Check embeds
	embeds, ok := resultMap["embeds"].([]map[string]interface{})
	if !ok || len(embeds) == 0 {
		t.Fatal("Should have embeds array")
	}

	embed := embeds[0]

	// Check title
	title, ok := embed["title"].(string)
	if !ok || title != "Question" {
		t.Errorf("Expected title 'Question', got %v", title)
	}

	// Check description
	desc, ok := embed["description"].(string)
	if !ok || desc != "What should we do next?" {
		t.Errorf("Expected description text, got %v", desc)
	}

	// Check color is integer
	color, ok := embed["color"].(int)
	if !ok {
		t.Errorf("Color should be integer, got %T", embed["color"])
	}
	if color != 0xffc107 {
		t.Errorf("Expected yellow color 0xffc107, got 0x%x", color)
	}

	// Check footer
	footer, ok := embed["footer"].(map[string]interface{})
	if !ok {
		t.Fatal("Should have footer map")
	}

	footerText, ok := footer["text"].(string)
	if !ok || !strings.Contains(footerText, "session-456") {
		t.Errorf("Footer text should contain session ID, got %v", footerText)
	}

	// Verify JSON serializable
	data, err := json.Marshal(result)
	if err != nil {
		t.Errorf("Result should be JSON-serializable: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSON data should not be empty")
	}
}

func TestDiscordFormatterColors(t *testing.T) {
	formatter := &DiscordFormatter{}
	statusInfo := config.StatusInfo{Title: "Test"}

	tests := []struct {
		status        analyzer.Status
		expectedColor int
	}{
		{analyzer.StatusTaskComplete, 0x28a745},
		{analyzer.StatusReviewComplete, 0x17a2b8},
		{analyzer.StatusQuestion, 0xffc107},
		{analyzer.StatusPlanReady, 0x007bff},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result, err := formatter.Format(tt.status, "test", "session-1", statusInfo)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			resultMap := result.(map[string]interface{})
			embeds := resultMap["embeds"].([]map[string]interface{})
			color := embeds[0]["color"].(int)

			if color != tt.expectedColor {
				t.Errorf("Expected color 0x%x for %s, got 0x%x", tt.expectedColor, tt.status, color)
			}
		})
	}
}

func TestTelegramFormatterFormat(t *testing.T) {
	formatter := &TelegramFormatter{ChatID: "123456789"}
	statusInfo := config.StatusInfo{
		Title: "Review Complete",
	}

	result, err := formatter.Format(
		analyzer.StatusReviewComplete,
		"Code review finished",
		"session-789",
		statusInfo,
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify structure
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("Result should be a map")
	}

	// Check chat_id
	chatID, ok := resultMap["chat_id"].(string)
	if !ok || chatID != "123456789" {
		t.Errorf("Expected chat_id '123456789', got %v", chatID)
	}

	// Check parse_mode
	parseMode, ok := resultMap["parse_mode"].(string)
	if !ok || parseMode != "HTML" {
		t.Errorf("Expected parse_mode 'HTML', got %v", parseMode)
	}

	// Check text contains HTML formatting
	text, ok := resultMap["text"].(string)
	if !ok {
		t.Fatal("Should have text field")
	}

	if !strings.Contains(text, "<b>") {
		t.Error("Text should contain HTML bold tags")
	}

	if !strings.Contains(text, "Review Complete") {
		t.Error("Text should contain title")
	}

	if !strings.Contains(text, "Code review finished") {
		t.Error("Text should contain message")
	}

	if !strings.Contains(text, "session-789") {
		t.Error("Text should contain session ID")
	}

	if !strings.Contains(text, "<i>") {
		t.Error("Text should contain HTML italic tags for session")
	}

	// Verify JSON serializable
	data, err := json.Marshal(result)
	if err != nil {
		t.Errorf("Result should be JSON-serializable: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSON data should not be empty")
	}
}

func TestTelegramFormatterEmojis(t *testing.T) {
	formatter := &TelegramFormatter{ChatID: "123"}
	statusInfo := config.StatusInfo{Title: "Test"}

	tests := []struct {
		status        analyzer.Status
		expectedEmoji string
	}{
		{analyzer.StatusTaskComplete, "✅"},
		{analyzer.StatusReviewComplete, "🔍"},
		{analyzer.StatusQuestion, "❓"},
		{analyzer.StatusPlanReady, "📋"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result, err := formatter.Format(tt.status, "test", "session-1", statusInfo)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			resultMap := result.(map[string]interface{})
			text := resultMap["text"].(string)

			if !strings.Contains(text, tt.expectedEmoji) {
				t.Errorf("Expected emoji %s for %s in text: %s", tt.expectedEmoji, tt.status, text)
			}
		})
	}
}

func TestGetColorForStatus(t *testing.T) {
	tests := []struct {
		status   analyzer.Status
		expected string
	}{
		{analyzer.StatusTaskComplete, "#28a745"},
		{analyzer.StatusReviewComplete, "#17a2b8"},
		{analyzer.StatusQuestion, "#ffc107"},
		{analyzer.StatusPlanReady, "#007bff"},
		{analyzer.Status("unknown"), "#6c757d"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := getColorForStatus(tt.status)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetDiscordColorInt(t *testing.T) {
	tests := []struct {
		status   analyzer.Status
		expected int
	}{
		{analyzer.StatusTaskComplete, 0x28a745},
		{analyzer.StatusReviewComplete, 0x17a2b8},
		{analyzer.StatusQuestion, 0xffc107},
		{analyzer.StatusPlanReady, 0x007bff},
		{analyzer.Status("unknown"), 0x6c757d},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := getDiscordColorInt(tt.status)
			if result != tt.expected {
				t.Errorf("Expected 0x%x, got 0x%x", tt.expected, result)
			}
		})
	}
}

func TestGetEmojiForStatus(t *testing.T) {
	tests := []struct {
		status   analyzer.Status
		expected string
	}{
		{analyzer.StatusTaskComplete, "✅"},
		{analyzer.StatusReviewComplete, "🔍"},
		{analyzer.StatusQuestion, "❓"},
		{analyzer.StatusPlanReady, "📋"},
		{analyzer.Status("unknown"), "ℹ️"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := getEmojiForStatus(tt.status)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
