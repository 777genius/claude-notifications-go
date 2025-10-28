package state

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === Load/Save/Delete Tests ===

func TestManager_LoadNonExistent(t *testing.T) {
	mgr := NewManager()

	state, err := mgr.Load("non-existent-session")
	require.NoError(t, err)
	assert.Nil(t, state, "should return nil for non-existent state")
}

func TestManager_SaveAndLoad(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-session-save-load"

	// Clean up after test
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create and save state
	state := &SessionState{
		SessionID:           sessionID,
		LastInteractiveTool: "ExitPlanMode",
		LastTimestamp:       platform.CurrentTimestamp(),
		CWD:                 "/test/dir",
	}

	err := mgr.Save(state)
	require.NoError(t, err)

	// Load state
	loaded, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify fields
	assert.Equal(t, sessionID, loaded.SessionID)
	assert.Equal(t, "ExitPlanMode", loaded.LastInteractiveTool)
	assert.Equal(t, state.LastTimestamp, loaded.LastTimestamp)
	assert.Equal(t, "/test/dir", loaded.CWD)
}

func TestManager_Delete(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-session-delete"

	// Save state
	state := &SessionState{SessionID: sessionID}
	err := mgr.Save(state)
	require.NoError(t, err)

	// Verify it exists
	loaded, err := mgr.Load(sessionID)
	require.NoError(t, err)
	assert.NotNil(t, loaded)

	// Delete
	err = mgr.Delete(sessionID)
	require.NoError(t, err)

	// Verify it's gone
	loaded, err = mgr.Load(sessionID)
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestManager_DeleteNonExistent(t *testing.T) {
	mgr := NewManager()

	// Should not error when deleting non-existent state
	err := mgr.Delete("non-existent")
	assert.NoError(t, err)
}

// === UpdateInteractiveTool Tests ===

func TestManager_UpdateInteractiveTool_NewState(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-interactive-new"
	defer func() { _ = mgr.Delete(sessionID) }()

	err := mgr.UpdateInteractiveTool(sessionID, "ExitPlanMode", "/test/dir")
	require.NoError(t, err)

	// Verify state was created
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, sessionID, state.SessionID)
	assert.Equal(t, "ExitPlanMode", state.LastInteractiveTool)
	assert.Equal(t, "/test/dir", state.CWD)
	assert.Greater(t, state.LastTimestamp, int64(0))
}

func TestManager_UpdateInteractiveTool_ExistingState(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-interactive-existing"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create initial state
	initial := &SessionState{
		SessionID:            sessionID,
		LastTaskCompleteTime: 12345,
	}
	err := mgr.Save(initial)
	require.NoError(t, err)

	// Update with new tool
	err = mgr.UpdateInteractiveTool(sessionID, "AskUserQuestion", "/new/dir")
	require.NoError(t, err)

	// Verify state was updated
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, "AskUserQuestion", state.LastInteractiveTool)
	assert.Equal(t, "/new/dir", state.CWD)
	assert.Greater(t, state.LastTimestamp, int64(0))
	// Existing fields should be preserved
	assert.Equal(t, int64(12345), state.LastTaskCompleteTime)
}

// === UpdateTaskComplete Tests ===

func TestManager_UpdateTaskComplete_NewState(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-task-new"
	defer func() { _ = mgr.Delete(sessionID) }()

	err := mgr.UpdateTaskComplete(sessionID)
	require.NoError(t, err)

	// Verify state was created
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, sessionID, state.SessionID)
	assert.Greater(t, state.LastTaskCompleteTime, int64(0))
}

func TestManager_UpdateTaskComplete_ExistingState(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-task-existing"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create initial state
	initial := &SessionState{
		SessionID:           sessionID,
		LastInteractiveTool: "ExitPlanMode",
	}
	err := mgr.Save(initial)
	require.NoError(t, err)

	// Update task complete
	err = mgr.UpdateTaskComplete(sessionID)
	require.NoError(t, err)

	// Verify state was updated
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Greater(t, state.LastTaskCompleteTime, int64(0))
	// Existing fields should be preserved
	assert.Equal(t, "ExitPlanMode", state.LastInteractiveTool)
}

