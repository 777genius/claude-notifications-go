package sessionname

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSessionName(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{
			name:      "Valid UUID",
			sessionID: "73b5e210-ec1a-4294-96e4-c2aecb2e1063",
			expected:  "zesty-peak", // Deterministic based on hash
		},
		{
			name:      "Different UUID",
			sessionID: "12345678-1234-1234-1234-123456789abc",
			expected:  "brave-deer", // Different deterministic result
		},
		{
			name:      "Empty session ID",
			sessionID: "",
			expected:  "unknown-session",
		},
		{
			name:      "Unknown session ID",
			sessionID: "unknown",
			expected:  "unknown-session",
		},
		{
			name:      "Short session ID",
			sessionID: "short",
			expected:  "unknown-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSessionName(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateSessionNameDeterministic(t *testing.T) {
	sessionID := "73b5e210-ec1a-4294-96e4-c2aecb2e1063"

	// Generate name multiple times
	name1 := GenerateSessionName(sessionID)
	name2 := GenerateSessionName(sessionID)
	name3 := GenerateSessionName(sessionID)

	// Should always return the same name
	assert.Equal(t, name1, name2)
	assert.Equal(t, name2, name3)
}

func TestGenerateSessionNameFormat(t *testing.T) {
	sessionID := "73b5e210-ec1a-4294-96e4-c2aecb2e1063"
	name := GenerateSessionName(sessionID)

	// Should be in format "adjective-noun"
	assert.Contains(t, name, "-")
	assert.NotEmpty(t, name)
}

func TestHexToInt(t *testing.T) {
	tests := []struct {
		hex      string
		expected int
	}{
		{"73b5e2", 7583202},
		{"ec1a42", 15473218},
		{"000000", 0},
		{"ffffff", 16777215},
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			result := hexToInt(tt.hex)
			assert.Equal(t, tt.expected, result)
		})
	}
}
