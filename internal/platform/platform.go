package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// OS returns the current operating system
func OS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	case "linux":
		return "linux"
	default:
		return "unknown"
	}
}

// TempDir returns the platform-specific temporary directory (without trailing slash)
func TempDir() string {
	tempDir := os.TempDir()
	// Remove trailing slash if present (macOS $TMPDIR ends with /)
	return strings.TrimSuffix(tempDir, string(os.PathSeparator))
}

// FileMTime returns the modification time of a file as Unix timestamp
// Returns 0 if the file doesn't exist or on error
func FileMTime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().Unix()
}

// CurrentTimestamp returns the current Unix timestamp
func CurrentTimestamp() int64 {
	return time.Now().Unix()
}

// FileAge returns the age of a file in seconds
// Returns -1 if the file doesn't exist
func FileAge(path string) int64 {
	mtime := FileMTime(path)
	if mtime == 0 {
		return -1
	}
	return CurrentTimestamp() - mtime
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CleanupOldFiles removes files older than maxAge seconds matching a pattern
func CleanupOldFiles(dir, pattern string, maxAge int64) error {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return err
	}

	for _, path := range matches {
		age := FileAge(path)
		if age >= 0 && age > maxAge {
			_ = os.Remove(path) // Ignore errors
		}
	}
	return nil
}

// AtomicCreateFile creates a file atomically using O_EXCL flag
// Returns true if file was created, false if it already exists
func AtomicCreateFile(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	f.Close()
	return true, nil
}

// NormalizePath normalizes a file path (removes double slashes, etc.)
func NormalizePath(path string) string {
	return filepath.Clean(path)
}

// ExpandEnv expands environment variables in a string (like ${VAR} or $VAR)
func ExpandEnv(s string) string {
	return os.ExpandEnv(s)
}

// IsWindows returns true if running on Windows
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsMacOS returns true if running on macOS
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true if running on Linux
func IsLinux() bool {
	return runtime.GOOS == "linux"
}
