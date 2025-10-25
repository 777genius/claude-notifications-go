package notifier

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/go-audio/aiff"
	"github.com/go-audio/audio"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
	"github.com/gopxl/beep/wav"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/errorhandler"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/platform"
)

// Notifier sends desktop notifications
type Notifier struct {
	cfg           *config.Config
	speakerInit   sync.Once
	speakerInited bool
	mu            sync.Mutex
	wg            sync.WaitGroup
}

// New creates a new notifier
func New(cfg *config.Config) *Notifier {
	return &Notifier{
		cfg: cfg,
	}
}

// SendDesktop sends a desktop notification using beeep (cross-platform)
func (n *Notifier) SendDesktop(status analyzer.Status, message string) error {
	if !n.cfg.IsDesktopEnabled() {
		logging.Debug("Desktop notifications disabled, skipping")
		return nil
	}

	statusInfo, exists := n.cfg.GetStatusInfo(string(status))
	if !exists {
		return fmt.Errorf("unknown status: %s", status)
	}

	// Extract session name from message (format: "[session-name] actual message")
	sessionName, cleanMessage := extractSessionName(message)

	// Build proper title with session name
	title := statusInfo.Title
	if sessionName != "" {
		title = fmt.Sprintf("%s [%s]", title, sessionName)
	}

	// Get app icon path if configured
	appIcon := n.cfg.Notifications.Desktop.AppIcon
	if appIcon != "" && !platform.FileExists(appIcon) {
		logging.Warn("App icon not found: %s, using default", appIcon)
		appIcon = ""
	}

	// Set unique AppName to prevent notification grouping/replacement
	// Each notification gets a unique group ID based on timestamp
	originalAppName := beeep.AppName
	beeep.AppName = fmt.Sprintf("claude-notif-%d", time.Now().UnixNano())
	defer func() {
		beeep.AppName = originalAppName
	}()

	// Send notification using beeep with proper title and clean message
	if err := beeep.Notify(title, cleanMessage, appIcon); err != nil {
		logging.Error("Failed to send desktop notification: %v", err)
		return err
	}

	logging.Debug("Desktop notification sent via beeep: title=%s", title)

	// Play sound if enabled (sequential playback handled by speaker mixer)
	if n.cfg.Notifications.Desktop.Sound && statusInfo.Sound != "" {
		n.wg.Add(1)
		// Use SafeGo to protect against panics in sound playback goroutine
		errorhandler.SafeGo(func() {
			defer n.wg.Done()
			n.playSound(statusInfo.Sound)
		})
	}

	return nil
}

// initSpeaker initializes the speaker once with sync.Once
func (n *Notifier) initSpeaker() error {
	// Check if already initialized
	n.mu.Lock()
	if n.speakerInited {
		n.mu.Unlock()
		return nil
	}
	n.mu.Unlock()

	var initErr error

	n.speakerInit.Do(func() {
		// Initialize speaker with standard sample rate (44100 Hz) and buffer size (4096 samples)
		// Buffer size of 4096 samples = ~93ms latency at 44100 Hz
		sampleRate := beep.SampleRate(44100)
		err := speaker.Init(sampleRate, sampleRate.N(time.Second/10))

		// Ignore "already initialized" error - can happen in tests
		if err != nil && err.Error() != "speaker cannot be initialized more than once" {
			initErr = err
		}

		n.mu.Lock()
		n.speakerInited = true
		n.mu.Unlock()

		logging.Debug("Speaker initialized: sampleRate=%d Hz, buffer=4096 samples", sampleRate)
	})

	return initErr
}

