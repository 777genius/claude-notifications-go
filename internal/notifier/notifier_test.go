package notifier

import (
	"testing"

	"github.com/gen2brain/beeep"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
)

func TestExtractSessionName(t *testing.T) {
	tests := []struct {
		name             string
		message          string
		expectedSession  string
		expectedCleanMsg string
	}{
		{
			name:             "Valid session name with message",
			message:          "[bold-cat] Created 3 files. Edited 2 files. Took 2m 15s",
			expectedSession:  "bold-cat",
			expectedCleanMsg: "Created 3 files. Edited 2 files. Took 2m 15s",
		},
		{
			name:             "Valid session name with short message",
			message:          "[swift-eagle] Task complete",
			expectedSession:  "swift-eagle",
			expectedCleanMsg: "Task complete",
		},
		{
			name:             "Message without session name",
			message:          "Task completed successfully",
			expectedSession:  "",
			expectedCleanMsg: "Task completed successfully",
		},
		{
			name:             "Message with only opening bracket",
			message:          "[no-closing-bracket Task complete",
			expectedSession:  "",
			expectedCleanMsg: "[no-closing-bracket Task complete",
		},
		{
			name:             "Empty message",
			message:          "",
			expectedSession:  "",
			expectedCleanMsg: "",
		},
		{
			name:             "Session name with extra spaces",
			message:          "[cool-fox]   Multiple   spaces   message",
			expectedSession:  "cool-fox",
			expectedCleanMsg: "Multiple   spaces   message",
		},
		{
			name:             "Session name only (no message)",
			message:          "[lonely-wolf]",
			expectedSession:  "lonely-wolf",
			expectedCleanMsg: "",
		},
		{
			name:             "Leading/trailing spaces",
			message:          "  [trim-test] Message with spaces  ",
			expectedSession:  "trim-test",
			expectedCleanMsg: "Message with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, cleanMsg := extractSessionName(tt.message)
			if session != tt.expectedSession {
				t.Errorf("extractSessionName(%q) session = %q, want %q", tt.message, session, tt.expectedSession)
			}
			if cleanMsg != tt.expectedCleanMsg {
				t.Errorf("extractSessionName(%q) cleanMsg = %q, want %q", tt.message, cleanMsg, tt.expectedCleanMsg)
			}
		})
	}
}

func TestSendDesktopRestoresAppName(t *testing.T) {
	// This test verifies that SendDesktop properly restores beeep.AppName
	// after sending a notification, even if the notification fails.

	// Save original AppName
	originalAppName := beeep.AppName
	defer func() {
		beeep.AppName = originalAppName
	}()

	// Set a test value
	testAppName := "test-app-name"
	beeep.AppName = testAppName

	// Create notifier with desktop notifications disabled to skip actual notification
	cfg := config.DefaultConfig()
	cfg.Notifications.Desktop.Enabled = false
	n := New(cfg)

	// Call SendDesktop - should not change AppName since notifications are disabled
	_ = n.SendDesktop(analyzer.StatusTaskComplete, "test message")

	// Verify AppName is unchanged (because we skipped notification)
	if beeep.AppName != testAppName {
		t.Errorf("AppName changed unexpectedly: got %q, want %q", beeep.AppName, testAppName)
	}

	// Now test with enabled notifications (will attempt real notification)
	cfg.Notifications.Desktop.Enabled = true
	beeep.AppName = testAppName

	// This will attempt to send a real notification and may fail in CI,
	// but the important thing is that AppName is restored afterward
	_ = n.SendDesktop(analyzer.StatusTaskComplete, "test message")

	// Verify AppName is restored to testAppName after the defer runs
	if beeep.AppName != testAppName {
		t.Errorf("AppName not restored after SendDesktop: got %q, want %q", beeep.AppName, testAppName)
	}
}

func TestVolumeToGain(t *testing.T) {
	tests := []struct {
		name     string
		volume   float64
		expected float64
	}{
		{"0% volume", 0.0, -1.0},
		{"30% volume", 0.3, -0.7},
		{"50% volume", 0.5, -0.5},
		{"100% volume", 1.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := volumeToGain(tt.volume)
			if result != tt.expected {
				t.Errorf("volumeToGain(%.1f) = %.1f, want %.1f", tt.volume, result, tt.expected)
			}
		})
	}
}
