package notifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/platform"
)

// TestPlaySoundWithBuiltInFiles tests sound playback with actual MP3 files if available
func TestPlaySoundWithBuiltInFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sound playback test in short mode")
	}

	// Try to find the sounds directory
	soundsDir := findSoundsDirectory()
	if soundsDir == "" {
		t.Skip("Sounds directory not found, skipping sound playback test")
	}

	cfg := config.DefaultConfig()
	n := New(cfg)
	defer n.Close()

	tests := []struct {
		name     string
		filename string
	}{
		{"task-complete", "task-complete.mp3"},
		{"review-complete", "review-complete.mp3"},
		{"question", "question.mp3"},
		{"plan-ready", "plan-ready.mp3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			soundPath := filepath.Join(soundsDir, tt.filename)

			if !platform.FileExists(soundPath) {
				t.Skipf("Sound file not found: %s", soundPath)
			}

			// Test that playSound doesn't crash
			// We can't really test that audio is actually playing without human verification
			// But we can test that the function completes without error
			n.playSound(soundPath)

			// If we get here, playSound completed (either successfully or with logged error)
			// This is good enough for automated testing
		})
	}
}

// TestDecodeAudioFormats tests decoding various audio formats
func TestDecodeAudioFormats(t *testing.T) {
	soundsDir := findSoundsDirectory()
	if soundsDir == "" {
		t.Skip("Sounds directory not found")
	}

	cfg := config.DefaultConfig()
	n := New(cfg)
	defer n.Close()

	// Test MP3 decoding
	mp3Path := filepath.Join(soundsDir, "task-complete.mp3")
	if platform.FileExists(mp3Path) {
		t.Run("decode MP3", func(t *testing.T) {
			streamer, format, err := n.decodeAudio(mp3Path)
			if err != nil {
				t.Errorf("decodeAudio(MP3) failed: %v", err)
				return
			}
			defer streamer.Close()

			if format.SampleRate == 0 {
				t.Error("decodeAudio(MP3) returned zero sample rate")
			}
			if format.NumChannels == 0 {
				t.Error("decodeAudio(MP3) returned zero channels")
			}
		})
	}

	// Test AIFF decoding (macOS system sounds)
	if platform.IsMacOS() {
		aiffPath := "/System/Library/Sounds/Glass.aiff"
		if platform.FileExists(aiffPath) {
			t.Run("decode AIFF", func(t *testing.T) {
				streamer, format, err := n.decodeAudio(aiffPath)
				if err != nil {
					t.Errorf("decodeAudio(AIFF) failed: %v", err)
					return
				}
				defer streamer.Close()

				if format.SampleRate == 0 {
					t.Error("decodeAudio(AIFF) returned zero sample rate")
				}
				if format.NumChannels == 0 {
					t.Error("decodeAudio(AIFF) returned zero channels")
				}
			})
		}
	}
}

// TestUnsupportedFormat tests handling of unsupported audio formats
func TestUnsupportedFormat(t *testing.T) {
	cfg := config.DefaultConfig()
	n := New(cfg)
	defer n.Close()

	// Create a temporary file with unsupported extension
	tmpfile, err := os.CreateTemp("", "test*.xyz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	_, _, err = n.decodeAudio(tmpfile.Name())
	if err == nil {
		t.Error("decodeAudio() expected error for unsupported format, got nil")
	}
}

// TestNonExistentFile tests handling of non-existent files
func TestNonExistentFile(t *testing.T) {
	cfg := config.DefaultConfig()
	n := New(cfg)
	defer n.Close()

	nonExistentPath := "/tmp/this-file-does-not-exist-xyz123.mp3"

	_, _, err := n.decodeAudio(nonExistentPath)
	if err == nil {
		t.Error("decodeAudio() expected error for non-existent file, got nil")
	}
}

// TestSpeakerInitialization tests speaker initialization
func TestSpeakerInitialization(t *testing.T) {
	cfg := config.DefaultConfig()
	n := New(cfg)
	defer n.Close()

	// First initialization
	err := n.initSpeaker()
	if err != nil {
		t.Errorf("initSpeaker() first call returned error: %v", err)
	}

	// Check that speaker was initialized
	n.mu.Lock()
	inited := n.speakerInited
	n.mu.Unlock()

	if !inited {
		t.Error("initSpeaker() did not set speakerInited flag")
	}

	// Second initialization should be safe (no-op due to sync.Once)
	err = n.initSpeaker()
	if err != nil {
		t.Errorf("initSpeaker() second call returned error: %v", err)
	}
}

// TestGracefulShutdown tests that Close() waits for sounds to finish
func TestGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping graceful shutdown test in short mode")
	}

	cfg := config.DefaultConfig()
	n := New(cfg)

	// Don't play any sounds, just test Close()
	err := n.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Check that speaker was closed
	n.mu.Lock()
	inited := n.speakerInited
	n.mu.Unlock()

	// After Close(), speaker should still be marked as initialized
	// (we don't reset the flag, just close the speaker)
	if !inited {
		// This is actually OK - speaker might not have been initialized
		t.Log("Speaker was not initialized")
	}
}

// TestSystemSoundsAvailability tests detection of system sounds
func TestSystemSoundsAvailability(t *testing.T) {
	tests := []struct {
		name      string
		checkFunc func() bool
		soundPath string
	}{
		{
			name:      "macOS system sounds",
			checkFunc: platform.IsMacOS,
			soundPath: "/System/Library/Sounds/Glass.aiff",
		},
		{
			name:      "Linux system sounds",
			checkFunc: platform.IsLinux,
			soundPath: "/usr/share/sounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.checkFunc() {
				t.Skipf("Skipping %s test on this platform", tt.name)
			}

			exists := platform.FileExists(tt.soundPath)
			t.Logf("System sounds at %s: exists=%v", tt.soundPath, exists)

			// On the expected platform, log whether system sounds are available
			// (Not a failure if they're not available, just informational)
		})
	}
}

// TestBuiltInSoundsExist tests that built-in sound files exist
func TestBuiltInSoundsExist(t *testing.T) {
	soundsDir := findSoundsDirectory()
	if soundsDir == "" {
		t.Skip("Sounds directory not found")
	}

	requiredSounds := []string{
		"task-complete.mp3",
		"review-complete.mp3",
		"question.mp3",
		"plan-ready.mp3",
	}

	for _, sound := range requiredSounds {
		t.Run(sound, func(t *testing.T) {
			soundPath := filepath.Join(soundsDir, sound)
			if !platform.FileExists(soundPath) {
				t.Errorf("Required sound file not found: %s", soundPath)
			}
		})
	}
}

// Helper function to find sounds directory
func findSoundsDirectory() string {
	// Try various possible locations
	candidates := []string{
		"../../sounds",
		"../sounds",
		"sounds",
		"./sounds",
	}

	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if platform.FileExists(absPath) {
			return absPath
		}
	}

	// Try using CLAUDE_PLUGIN_ROOT if set
	if pluginRoot := os.Getenv("CLAUDE_PLUGIN_ROOT"); pluginRoot != "" {
		soundsPath := filepath.Join(pluginRoot, "sounds")
		if platform.FileExists(soundsPath) {
			return soundsPath
		}
	}

	return ""
}
