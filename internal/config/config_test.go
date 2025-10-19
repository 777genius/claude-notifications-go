package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.Notifications.Desktop.Enabled)
	assert.True(t, cfg.Notifications.Desktop.Sound)
	assert.False(t, cfg.Notifications.Webhook.Enabled)
	assert.Equal(t, 7, cfg.Notifications.SuppressQuestionAfterTaskCompleteSeconds)

	// Check statuses
	assert.Contains(t, cfg.Statuses, "task_complete")
	assert.Contains(t, cfg.Statuses, "question")
	assert.Contains(t, cfg.Statuses, "plan_ready")
}

func TestLoadConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"notifications": {
			"desktop": {
				"enabled": false,
				"sound": false,
				"appIcon": ""
			},
			"webhook": {
				"enabled": true,
				"preset": "slack",
				"url": "https://hooks.slack.com/test",
				"format": "json"
			},
			"suppressQuestionAfterTaskCompleteSeconds": 10
		},
		"statuses": {
			"task_complete": {
				"title": "Done",
				"sound": "",
				"keywords": ["done"]
			}
		}
	}`

	err := os.WriteFile(configPath, []byte(configJSON), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.False(t, cfg.Notifications.Desktop.Enabled)
	assert.True(t, cfg.Notifications.Webhook.Enabled)
	assert.Equal(t, "slack", cfg.Notifications.Webhook.Preset)
	assert.Equal(t, 10, cfg.Notifications.SuppressQuestionAfterTaskCompleteSeconds)
}

func TestLoadConfigNotExists(t *testing.T) {
	// Load non-existent config should return defaults
	cfg, err := Load("/nonexistent/config.json")
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.True(t, cfg.Notifications.Desktop.Enabled)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid webhook preset",
			cfg: &Config{
				Notifications: NotificationsConfig{
					Webhook: WebhookConfig{
						Enabled: true,
						Preset:  "invalid",
						URL:     "https://example.com",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "webhook enabled but no URL",
			cfg: &Config{
				Notifications: NotificationsConfig{
					Webhook: WebhookConfig{
						Enabled: true,
						Preset:  "slack",
						URL:     "",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "telegram without chat_id",
			cfg: &Config{
				Notifications: NotificationsConfig{
					Webhook: WebhookConfig{
						Enabled: true,
						Preset:  "telegram",
						URL:     "https://api.telegram.org",
						ChatID:  "",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetStatusInfo(t *testing.T) {
	cfg := DefaultConfig()

	info, exists := cfg.GetStatusInfo("task_complete")
	assert.True(t, exists)
	assert.Contains(t, info.Title, "Task Completed")

	_, exists = cfg.GetStatusInfo("nonexistent")
	assert.False(t, exists)
}

func TestIsNotificationEnabled(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.IsDesktopEnabled())
	assert.False(t, cfg.IsWebhookEnabled())
	assert.True(t, cfg.IsAnyNotificationEnabled())

	// Disable all
	cfg.Notifications.Desktop.Enabled = false
	assert.False(t, cfg.IsAnyNotificationEnabled())
}

func TestDefaultConfigPathsNoMixedSeparators(t *testing.T) {
	cfg := DefaultConfig()

	// Check AppIcon path doesn't contain forward slashes on any platform
	// (should use OS-specific separators via filepath.Join)
	appIcon := cfg.Notifications.Desktop.AppIcon
	assert.NotContains(t, appIcon, "/claude_icon.png", "AppIcon should use filepath.Join, not string concatenation")

	// Check all sound paths don't contain forward slashes
	for status, info := range cfg.Statuses {
		assert.NotContains(t, info.Sound, "/sounds/", "Sound path for %s should use filepath.Join, not string concatenation", status)
	}

	// Verify paths are valid (contain expected filename)
	assert.Contains(t, appIcon, "claude_icon.png")
	assert.Contains(t, cfg.Statuses["task_complete"].Sound, "task-complete.mp3")
	assert.Contains(t, cfg.Statuses["question"].Sound, "question.mp3")
}
