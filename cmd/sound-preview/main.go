package main

import (
	"flag"
	"fmt"
	"log"
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
		if err := speaker.Init(sampleRate, sampleRate.N(time.Second/10)); err != nil {
			// Ignore "already initialized" error
			if err.Error() != "speaker cannot be initialized more than once" {
				log.Fatalf("Failed to initialize speaker: %v", err)
			}
		}

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

	// Apply volume control using effects.Gain
	// effects.Gain formula: output = input * (1 + Gain)
	// Examples: volume 1.0 → Gain 0.0 (100%), volume 0.3 → Gain -0.7 (30%)
	var gainStreamer beep.Streamer = resampled
	if volume < 1.0 {
		gainStreamer = &effects.Gain{
			Streamer: resampled,
			Gain:     volumeToGain(volume),
		}
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
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("playback timed out")
	}
}

// volumeToGain converts linear volume (0.0-1.0) to gain value for effects.Gain
// effects.Gain formula: output = input * (1 + Gain)
// Examples: volume 1.0 → Gain 0.0 (100%), volume 0.3 → Gain -0.7 (30%), volume 0.5 → Gain -0.5 (50%)
func volumeToGain(volume float64) float64 {
	return volume - 1.0
}
