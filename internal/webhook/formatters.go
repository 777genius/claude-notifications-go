package webhook

import (
	"fmt"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
)

// Formatter interface for different webhook formats
type Formatter interface {
	Format(status analyzer.Status, message, sessionID string, statusInfo config.StatusInfo) (interface{}, error)
}

// SlackFormatter formats messages for Slack
type SlackFormatter struct{}

func (f *SlackFormatter) Format(status analyzer.Status, message, sessionID string, statusInfo config.StatusInfo) (interface{}, error) {
	color := getColorForStatus(status)

	return map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":       color,
				"title":       statusInfo.Title,
				"text":        message,
				"footer":      fmt.Sprintf("Session: %s | Claude Notifications", sessionID),
				"footer_icon": "https://claude.ai/favicon.ico",
				"ts":          time.Now().Unix(),
				"mrkdwn_in":   []string{"text"},
			},
		},
	}, nil
}

// DiscordFormatter formats messages for Discord with embeds
type DiscordFormatter struct{}

func (f *DiscordFormatter) Format(status analyzer.Status, message, sessionID string, statusInfo config.StatusInfo) (interface{}, error) {
	colorInt := getDiscordColorInt(status)

	return map[string]interface{}{
		"username": "Claude Code",
		"embeds": []map[string]interface{}{
			{
				"title":       statusInfo.Title,
				"description": message,
				"color":       colorInt,
				"footer": map[string]interface{}{
					"text": fmt.Sprintf("Session: %s", sessionID),
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}, nil
}

// TelegramFormatter formats messages for Telegram with HTML
type TelegramFormatter struct {
	ChatID string
}

func (f *TelegramFormatter) Format(status analyzer.Status, message, sessionID string, statusInfo config.StatusInfo) (interface{}, error) {
	// HTML formatting for Telegram
	emoji := getEmojiForStatus(status)
	text := fmt.Sprintf("<b>%s %s</b>\n\n%s\n\n<i>Session: %s</i>",
		emoji, statusInfo.Title, message, sessionID)

	return map[string]interface{}{
		"chat_id":    f.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}, nil
}

// getColorForStatus returns color hex code for status (Slack)
func getColorForStatus(status analyzer.Status) string {
	switch status {
	case analyzer.StatusTaskComplete:
		return "#28a745" // Green
	case analyzer.StatusReviewComplete:
		return "#17a2b8" // Teal
	case analyzer.StatusQuestion:
		return "#ffc107" // Yellow/Orange
	case analyzer.StatusPlanReady:
		return "#007bff" // Blue
	default:
		return "#6c757d" // Gray
	}
}

// getDiscordColorInt returns Discord color integer for status
func getDiscordColorInt(status analyzer.Status) int {
	switch status {
	case analyzer.StatusTaskComplete:
		return 0x28a745 // Green
	case analyzer.StatusReviewComplete:
		return 0x17a2b8 // Teal
	case analyzer.StatusQuestion:
		return 0xffc107 // Yellow
	case analyzer.StatusPlanReady:
		return 0x007bff // Blue
	default:
		return 0x6c757d // Gray
	}
}

// getEmojiForStatus returns emoji for status (Telegram)
func getEmojiForStatus(status analyzer.Status) string {
	switch status {
	case analyzer.StatusTaskComplete:
		return "✅"
	case analyzer.StatusReviewComplete:
		return "🔍"
	case analyzer.StatusQuestion:
		return "❓"
	case analyzer.StatusPlanReady:
		return "📋"
	default:
		return "ℹ️"
	}
}
