package hooks

import (
	"testing"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/stretchr/testify/assert"
)

// focusNotifyConfig returns a minimal config with desktop notifications enabled
// and the focus/delay options set as requested.
func focusNotifyConfig(onlyWhenUnfocused *bool, delaySeconds *int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Notifications.Desktop.Enabled = true
	cfg.Notifications.NotifyOnlyWhenUnfocused = onlyWhenUnfocused
	cfg.Notifications.NotifyDelaySeconds = delaySeconds
	return cfg
}

func TestSendDesktopNotification_SuppressedWhenFocused(t *testing.T) {
	on := true
	handler, mockNotif, _ := newTestHandler(t, focusNotifyConfig(&on, nil))

	restore := isTerminalFocused
	isTerminalFocused = func() bool { return true }
	defer func() { isTerminalFocused = restore }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.False(t, mockNotif.wasCalled(), "notification should be suppressed when the terminal is focused")
}

func TestSendDesktopNotification_DeliveredWhenUnfocused(t *testing.T) {
	on := true
	handler, mockNotif, _ := newTestHandler(t, focusNotifyConfig(&on, nil))

	restore := isTerminalFocused
	isTerminalFocused = func() bool { return false }
	defer func() { isTerminalFocused = restore }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.True(t, mockNotif.wasCalled(), "notification should be delivered when the terminal is not focused")
}

func TestSendDesktopNotification_FocusIgnoredWhenOptionOff(t *testing.T) {
	// notifyOnlyWhenUnfocused unset (default) — focus state must not be consulted.
	handler, mockNotif, _ := newTestHandler(t, focusNotifyConfig(nil, nil))

	restore := isTerminalFocused
	called := false
	isTerminalFocused = func() bool { called = true; return true }
	defer func() { isTerminalFocused = restore }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.True(t, mockNotif.wasCalled(), "notification should always be delivered when the option is off")
	assert.False(t, called, "focus must not be checked when notifyOnlyWhenUnfocused is off")
}

func TestSendDesktopNotification_DelayUsesConfiguredSeconds(t *testing.T) {
	delay := 7
	handler, mockNotif, _ := newTestHandler(t, focusNotifyConfig(nil, &delay))

	restoreSleep := sleepFunc
	var slept time.Duration
	sleepFunc = func(d time.Duration) { slept = d }
	defer func() { sleepFunc = restoreSleep }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.Equal(t, 7*time.Second, slept, "should wait for the configured delay")
	assert.True(t, mockNotif.wasCalled())
}

func TestSendDesktopNotification_DelayClampedToMax(t *testing.T) {
	delay := maxNotifyDelaySeconds + 100
	handler, _, _ := newTestHandler(t, focusNotifyConfig(nil, &delay))

	restoreSleep := sleepFunc
	var slept time.Duration
	sleepFunc = func(d time.Duration) { slept = d }
	defer func() { sleepFunc = restoreSleep }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.Equal(t, time.Duration(maxNotifyDelaySeconds)*time.Second, slept, "delay must be clamped to the hook-timeout budget")
}

func TestSendDesktopNotification_NoDelayWhenZero(t *testing.T) {
	handler, mockNotif, _ := newTestHandler(t, focusNotifyConfig(nil, nil))

	restoreSleep := sleepFunc
	sleepCalled := false
	sleepFunc = func(d time.Duration) { sleepCalled = true }
	defer func() { sleepFunc = restoreSleep }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.False(t, sleepCalled, "no delay should occur when notifyDelaySeconds is unset")
	assert.True(t, mockNotif.wasCalled())
}

func TestSendDesktopNotification_DelayThenSuppressedOnFocus(t *testing.T) {
	// Combined behavior: wait, then suppress because the terminal regained focus.
	on := true
	delay := 5
	handler, mockNotif, _ := newTestHandler(t, focusNotifyConfig(&on, &delay))

	restoreSleep := sleepFunc
	var slept time.Duration
	sleepFunc = func(d time.Duration) { slept = d }
	defer func() { sleepFunc = restoreSleep }()

	restoreFocus := isTerminalFocused
	isTerminalFocused = func() bool { return true }
	defer func() { isTerminalFocused = restoreFocus }()

	handler.sendDesktopNotification(analyzer.StatusTaskComplete, "[s folder] done", "sess", "/cwd")

	assert.Equal(t, 5*time.Second, slept, "delay still runs before the focus re-check")
	assert.False(t, mockNotif.wasCalled(), "notification suppressed after delay because terminal is focused")
}
