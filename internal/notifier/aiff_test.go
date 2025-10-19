package notifier

import (
	"os"
	"testing"

	"github.com/go-audio/audio"
)

// === Helper Functions ===

// createMockAIFFBuffer creates a mock AIFF audio buffer for testing
func createMockAIFFBuffer(numChannels, numSamples int, sampleRate int) *audio.IntBuffer {
	// Create mock PCM data
	data := make([]int, numSamples*numChannels)
	for i := range data {
		// Simple sine wave pattern
		data[i] = int((float64(i) / float64(len(data))) * 32767)
	}

	return &audio.IntBuffer{
		Data: data,
		Format: &audio.Format{
			NumChannels: numChannels,
			SampleRate:  sampleRate,
		},
	}
}

// === aiffStreamer Tests ===

func TestAIFFStreamer_Stream_Mono(t *testing.T) {
	// Create mono buffer (1 channel, 100 samples)
	buffer := createMockAIFFBuffer(1, 100, 44100)

	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil, // No actual file for this test
	}

	// Stream some samples
	samples := make([][2]float64, 10)
	n, ok := streamer.Stream(samples)

	if !ok || n != 10 {
		t.Errorf("Stream() = (%d, %v), want (10, true)", n, ok)
	}

	// Verify samples are in valid range [-1, 1]
	for i := 0; i < n; i++ {
		if samples[i][0] < -1.0 || samples[i][0] > 1.0 {
			t.Errorf("Sample %d left channel out of range: %f", i, samples[i][0])
		}
		if samples[i][1] < -1.0 || samples[i][1] > 1.0 {
			t.Errorf("Sample %d right channel out of range: %f", i, samples[i][1])
		}
		// For mono, both channels should be identical
		if samples[i][0] != samples[i][1] {
			t.Errorf("Mono sample %d: left != right (%f != %f)", i, samples[i][0], samples[i][1])
		}
	}

	// Verify position advanced
	expectedPos := 10 // 10 samples read from mono channel
	if streamer.pos != expectedPos {
		t.Errorf("After Stream(), pos = %d, want %d", streamer.pos, expectedPos)
	}
}

func TestAIFFStreamer_Stream_Stereo(t *testing.T) {
	// Create stereo buffer (2 channels, 100 samples)
	buffer := createMockAIFFBuffer(2, 100, 44100)

	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	// Stream some samples
	samples := make([][2]float64, 10)
	n, ok := streamer.Stream(samples)

	if !ok || n != 10 {
		t.Errorf("Stream() = (%d, %v), want (10, true)", n, ok)
	}

	// Verify samples are in valid range
	for i := 0; i < n; i++ {
		if samples[i][0] < -1.0 || samples[i][0] > 1.0 {
			t.Errorf("Sample %d left out of range: %f", i, samples[i][0])
		}
		if samples[i][1] < -1.0 || samples[i][1] > 1.0 {
			t.Errorf("Sample %d right out of range: %f", i, samples[i][1])
		}
	}

	// Verify position advanced (2 channels * 10 samples = 20)
	expectedPos := 20
	if streamer.pos != expectedPos {
		t.Errorf("After Stream(), pos = %d, want %d", streamer.pos, expectedPos)
	}
}

func TestAIFFStreamer_Stream_MultiChannel(t *testing.T) {
	// Create 5.1 surround buffer (6 channels, 60 samples)
	buffer := createMockAIFFBuffer(6, 60, 48000)

	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	// Stream some samples
	samples := make([][2]float64, 10)
	n, ok := streamer.Stream(samples)

	if !ok || n != 10 {
		t.Errorf("Stream() = (%d, %v), want (10, true)", n, ok)
	}

	// For multi-channel, only first 2 channels are used (stereo downmix)
	// Position should advance by numChannels * numSamples
	expectedPos := 60 // 6 channels * 10 samples
	if streamer.pos != expectedPos {
		t.Errorf("After Stream(), pos = %d, want %d", streamer.pos, expectedPos)
	}
}

func TestAIFFStreamer_Stream_EndOfStream(t *testing.T) {
	// Create small buffer (1 channel, 5 samples)
	buffer := createMockAIFFBuffer(1, 5, 44100)

	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	// Try to read 10 samples (more than available)
	samples := make([][2]float64, 10)
	n, ok := streamer.Stream(samples)

	// Should return 5 samples and ok=true
	if n != 5 || !ok {
		t.Errorf("Stream() = (%d, %v), want (5, true)", n, ok)
	}

	// Try to read again (should be at end)
	n, ok = streamer.Stream(samples)
	if n != 0 || ok {
		t.Errorf("Stream() after end = (%d, %v), want (0, false)", n, ok)
	}
}

