package dedup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/claude-notifications/internal/platform"
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

// getLockPath returns the path to the lock file for a session and hook event
// If hookEvent is empty, uses a global lock for the session (backward compatibility)
func (m *Manager) getLockPath(sessionID string, hookEvent ...string) string {
	if len(hookEvent) > 0 && hookEvent[0] != "" {
		return filepath.Join(m.tempDir, fmt.Sprintf("claude-notification-%s-%s.lock", sessionID, hookEvent[0]))
	}
	return filepath.Join(m.tempDir, fmt.Sprintf("claude-notification-%s.lock", sessionID))
}

// CheckEarlyDuplicate performs Phase 1 check for duplicates
// Returns true if this is a duplicate and should be skipped
// hookEvent parameter is optional - if provided, checks hook-specific lock file
func (m *Manager) CheckEarlyDuplicate(sessionID string, hookEvent ...string) bool {
	lockPath := m.getLockPath(sessionID, hookEvent...)

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
// hookEvent parameter is optional - if provided, uses hook-specific lock file
func (m *Manager) AcquireLock(sessionID string, hookEvent ...string) (bool, error) {
	lockPath := m.getLockPath(sessionID, hookEvent...)

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
	_ = os.Remove(lockPath) // Ignore error - someone else might have deleted it

	// Try again
	created, err = platform.AtomicCreateFile(lockPath)
	if err != nil {
		return false, fmt.Errorf("failed to create lock file after cleanup: %w", err)
	}

	return created, nil
}

// ReleaseLock releases a lock (optional, locks are cleaned up automatically)
// hookEvent parameter is optional - if provided, releases hook-specific lock file
func (m *Manager) ReleaseLock(sessionID string, hookEvent ...string) error {
	lockPath := m.getLockPath(sessionID, hookEvent...)
	if platform.FileExists(lockPath) {
		return os.Remove(lockPath)
	}
	return nil
}

// Cleanup cleans up old lock files (older than maxAge seconds)
func (m *Manager) Cleanup(maxAge int64) error {
	return platform.CleanupOldFiles(m.tempDir, "claude-notification-*.lock", maxAge)
}

// CleanupForSession cleans up lock file for a specific session
func (m *Manager) CleanupForSession(sessionID string) error {
	lockPath := m.getLockPath(sessionID)
	if platform.FileExists(lockPath) {
		return os.Remove(lockPath)
	}
	return nil
}