// === UpdateLastNotification Tests ===

func TestManager_UpdateLastNotification_NewState(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-notif-new"
	defer func() { _ = mgr.Delete(sessionID) }()

	err := mgr.UpdateLastNotification(sessionID, analyzer.StatusPlanReady)
	require.NoError(t, err)

	// Verify state was created
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, sessionID, state.SessionID)
	assert.Greater(t, state.LastNotificationTime, int64(0))
	assert.Equal(t, string(analyzer.StatusPlanReady), state.LastNotificationStatus)
}

func TestManager_UpdateLastNotification_ExistingState(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-notif-existing"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create initial state
	initial := &SessionState{
		SessionID:           sessionID,
		LastInteractiveTool: "ExitPlanMode",
	}
	err := mgr.Save(initial)
	require.NoError(t, err)

	// Update last notification
	err = mgr.UpdateLastNotification(sessionID, analyzer.StatusTaskComplete)
	require.NoError(t, err)

	// Verify state was updated
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Greater(t, state.LastNotificationTime, int64(0))
	assert.Equal(t, string(analyzer.StatusTaskComplete), state.LastNotificationStatus)
	// Existing fields should be preserved
	assert.Equal(t, "ExitPlanMode", state.LastInteractiveTool)
}

// === ShouldSuppressQuestion Tests ===

func TestManager_ShouldSuppressQuestion_NoState(t *testing.T) {
	mgr := NewManager()

	suppress, err := mgr.ShouldSuppressQuestion("non-existent", 5)
	require.NoError(t, err)
	assert.False(t, suppress, "should not suppress when no state exists")
}

func TestManager_ShouldSuppressQuestion_NoTaskCompleteTime(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-no-time"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create state without LastTaskCompleteTime
	state := &SessionState{SessionID: sessionID}
	err := mgr.Save(state)
	require.NoError(t, err)

	suppress, err := mgr.ShouldSuppressQuestion(sessionID, 5)
	require.NoError(t, err)
	assert.False(t, suppress, "should not suppress when no task complete time")
}

func TestManager_ShouldSuppressQuestion_WithinCooldown(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-within"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create state with recent task complete
	state := &SessionState{
		SessionID:            sessionID,
		LastTaskCompleteTime: platform.CurrentTimestamp(),
	}
	err := mgr.Save(state)
	require.NoError(t, err)

	// Check immediately - should suppress
	suppress, err := mgr.ShouldSuppressQuestion(sessionID, 5)
	require.NoError(t, err)
	assert.True(t, suppress, "should suppress within cooldown window")
}

func TestManager_ShouldSuppressQuestion_OutsideCooldown(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-outside"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create state with old task complete (6 seconds ago)
	state := &SessionState{
		SessionID:            sessionID,
		LastTaskCompleteTime: platform.CurrentTimestamp() - 6,
	}
	err := mgr.Save(state)
	require.NoError(t, err)

	// Check with 5s cooldown - should not suppress
	suppress, err := mgr.ShouldSuppressQuestion(sessionID, 5)
	require.NoError(t, err)
	assert.False(t, suppress, "should not suppress outside cooldown window")
}

func TestManager_ShouldSuppressQuestion_ZeroCooldown(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-zero"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Create state
	state := &SessionState{
		SessionID:            sessionID,
		LastTaskCompleteTime: platform.CurrentTimestamp(),
	}
	err := mgr.Save(state)
	require.NoError(t, err)

	// Zero cooldown should never suppress
	suppress, err := mgr.ShouldSuppressQuestion(sessionID, 0)
	require.NoError(t, err)
	assert.False(t, suppress, "zero cooldown should never suppress")
}

func TestManager_ShouldSuppressQuestion_NegativeCooldown(t *testing.T) {
	mgr := NewManager()

	suppress, err := mgr.ShouldSuppressQuestion("any-session", -5)
	require.NoError(t, err)
	assert.False(t, suppress, "negative cooldown should never suppress")
}

// === ShouldSuppressQuestionAfterAnyNotification Tests ===

