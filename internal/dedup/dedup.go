package dedup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/belief/claude-notifications/internal/platform"
)

// Manager handles deduplication using two-phase locking
type Manager struct {
	tempDir string
}

// NewManager creates a new deduplication manager
func NewManager() *Manager {
	return &Manager{
		tempDir: platform.TempDir(),
	}
}

// getLockPath returns the path to the lock file for a hook event and session
func (m *Manager) getLockPath(hookEvent, sessionID string) string {
	return filepath.Join(m.tempDir, fmt.Sprintf("claude-notification-%s-%s.lock", hookEvent, sessionID))
}

// CheckEarlyDuplicate performs Phase 1 check for duplicates
// Returns true if this is a duplicate and should be skipped
func (m *Manager) CheckEarlyDuplicate(hookEvent, sessionID string) bool {
	lockPath := m.getLockPath(hookEvent, sessionID)

	if !platform.FileExists(lockPath) {
		return false
	}

	// Check lock age
	age := platform.FileAge(lockPath)

	// If mtime is unavailable (Windows issue) or lock is fresh (<2s), treat as duplicate
	if age == -1 || (age >= 0 && age < 2) {
		return true
	}

	return false
}

// AcquireLock performs Phase 2 lock acquisition
// Returns true if lock was successfully acquired
func (m *Manager) AcquireLock(hookEvent, sessionID string) (bool, error) {
	lockPath := m.getLockPath(hookEvent, sessionID)

	// Try to create lock atomically
	created, err := platform.AtomicCreateFile(lockPath)
	if err != nil {
		return false, fmt.Errorf("failed to create lock file: %w", err)
	}

	if created {
		// Lock acquired successfully
		return true, nil
	}

	// Lock exists - check if it's stale
	age := platform.FileAge(lockPath)

	// If lock is fresh (<2s), we're a duplicate
	if age >= 0 && age < 2 {
		return false, nil
	}

	// Lock is stale - try to replace it
	if err := os.Remove(lockPath); err != nil {
		// Someone else might have deleted it, try to create anyway
	}

	// Try again
	created, err = platform.AtomicCreateFile(lockPath)
	if err != nil {
		return false, fmt.Errorf("failed to create lock file after cleanup: %w", err)
	}

	return created, nil
}

// ReleaseLock releases a lock (optional, locks are cleaned up automatically)
func (m *Manager) ReleaseLock(hookEvent, sessionID string) error {
	lockPath := m.getLockPath(hookEvent, sessionID)
	if platform.FileExists(lockPath) {
		return os.Remove(lockPath)
	}
	return nil
}

// Cleanup cleans up old lock files (older than maxAge seconds)
func (m *Manager) Cleanup(maxAge int64) error {
	return platform.CleanupOldFiles(m.tempDir, "claude-notification-*.lock", maxAge)
}

// CleanupForSession cleans up lock files for a specific session
func (m *Manager) CleanupForSession(sessionID string) error {
	pattern := fmt.Sprintf("claude-notification-*-%s.lock", sessionID)
	matches, err := filepath.Glob(filepath.Join(m.tempDir, pattern))
	if err != nil {
		return err
	}

	for _, path := range matches {
		_ = os.Remove(path) // Ignore errors
	}

	return nil
}
