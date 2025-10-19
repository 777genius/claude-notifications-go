package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/belief/claude-notifications/internal/analyzer"
	"github.com/belief/claude-notifications/internal/config"
	"github.com/belief/claude-notifications/internal/logging"
)

// Sender sends webhook notifications
type Sender struct {
	cfg    *config.Config
	client *http.Client
}

// New creates a new webhook sender
func New(cfg *config.Config) *Sender {
	return &Sender{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends a webhook notification
func (s *Sender) Send(status analyzer.Status, message, sessionID string) error {
	if !s.cfg.IsWebhookEnabled() {
		logging.Debug("Webhooks disabled, skipping")
		return nil
	}

	webhookCfg := s.cfg.Notifications.Webhook

	// Build payload based on preset
	var payload []byte
	var contentType string
	var err error

	switch webhookCfg.Preset {
	case "slack":
		payload, err = s.buildSlackPayload(status, message)
		contentType = "application/json"
	case "discord":
		payload, err = s.buildDiscordPayload(status, message)
		contentType = "application/json"
	case "telegram":
		payload, err = s.buildTelegramPayload(status, message, webhookCfg.ChatID)
		contentType = "application/json"
	case "custom":
		payload, contentType, err = s.buildCustomPayload(status, message, sessionID, webhookCfg.Format)
	default:
		return fmt.Errorf("unknown webhook preset: %s", webhookCfg.Preset)
	}

	if err != nil {
		return fmt.Errorf("failed to build webhook payload: %w", err)
	}

	// Send webhook
	return s.sendHTTP(webhookCfg.URL, payload, contentType, webhookCfg.Headers)
}

// buildSlackPayload builds a Slack webhook payload
func (s *Sender) buildSlackPayload(status analyzer.Status, message string) ([]byte, error) {
	statusInfo, _ := s.cfg.GetStatusInfo(string(status))
	text := fmt.Sprintf("%s: %s", statusInfo.Title, message)

	payload := map[string]interface{}{
		"text": text,
	}

	return json.Marshal(payload)
}

// buildDiscordPayload builds a Discord webhook payload
func (s *Sender) buildDiscordPayload(status analyzer.Status, message string) ([]byte, error) {
	statusInfo, _ := s.cfg.GetStatusInfo(string(status))
	content := fmt.Sprintf("%s: %s", statusInfo.Title, message)

	payload := map[string]interface{}{
		"content":  content,
		"username": "Claude Code",
	}

	return json.Marshal(payload)
}

// buildTelegramPayload builds a Telegram webhook payload
func (s *Sender) buildTelegramPayload(status analyzer.Status, message, chatID string) ([]byte, error) {
	statusInfo, _ := s.cfg.GetStatusInfo(string(status))
	text := fmt.Sprintf("%s: %s", statusInfo.Title, message)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	return json.Marshal(payload)
}

// buildCustomPayload builds a custom webhook payload
func (s *Sender) buildCustomPayload(status analyzer.Status, message, sessionID, format string) ([]byte, string, error) {
	if format == "text" {
		text := fmt.Sprintf("[%s] %s", status, message)
		return []byte(text), "text/plain", nil
	}

	// JSON format
	payload := map[string]interface{}{
		"status":     string(status),
		"message":    message,
		"timestamp":  time.Now().Format(time.RFC3339),
		"session_id": sessionID,
		"source":     "claude-notifications",
	}

	data, err := json.Marshal(payload)
	return data, "application/json", err
}

// sendHTTP sends an HTTP POST request with the payload
func (s *Sender) sendHTTP(url string, payload []byte, contentType string, headers map[string]string) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type
	req.Header.Set("Content-Type", contentType)

	// Set custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		logging.Error("Webhook request failed: %v", err)
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Error("Webhook failed: HTTP %d, Response: %s", resp.StatusCode, string(body))
		return fmt.Errorf("webhook failed: HTTP %d", resp.StatusCode)
	}

	logging.Info("Webhook sent successfully (HTTP %d)", resp.StatusCode)
	return nil
}

// SendAsync sends a webhook asynchronously (non-blocking)
func (s *Sender) SendAsync(status analyzer.Status, message, sessionID string) {
	go func() {
		if err := s.Send(status, message, sessionID); err != nil {
			logging.Error("Async webhook send failed: %v", err)
		}
	}()
}