func TestAIFFStreamer_Stream_EmptyBuffer(t *testing.T) {
	// Empty buffer
	streamer := &aiffStreamer{
		buffer: nil,
		pos:    0,
		file:   nil,
	}

	samples := make([][2]float64, 10)
	n, ok := streamer.Stream(samples)

	if n != 0 || ok {
		t.Errorf("Stream() with nil buffer = (%d, %v), want (0, false)", n, ok)
	}
}

func TestAIFFStreamer_Err(t *testing.T) {
	buffer := createMockAIFFBuffer(1, 100, 44100)
	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	// Err() should always return nil (no error tracking)
	if err := streamer.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
}

func TestAIFFStreamer_Len(t *testing.T) {
	tests := []struct {
		name        string
		numChannels int
		numSamples  int
		wantLen     int
	}{
		{"Mono", 1, 100, 100},         // 100 samples * 1 channel = 100 data points, Len = 100 / 1 = 100
		{"Stereo", 2, 100, 100},       // 100 samples * 2 channels = 200 data points, Len = 200 / 2 = 100
		{"5.1 Surround", 6, 100, 100}, // 100 samples * 6 channels = 600 data points, Len = 600 / 6 = 100
		{"Zero channels", 0, 100, 0},  // 100 samples * 0 channels = 0 data points, Len = 0 / 1 = 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := createMockAIFFBuffer(tt.numChannels, tt.numSamples, 44100)
			streamer := &aiffStreamer{
				buffer: buffer,
				pos:    0,
				file:   nil,
			}

			length := streamer.Len()
			if length != tt.wantLen {
				t.Errorf("Len() = %d, want %d", length, tt.wantLen)
			}
		})
	}
}

func TestAIFFStreamer_Len_NilBuffer(t *testing.T) {
	streamer := &aiffStreamer{
		buffer: nil,
		pos:    0,
		file:   nil,
	}

	length := streamer.Len()
	if length != 0 {
		t.Errorf("Len() with nil buffer = %d, want 0", length)
	}
}

func TestAIFFStreamer_Position(t *testing.T) {
	buffer := createMockAIFFBuffer(2, 100, 44100)
	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    40, // 40 data points = 20 samples in stereo
		file:   nil,
	}

	position := streamer.Position()
	expectedPosition := 20 // 40 / 2 channels
	if position != expectedPosition {
		t.Errorf("Position() = %d, want %d", position, expectedPosition)
	}
}

func TestAIFFStreamer_Position_ZeroChannels(t *testing.T) {
	buffer := createMockAIFFBuffer(0, 100, 44100)
	buffer.Format.NumChannels = 0 // Force zero channels

	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    50,
		file:   nil,
	}

	// Should handle zero channels gracefully (defaults to 1)
	position := streamer.Position()
	if position != 50 {
		t.Errorf("Position() with 0 channels = %d, want 50", position)
	}
}

func TestAIFFStreamer_Seek(t *testing.T) {
	buffer := createMockAIFFBuffer(2, 100, 44100)
	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	// Seek to sample 25 (stereo: 25 * 2 = 50 data points)
	err := streamer.Seek(25)
	if err != nil {
		t.Fatalf("Seek(25) error: %v", err)
	}

	if streamer.pos != 50 {
		t.Errorf("After Seek(25), pos = %d, want 50", streamer.pos)
	}

	// Verify we can stream from new position
	samples := make([][2]float64, 5)
	n, ok := streamer.Stream(samples)
	if !ok || n != 5 {
		t.Errorf("Stream after Seek() = (%d, %v), want (5, true)", n, ok)
	}
}

func TestAIFFStreamer_Seek_ZeroChannels(t *testing.T) {
	buffer := createMockAIFFBuffer(0, 100, 44100)
	buffer.Format.NumChannels = 0

	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	err := streamer.Seek(50)
	if err != nil {
		t.Fatalf("Seek(50) with 0 channels error: %v", err)
	}

	// Should default to 1 channel
	if streamer.pos != 50 {
		t.Errorf("After Seek(50), pos = %d, want 50", streamer.pos)
	}
}

func TestAIFFStreamer_Close_NoFile(t *testing.T) {
	streamer := &aiffStreamer{
		buffer: createMockAIFFBuffer(1, 100, 44100),
		pos:    0,
		file:   nil,
	}

	err := streamer.Close()
	if err != nil {
		t.Errorf("Close() with nil file = %v, want nil", err)
	}
}

func TestAIFFStreamer_Close_WithFile(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "test-aiff-*.dat")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open file for reading
	file, err := os.Open(tmpPath)
	if err != nil {
		t.Fatalf("Failed to open temp file: %v", err)
	}

	streamer := &aiffStreamer{
		buffer: createMockAIFFBuffer(1, 100, 44100),
		pos:    0,
		file:   file,
	}

	err = streamer.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Verify file is closed (reading should fail)
	buf := make([]byte, 10)
	_, err = file.Read(buf)
	if err == nil {
		t.Error("File should be closed, but Read() succeeded")
	}
}

