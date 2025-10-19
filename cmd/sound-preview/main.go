package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-audio/aiff"
	"github.com/go-audio/audio"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
	"github.com/gopxl/beep/wav"
)

var (
	speakerInit   sync.Once
	speakerInited bool
	mu            sync.Mutex
)

func main() {
	// Define flags
	volumeFlag := flag.Float64("volume", 1.0, "Volume level (0.0 to 1.0)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sound-preview [options] <path-to-audio-file>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nSupported formats: MP3, WAV, FLAC, OGG/Vorbis, AIFF\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  sound-preview sounds/task-complete.mp3\n")
		fmt.Fprintf(os.Stderr, "  sound-preview --volume 0.3 /System/Library/Sounds/Glass.aiff\n")
		fmt.Fprintf(os.Stderr, "  sound-preview --volume 0.5 sounds/question.mp3\n")
	}
	flag.Parse()

	// Validate volume range
	if *volumeFlag < 0.0 || *volumeFlag > 1.0 {
		fmt.Fprintf(os.Stderr, "Error: Volume must be between 0.0 and 1.0 (got %.2f)\n", *volumeFlag)
		os.Exit(1)
	}

	// Check if sound path is provided
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	soundPath := flag.Arg(0)

	// Check if file exists
	if _, err := os.Stat(soundPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Sound file not found: %s\n", soundPath)
		os.Exit(1)
	}

	// Show volume indicator
	volumePercent := int(*volumeFlag * 100)
	if *volumeFlag < 1.0 {
		fmt.Printf("🔉 Playing: %s (volume: %d%%)\n", filepath.Base(soundPath), volumePercent)
	} else {
		fmt.Printf("🔊 Playing: %s\n", filepath.Base(soundPath))
	}

	// Play the sound with volume control
	if err := playSound(soundPath, *volumeFlag); err != nil {
		fmt.Fprintf(os.Stderr, "Error playing sound: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Playback completed")
}

// initSpeaker initializes the speaker once with sync.Once
func initSpeaker() error {
	var initErr error

	speakerInit.Do(func() {
		// Initialize speaker with standard sample rate (44100 Hz)
		sampleRate := beep.SampleRate(44100)
		speaker.Init(sampleRate, sampleRate.N(time.Second/10))

		mu.Lock()
		speakerInited = true
		mu.Unlock()
	})

	return initErr
}

// decodeAudio decodes an audio file and returns a streamer and format
func decodeAudio(soundPath string) (beep.StreamSeekCloser, beep.Format, error) {
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
		decoder := aiff.NewDecoder(f)
		if !decoder.IsValidFile() {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("invalid AIFF file")
		}

		decoder.ReadInfo()

		format := beep.Format{
			SampleRate:  beep.SampleRate(decoder.SampleRate),
			NumChannels: int(decoder.NumChans),
			Precision:   2,
		}

		buf, err := decoder.FullPCMBuffer()
		if err != nil {
			f.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to read AIFF data: %w", err)
		}

		streamer := &aiffStreamer{
			buffer: buf,
			pos:    0,
			file:   f,
		}

		return streamer, format, nil

	default:
		f.Close()
		return nil, beep.Format{}, fmt.Errorf("unsupported audio format: %s (supported: .mp3, .wav, .flac, .ogg, .aiff)", ext)
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

		samples[i][0] = float64(intData[s.pos]) / 32768.0
		s.pos++

		if numChannels == 1 {
			samples[i][1] = samples[i][0]
		} else {
			if s.pos >= len(intData) {
				return i + 1, i >= 0
			}
			samples[i][1] = float64(intData[s.pos]) / 32768.0
			s.pos++
		}

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

// playSound plays a sound file using gopxl/beep with volume control
func playSound(soundPath string, volume float64) error {
	// Initialize speaker once
	if err := initSpeaker(); err != nil {
		return fmt.Errorf("failed to initialize speaker: %w", err)
	}

	// Decode audio file
	streamer, format, err := decodeAudio(soundPath)
	if err != nil {
		return err
	}
	defer streamer.Close()

	// Resample if needed (convert to speaker's sample rate: 44100 Hz)
	resampled := beep.Resample(4, format.SampleRate, beep.SampleRate(44100), streamer)

	// Apply volume control
	// effects.Volume uses a base of 2, so we need to convert:
	// volume 1.0 (100%) -> base 0 (no change)
	// volume 0.5 (50%)  -> base -1 (half volume)
	// volume 0.3 (30%)  -> base ~-1.7
	// Formula: base = log2(volume)
	var volumeStreamer beep.Streamer = resampled
	if volume < 1.0 {
		// Convert linear volume (0.0-1.0) to decibel-like scale
		// Using Volume with base and silent flag
		volumeStreamer = &effects.Volume{
			Streamer: resampled,
			Base:     2,
			Volume:   volumeToBase(volume),
			Silent:   false,
		}
	}

	// Create done channel to wait for playback completion
	done := make(chan bool)

	// Play sound with callback when finished
	speaker.Play(beep.Seq(volumeStreamer, beep.Callback(func() {
		done <- true
	})))

	// Wait for playback to complete with timeout
	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("playback timed out")
	}
}

// volumeToBase converts linear volume (0.0-1.0) to beep.Volume base
// beep uses exponential scale: volume = base^units
// We want: when volume=0.5, actual volume should be 0.5
// So: 0.5 = 2^units => units = log2(0.5) = -1
func volumeToBase(volume float64) float64 {
	if volume <= 0 {
		return -10 // Very quiet (almost silent)
	}
	if volume >= 1.0 {
		return 0 // Full volume
	}
	// log2(volume) gives us the right units
	// For volume=0.5: log2(0.5) = -1
	// For volume=0.3: log2(0.3) ≈ -1.737
	// For volume=0.1: log2(0.1) ≈ -3.322
	return logBase2(volume)
}

// logBase2 calculates log base 2 of x
func logBase2(x float64) float64 {
	// log2(x) = ln(x) / ln(2)
	return math.Log(x) / math.Log(2)
}