func TestManager_ShouldSuppressAfterAny_NoState(t *testing.T) {
	mgr := NewManager()

	suppress, err := mgr.ShouldSuppressQuestionAfterAnyNotification("non-existent", 5)
	require.NoError(t, err)
	assert.False(t, suppress)
}

func TestManager_ShouldSuppressAfterAny_NoNotificationTime(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-any-no-time"
	defer func() { _ = mgr.Delete(sessionID) }()

	state := &SessionState{SessionID: sessionID}
	err := mgr.Save(state)
	require.NoError(t, err)

	suppress, err := mgr.ShouldSuppressQuestionAfterAnyNotification(sessionID, 5)
	require.NoError(t, err)
	assert.False(t, suppress)
}

func TestManager_ShouldSuppressAfterAny_WithinCooldown(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-any-within"
	defer func() { _ = mgr.Delete(sessionID) }()

	state := &SessionState{
		SessionID:            sessionID,
		LastNotificationTime: platform.CurrentTimestamp(),
	}
	err := mgr.Save(state)
	require.NoError(t, err)

	suppress, err := mgr.ShouldSuppressQuestionAfterAnyNotification(sessionID, 5)
	require.NoError(t, err)
	assert.True(t, suppress)
}

func TestManager_ShouldSuppressAfterAny_OutsideCooldown(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-suppress-any-outside"
	defer func() { _ = mgr.Delete(sessionID) }()

	state := &SessionState{
		SessionID:            sessionID,
		LastNotificationTime: platform.CurrentTimestamp() - 6,
	}
	err := mgr.Save(state)
	require.NoError(t, err)

	suppress, err := mgr.ShouldSuppressQuestionAfterAnyNotification(sessionID, 5)
	require.NoError(t, err)
	assert.False(t, suppress)
}

// === UpdateState Tests ===

func TestManager_UpdateState_TaskComplete(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-update-task"
	defer func() { _ = mgr.Delete(sessionID) }()

	err := mgr.UpdateState(sessionID, analyzer.StatusTaskComplete, "", "")
	require.NoError(t, err)

	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	assert.Greater(t, state.LastTaskCompleteTime, int64(0))
}

func TestManager_UpdateState_PlanReady(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-update-plan"
	defer func() { _ = mgr.Delete(sessionID) }()

	err := mgr.UpdateState(sessionID, analyzer.StatusPlanReady, "ExitPlanMode", "/test")
	require.NoError(t, err)

	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ExitPlanMode", state.LastInteractiveTool)
	assert.Equal(t, "/test", state.CWD)
}

func TestManager_UpdateState_Question(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-update-question"
	defer func() { _ = mgr.Delete(sessionID) }()

	err := mgr.UpdateState(sessionID, analyzer.StatusQuestion, "AskUserQuestion", "/test")
	require.NoError(t, err)

	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	assert.Equal(t, "AskUserQuestion", state.LastInteractiveTool)
}

