package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/belief/claude-notifications/internal/platform"
)

// Config represents the plugin configuration
type Config struct {
	Notifications NotificationsConfig   `json:"notifications"`
	Statuses      map[string]StatusInfo `json:"statuses"`
}

// NotificationsConfig represents notification settings
type NotificationsConfig struct {
	Desktop                                DesktopConfig  `json:"desktop"`
	Webhook                                WebhookConfig  `json:"webhook"`
	SuppressQuestionAfterTaskCompleteSeconds int            `json:"suppressQuestionAfterTaskCompleteSeconds"`
}

// DesktopConfig represents desktop notification settings
type DesktopConfig struct {
	Enabled bool   `json:"enabled"`
	Sound   bool   `json:"sound"`
	AppIcon string `json:"appIcon"`
}

// WebhookConfig represents webhook settings
type WebhookConfig struct {
	Enabled bool              `json:"enabled"`
	Preset  string            `json:"preset"`
	URL     string            `json:"url"`
	ChatID  string            `json:"chat_id"`
	Format  string            `json:"format"`
	Headers map[string]string `json:"headers"`
}

// StatusInfo represents configuration for a specific status
type StatusInfo struct {
	Title string `json:"title"`
	Sound string `json:"sound"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	// Get plugin root from environment, fallback to current directory
	pluginRoot := platform.ExpandEnv("${CLAUDE_PLUGIN_ROOT}")
	if pluginRoot == "" || pluginRoot == "${CLAUDE_PLUGIN_ROOT}" {
		pluginRoot = "."
	}

	return &Config{
		Notifications: NotificationsConfig{
			Desktop: DesktopConfig{
				Enabled: true,
				Sound:   true,
				AppIcon: filepath.Join(pluginRoot, "claude_icon.png"),
			},
			Webhook: WebhookConfig{
				Enabled: false,
				Preset:  "custom",
				URL:     "",
				ChatID:  "",
				Format:  "json",
				Headers: make(map[string]string),
			},
			SuppressQuestionAfterTaskCompleteSeconds: 7,
		},
		Statuses: map[string]StatusInfo{
			"task_complete": {
				Title: "✅ Task Completed",
				Sound: filepath.Join(pluginRoot, "sounds", "task-complete.mp3"),
			},
			"review_complete": {
				Title: "🔍 Review Completed",
				Sound: filepath.Join(pluginRoot, "sounds", "review-complete.mp3"),
			},
			"question": {
				Title: "❓ Claude Has Questions",
				Sound: filepath.Join(pluginRoot, "sounds", "question.mp3"),
			},
			"plan_ready": {
				Title: "📋 Plan Ready for Review",
				Sound: filepath.Join(pluginRoot, "sounds", "plan-ready.mp3"),
			},
		},
	}
}

// Load loads configuration from a file
// If the file doesn't exist, returns default config
func Load(path string) (*Config, error) {
	// If path doesn't exist, use default config
	if !platform.FileExists(path) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand environment variables in paths
	config.Notifications.Desktop.AppIcon = platform.ExpandEnv(config.Notifications.Desktop.AppIcon)
	config.Notifications.Webhook.URL = platform.ExpandEnv(config.Notifications.Webhook.URL)

	// Expand environment variables in sound paths
	for status, info := range config.Statuses {
		info.Sound = platform.ExpandEnv(info.Sound)
		config.Statuses[status] = info
	}

	// Apply defaults for missing fields
	config.ApplyDefaults()

	return config, nil
}

// LoadFromPluginRoot loads configuration from plugin root directory
func LoadFromPluginRoot(pluginRoot string) (*Config, error) {
	configPath := filepath.Join(pluginRoot, "config", "config.json")
	return Load(configPath)
}

// ApplyDefaults fills in missing fields with default values
func (c *Config) ApplyDefaults() {
	// Desktop defaults
	if c.Notifications.Desktop.AppIcon == "" {
		// Keep empty if not set
	}

	// Webhook defaults
	if c.Notifications.Webhook.Preset == "" {
		c.Notifications.Webhook.Preset = "custom"
	}
	if c.Notifications.Webhook.Format == "" {
		c.Notifications.Webhook.Format = "json"
	}
	if c.Notifications.Webhook.Headers == nil {
		c.Notifications.Webhook.Headers = make(map[string]string)
	}

	// Cooldown default
	if c.Notifications.SuppressQuestionAfterTaskCompleteSeconds == 0 {
		c.Notifications.SuppressQuestionAfterTaskCompleteSeconds = 7
	}

	// Status defaults
	defaults := DefaultConfig()
	if c.Statuses == nil {
		c.Statuses = defaults.Statuses
	} else {
		// Fill in missing statuses
		for key, val := range defaults.Statuses {
			if _, exists := c.Statuses[key]; !exists {
				c.Statuses[key] = val
			}
		}
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate webhook preset
	validPresets := map[string]bool{
		"slack":    true,
		"discord":  true,
		"telegram": true,
		"custom":   true,
	}
	if !validPresets[c.Notifications.Webhook.Preset] {
		return fmt.Errorf("invalid webhook preset: %s (must be one of: slack, discord, telegram, custom)", c.Notifications.Webhook.Preset)
	}

	// Validate webhook format
	validFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validFormats[c.Notifications.Webhook.Format] {
		return fmt.Errorf("invalid webhook format: %s (must be one of: json, text)", c.Notifications.Webhook.Format)
	}

	// Validate webhook URL if enabled
	if c.Notifications.Webhook.Enabled && c.Notifications.Webhook.URL == "" {
		return fmt.Errorf("webhook URL is required when webhooks are enabled")
	}

	// Validate Telegram chat_id if Telegram preset is used
	if c.Notifications.Webhook.Enabled && c.Notifications.Webhook.Preset == "telegram" && c.Notifications.Webhook.ChatID == "" {
		return fmt.Errorf("chat_id is required for Telegram webhook")
	}

	// Validate cooldown
	if c.Notifications.SuppressQuestionAfterTaskCompleteSeconds < 0 {
		return fmt.Errorf("suppressQuestionAfterTaskCompleteSeconds must be >= 0")
	}

	return nil
}

// GetStatusInfo returns status information for a given status
func (c *Config) GetStatusInfo(status string) (StatusInfo, bool) {
	info, exists := c.Statuses[status]
	return info, exists
}

// IsDesktopEnabled returns true if desktop notifications are enabled
func (c *Config) IsDesktopEnabled() bool {
	return c.Notifications.Desktop.Enabled
}

// IsWebhookEnabled returns true if webhook notifications are enabled
func (c *Config) IsWebhookEnabled() bool {
	return c.Notifications.Webhook.Enabled
}

// IsAnyNotificationEnabled returns true if at least one notification method is enabled
func (c *Config) IsAnyNotificationEnabled() bool {
	return c.IsDesktopEnabled() || c.IsWebhookEnabled()
}