// decodeAudio decodes an audio file and returns a streamer and format
// Supports: MP3, WAV, FLAC, AIFF, Vorbis (OGG)
func (n *Notifier) decodeAudio(soundPath string) (beep.StreamSeekCloser, beep.Format, error) {
	f, err := os.Open(soundPath)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("failed to open audio file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(soundPath))

	switch ext {
	case ".mp3":
		streamer, format, err := mp3.Decode(f)
		if err != nil {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to decode MP3: %w", err)
		}
		return streamer, format, nil

	case ".wav":
		streamer, format, err := wav.Decode(f)
		if err != nil {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to decode WAV: %w", err)
		}
		return streamer, format, nil

	case ".flac":
		streamer, format, err := flac.Decode(f)
		if err != nil {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to decode FLAC: %w", err)
		}
		return streamer, format, nil

	case ".ogg":
		streamer, format, err := vorbis.Decode(f)
		if err != nil {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to decode Vorbis: %w", err)
		}
		return streamer, format, nil

	case ".aiff", ".aif":
		// AIFF requires special handling - decode to PCM then convert to beep streamer
		decoder := aiff.NewDecoder(f)
		if !decoder.IsValidFile() {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("invalid AIFF file")
		}

		// Read AIFF format info
		decoder.ReadInfo()

		// Create custom streamer for AIFF
		format := beep.Format{
			SampleRate:  beep.SampleRate(decoder.SampleRate),
			NumChannels: int(decoder.NumChans),
			Precision:   2, // 16-bit
		}

		// Read all PCM data
		buf, err := decoder.FullPCMBuffer()
		if err != nil {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to read AIFF data: %w", err)
		}

		// Convert PCM buffer to beep.StreamSeekCloser
		streamer := &aiffStreamer{
			buffer: buf,
			pos:    0,
			file:   f,
		}

		return streamer, format, nil

	default:
		f.Close()
		return nil, beep.Format{}, fmt.Errorf("unsupported audio format: %s", ext)
	}
}

// aiffStreamer implements beep.StreamSeekCloser for AIFF files
type aiffStreamer struct {
	buffer *audio.IntBuffer
	pos    int
	file   *os.File
}

func (s *aiffStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	if s.buffer == nil || len(s.buffer.Data) == 0 {
		return 0, false
	}

	numChannels := s.buffer.Format.NumChannels
	intData := s.buffer.Data

	for i := range samples {
		if s.pos >= len(intData) {
			return i, i > 0
		}

		// Convert int samples to float64 in range [-1, 1]
		// Mono or multi-channel handling
		samples[i][0] = float64(intData[s.pos]) / 32768.0
		s.pos++

		if numChannels == 1 {
			// Mono: duplicate to both channels
			samples[i][1] = samples[i][0]
		} else {
			// Stereo or multi-channel: read second channel
			if s.pos >= len(intData) {
				return i + 1, i >= 0
			}
			samples[i][1] = float64(intData[s.pos]) / 32768.0
			s.pos++
		}

		// Skip additional channels if more than 2
		for c := 2; c < numChannels && s.pos < len(intData); c++ {
			s.pos++
		}
	}

	return len(samples), true
}

func (s *aiffStreamer) Err() error {
	return nil
}

func (s *aiffStreamer) Len() int {
	if s.buffer == nil || len(s.buffer.Data) == 0 {
		return 0
	}
	numChannels := s.buffer.Format.NumChannels
	if numChannels == 0 {
		numChannels = 1
	}
	return len(s.buffer.Data) / numChannels
}

func (s *aiffStreamer) Position() int {
	numChannels := s.buffer.Format.NumChannels
	if numChannels == 0 {
		numChannels = 1
	}
	return s.pos / numChannels
}

func (s *aiffStreamer) Seek(p int) error {
	numChannels := s.buffer.Format.NumChannels
	if numChannels == 0 {
		numChannels = 1
	}
	s.pos = p * numChannels
	return nil
}

func (s *aiffStreamer) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// playSound plays a sound file using gopxl/beep (cross-platform) with volume control
func (n *Notifier) playSound(soundPath string) {
	if !platform.FileExists(soundPath) {
		logging.Warn("Sound file not found: %s", soundPath)
		return
	}

	// Initialize speaker once
	if err := n.initSpeaker(); err != nil {
		logging.Error("Failed to initialize speaker: %v", err)
		return
	}

	// Decode audio file
	streamer, format, err := n.decodeAudio(soundPath)
	if err != nil {
		logging.Error("Failed to decode audio %s: %v", soundPath, err)
		return
	}
	defer streamer.Close()

	// Resample if needed (convert to speaker's sample rate: 44100 Hz)
	resampled := beep.Resample(4, format.SampleRate, beep.SampleRate(44100), streamer)

	// Apply volume control from config
	volume := n.cfg.Notifications.Desktop.Volume
	var gainStreamer beep.Streamer = resampled
	if volume < 1.0 {
		gainStreamer = &effects.Gain{
			Streamer: resampled,
			Gain:     volumeToGain(volume),
		}
		logging.Debug("Applying volume control: %.0f%%", volume*100)
	}

	// Create done channel to wait for playback completion
	done := make(chan bool)

	// Play sound with callback when finished
	speaker.Play(beep.Seq(gainStreamer, beep.Callback(func() {
		done <- true
	})))

	// Wait for playback to complete with timeout
	select {
	case <-done:
		logging.Debug("Sound played successfully: %s (volume: %.0f%%)", soundPath, volume*100)
	case <-time.After(30 * time.Second):
		logging.Warn("Sound playback timed out: %s", soundPath)
	}
}

// volumeToGain converts linear volume (0.0-1.0) to gain value for effects.Gain
// effects.Gain formula: output = input * (1 + Gain)
// Examples: volume 1.0 → Gain 0.0 (100%), volume 0.3 → Gain -0.7 (30%), volume 0.5 → Gain -0.5 (50%)
func volumeToGain(volume float64) float64 {
	return volume - 1.0
}

// Close waits for all sounds to finish playing and cleans up resources
func (n *Notifier) Close() error {
	// Wait for all sounds to finish
	n.wg.Wait()

	// Close speaker if it was initialized
	n.mu.Lock()
	if n.speakerInited {
		speaker.Close()
		logging.Debug("Speaker closed")
	}
	n.mu.Unlock()

	return nil
}

// extractSessionName extracts session name from message with format "[session-name] message"
// Returns session name and clean message without the prefix
func extractSessionName(message string) (string, string) {
	message = strings.TrimSpace(message)

	// Check if message starts with [
	if !strings.HasPrefix(message, "[") {
		return "", message
	}

	// Find closing bracket
	closingIdx := strings.Index(message, "]")
	if closingIdx == -1 {
		return "", message
	}

	// Extract session name (without brackets)
	sessionName := message[1:closingIdx]

	// Extract clean message (everything after "] ")
	cleanMessage := strings.TrimSpace(message[closingIdx+1:])

	return sessionName, cleanMessage
}
