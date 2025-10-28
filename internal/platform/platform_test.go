package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOS(t *testing.T) {
	osType := OS()
	assert.Contains(t, []string{"macos", "linux", "windows", "unknown"}, osType)
}

func TestTempDir(t *testing.T) {
	tempDir := TempDir()
	assert.NotEmpty(t, tempDir)
	// Should not end with slash
	assert.NotEqual(t, "/", tempDir[len(tempDir)-1:])
}

func TestFileExists(t *testing.T) {
	// Create temp file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tmpFile, []byte("test"), 0644)
	require.NoError(t, err)

	assert.True(t, FileExists(tmpFile))
	assert.False(t, FileExists("/nonexistent/file"))
}

func TestFileMTime(t *testing.T) {
	// Create temp file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tmpFile, []byte("test"), 0644)
	require.NoError(t, err)

	mtime := FileMTime(tmpFile)
	assert.Greater(t, mtime, int64(0))

	// Non-existent file should return 0
	mtime = FileMTime("/nonexistent/file")
	assert.Equal(t, int64(0), mtime)
}

func TestFileAge(t *testing.T) {
	// Create temp file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tmpFile, []byte("test"), 0644)
	require.NoError(t, err)

	age := FileAge(tmpFile)
	assert.GreaterOrEqual(t, age, int64(0))
	assert.Less(t, age, int64(5)) // Should be very recent

	// Non-existent file
	age = FileAge("/nonexistent/file")
	assert.Equal(t, int64(-1), age)
}

func TestCurrentTimestamp(t *testing.T) {
	ts := CurrentTimestamp()
	now := time.Now().Unix()
	assert.InDelta(t, now, ts, 1) // Within 1 second
}

func TestAtomicCreateFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.lock")

	// First creation should succeed
	created, err := AtomicCreateFile(filePath)
	require.NoError(t, err)
	assert.True(t, created)

	// Second creation should fail (file exists)
	created, err = AtomicCreateFile(filePath)
	require.NoError(t, err)
	assert.False(t, created)
}

func TestCleanupOldFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old file
	oldFile := filepath.Join(tmpDir, "old-file.txt")
	err := os.WriteFile(oldFile, []byte("old"), 0644)
	require.NoError(t, err)

	// Change mtime to be old (requires touch or similar)
	oldTime := time.Now().Add(-2 * time.Minute)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	require.NoError(t, err)

	// Create recent file
	newFile := filepath.Join(tmpDir, "new-file.txt")
	err = os.WriteFile(newFile, []byte("new"), 0644)
	require.NoError(t, err)

	// Cleanup files older than 60 seconds
	err = CleanupOldFiles(tmpDir, "*.txt", 60)
	require.NoError(t, err)

	// Old file should be deleted, new file should remain
	assert.False(t, FileExists(oldFile))
	assert.True(t, FileExists(newFile))
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/foo//bar", "/foo/bar"},
		{"/foo/./bar", "/foo/bar"},
		{"/foo/../bar", "/bar"},
		{".", "."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizePath(tt.input)
			// Convert to forward slashes for cross-platform comparison
			result = filepath.ToSlash(result)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	result := ExpandEnv("${TEST_VAR}/path")
	assert.Equal(t, "test_value/path", result)

	result = ExpandEnv("$TEST_VAR/path")
	assert.Equal(t, "test_value/path", result)
}

func TestPlatformChecks(t *testing.T) {
	// At least one should be true
	assert.True(t, IsMacOS() || IsLinux() || IsWindows())

	// Can't be multiple
	count := 0
	if IsMacOS() {
		count++
	}
	if IsLinux() {
		count++
	}
	if IsWindows() {
		count++
	}
	assert.LessOrEqual(t, count, 1)
}

func TestCleanupOldFiles_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// Invalid glob pattern with malformed bracket expression
	err := CleanupOldFiles(tmpDir, "[invalid", 0)
	assert.Error(t, err, "Invalid glob pattern should return error")
}

func TestAtomicCreateFile_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0444) // Read-only directory
	require.NoError(t, err)
	defer func() { _ = os.Chmod(readOnlyDir, 0755) }() // Restore permissions for cleanup

	filePath := filepath.Join(readOnlyDir, "test.lock")
	created, err := AtomicCreateFile(filePath)
	assert.False(t, created)
	assert.Error(t, err, "Creating file in read-only directory should fail")
}