func TestManager_UpdateState_UnknownStatus(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-update-unknown"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Unknown status should not create state
	err := mgr.UpdateState(sessionID, analyzer.StatusUnknown, "SomeTool", "/test")
	require.NoError(t, err)

	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestManager_UpdateState_QuestionWithoutTool(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-update-question-no-tool"
	defer func() { _ = mgr.Delete(sessionID) }()

	// Question without tool name should not update
	err := mgr.UpdateState(sessionID, analyzer.StatusQuestion, "", "/test")
	require.NoError(t, err)

	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	assert.Nil(t, state)
}

// === Cleanup Tests ===

func TestManager_Cleanup_OldFiles(t *testing.T) {
	mgr := NewManager()

	// Create two state files
	session1 := "test-cleanup-1"
	session2 := "test-cleanup-2"

	state1 := &SessionState{SessionID: session1}
	state2 := &SessionState{SessionID: session2}

	err := mgr.Save(state1)
	require.NoError(t, err)
	err = mgr.Save(state2)
	require.NoError(t, err)

	// Make session1 old by modifying its mtime
	path1 := mgr.getStatePath(session1)
	oldTime := time.Now().Add(-120 * time.Second)
	err = os.Chtimes(path1, oldTime, oldTime)
	require.NoError(t, err)

	// Clean up files older than 60 seconds
	err = mgr.Cleanup(60)
	require.NoError(t, err)

	// session1 should be deleted, session2 should remain
	state, _ := mgr.Load(session1)
	assert.Nil(t, state, "old state should be deleted")

	state, err = mgr.Load(session2)
	require.NoError(t, err)
	assert.NotNil(t, state, "recent state should remain")

	// Cleanup
	_ = mgr.Delete(session2)
}

func TestManager_Cleanup_EmptyDirectory(t *testing.T) {
	mgr := NewManager()

	// Should not error on empty directory
	err := mgr.Cleanup(60)
	assert.NoError(t, err)
}

// === Integration Tests ===

func TestManager_FullWorkflow(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-workflow"
	defer func() { _ = mgr.Delete(sessionID) }()

	// 1. Update interactive tool (plan ready)
	err := mgr.UpdateInteractiveTool(sessionID, "ExitPlanMode", "/project")
	require.NoError(t, err)

	// 2. Update notification
	err = mgr.UpdateLastNotification(sessionID, analyzer.StatusPlanReady)
	require.NoError(t, err)

	// 3. Question should be suppressed within cooldown
	suppress, err := mgr.ShouldSuppressQuestionAfterAnyNotification(sessionID, 60)
	require.NoError(t, err)
	assert.True(t, suppress, "question should be suppressed after plan notification")

	// 4. Update task complete
	err = mgr.UpdateTaskComplete(sessionID)
	require.NoError(t, err)

	// 5. Update last notification
	err = mgr.UpdateLastNotification(sessionID, analyzer.StatusTaskComplete)
	require.NoError(t, err)

	// 6. Verify state contains all expected fields
	state, err := mgr.Load(sessionID)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, sessionID, state.SessionID)
	assert.Equal(t, "ExitPlanMode", state.LastInteractiveTool)
	assert.Equal(t, "/project", state.CWD)
	assert.Greater(t, state.LastTimestamp, int64(0))
	assert.Greater(t, state.LastTaskCompleteTime, int64(0))
	assert.Greater(t, state.LastNotificationTime, int64(0))
	assert.Equal(t, string(analyzer.StatusTaskComplete), state.LastNotificationStatus)
}

func TestManager_StateFilePath(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-abc-123"

	path := mgr.getStatePath(sessionID)

	// Should contain session ID in filename
	assert.Contains(t, path, "claude-session-state-test-abc-123.json")

	// Should be an absolute path
	assert.True(t, filepath.IsAbs(path), "path should be absolute")

	// Should have correct filename format
	expectedFilename := "claude-session-state-test-abc-123.json"
	assert.Equal(t, expectedFilename, filepath.Base(path))
}

func TestLoad_InvalidJSON(t *testing.T) {
	mgr := NewManager()
	sessionID := "test-invalid-json"

	// Create a file with invalid JSON
	path := mgr.getStatePath(sessionID)
	err := os.WriteFile(path, []byte("{invalid json}"), 0644)
	require.NoError(t, err)
	defer os.Remove(path)

	// Load should return error for invalid JSON
	state, err := mgr.Load(sessionID)
	assert.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestDelete_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: Unix-style permissions not supported")
	}

	// Create a custom temp directory that we can control permissions on
	testTempDir := filepath.Join(t.TempDir(), "states")
	err := os.MkdirAll(testTempDir, 0755)
	require.NoError(t, err)

	// Create manager with custom temp dir
	mgr := &Manager{tempDir: testTempDir}
	sessionID := "test-delete-protected"

	// Create a state file
	state := &SessionState{SessionID: sessionID}
	err = mgr.Save(state)
	require.NoError(t, err)

	// Make the directory read-only to prevent deletion
	err = os.Chmod(testTempDir, 0555) // Read + execute only
	require.NoError(t, err)

	// Delete should fail due to permissions
	err = mgr.Delete(sessionID)
	assert.Error(t, err, "Delete should fail on permission denied")

	// Restore permissions for cleanup
	_ = os.Chmod(testTempDir, 0755)
}