func TestAIFFStreamer_FullStreamCycle(t *testing.T) {
	// Integration test: full cycle of stream operations
	buffer := createMockAIFFBuffer(2, 50, 44100)
	streamer := &aiffStreamer{
		buffer: buffer,
		pos:    0,
		file:   nil,
	}

	// 1. Check initial state
	if streamer.Position() != 0 {
		t.Errorf("Initial Position() = %d, want 0", streamer.Position())
	}
	if streamer.Len() != 50 {
		t.Errorf("Len() = %d, want 50", streamer.Len())
	}

	// 2. Stream first 10 samples
	samples := make([][2]float64, 10)
	n, ok := streamer.Stream(samples)
	if !ok || n != 10 {
		t.Fatalf("First Stream() = (%d, %v), want (10, true)", n, ok)
	}
	if streamer.Position() != 10 {
		t.Errorf("Position after stream = %d, want 10", streamer.Position())
	}

	// 3. Seek to position 30
	err := streamer.Seek(30)
	if err != nil {
		t.Fatalf("Seek(30) error: %v", err)
	}
	if streamer.Position() != 30 {
		t.Errorf("Position after seek = %d, want 30", streamer.Position())
	}

	// 4. Stream remaining samples (20 left)
	samples = make([][2]float64, 25) // Request more than available
	n, ok = streamer.Stream(samples)
	if n != 20 || !ok {
		t.Errorf("Final Stream() = (%d, %v), want (20, true)", n, ok)
	}

	// 5. Try streaming at end
	n, ok = streamer.Stream(samples)
	if n != 0 || ok {
		t.Errorf("Stream at end = (%d, %v), want (0, false)", n, ok)
	}

	// 6. Seek back to start
	err = streamer.Seek(0)
	if err != nil {
		t.Fatalf("Seek(0) error: %v", err)
	}
	if streamer.Position() != 0 {
		t.Errorf("Position after seek to start = %d, want 0", streamer.Position())
	}

	// 7. Close
	err = streamer.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

// === decodeAudio Tests ===

func TestDecodeAudio_UnsupportedFormat(t *testing.T) {
	// Create temp file with unsupported extension
	tmpFile, err := os.CreateTemp("", "test-audio-*.xyz")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	n := &Notifier{cfg: nil}
	_, _, err = n.decodeAudio(tmpFile.Name())

	if err == nil {
		t.Fatal("decodeAudio() should fail for unsupported format, got nil")
	}

	if !contains(err.Error(), "unsupported audio format") {
		t.Errorf("Expected 'unsupported audio format' error, got: %v", err)
	}
}

func TestDecodeAudio_NonexistentFile(t *testing.T) {
	n := &Notifier{cfg: nil}
	_, _, err := n.decodeAudio("/nonexistent/file.mp3")

	if err == nil {
		t.Fatal("decodeAudio() should fail for nonexistent file, got nil")
	}

	if !contains(err.Error(), "failed to open audio file") {
		t.Errorf("Expected 'failed to open' error, got: %v", err)
	}
}

func TestDecodeAudio_EmptyPath(t *testing.T) {
	n := &Notifier{cfg: nil}
	_, _, err := n.decodeAudio("")

	if err == nil {
		t.Fatal("decodeAudio() should fail for empty path, got nil")
	}
}

func TestDecodeAudio_SupportedExtensions(t *testing.T) {
	// Test that all supported extensions are recognized
	// (actual decoding will fail without valid audio data, but we test extension detection)
	extensions := []string{".mp3", ".wav", ".flac", ".ogg", ".aiff", ".aif"}

	for _, ext := range extensions {
		// Create temp file
		tmpFile, err := os.CreateTemp("", "test-audio-*"+ext)
		if err != nil {
			t.Fatalf("Failed to create temp file for %s: %v", ext, err)
		}
		tmpPath := tmpFile.Name()

		// Write some dummy data (not valid audio, but tests path handling)
		if _, err := tmpFile.Write([]byte("dummy data")); err != nil {
			t.Fatalf("failed to write test data: %v", err)
		}
		tmpFile.Close()
		defer os.Remove(tmpPath)

		n := &Notifier{cfg: nil}
		_, _, err = n.decodeAudio(tmpPath)

		// Should get a decoding error (not "unsupported format")
		if err != nil && contains(err.Error(), "unsupported audio format") {
			t.Errorf("Extension %s should be supported, but got unsupported format error", ext)
		}
	}
}
