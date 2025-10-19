package sessionname

import (
	"fmt"
	"strings"
)

// Lists for friendly name generation (same as bash version)
var adjectives = []string{
	"bold", "brave", "bright", "calm", "clever",
	"cool", "cosmic", "crisp", "daring", "eager",
	"fair", "fancy", "fast", "gentle", "glad",
	"grand", "happy", "kind", "lively", "lucky",
	"merry", "noble", "proud", "quick", "quiet",
	"rapid", "smart", "solid", "swift", "warm",
	"wise", "witty", "zesty", "agile", "alert",
}

var nouns = []string{
	"bear", "bird", "cat", "deer", "eagle",
	"fish", "fox", "hawk", "lion", "owl",
	"star", "moon", "sun", "wind", "wave",
	"tree", "river", "mountain", "ocean", "cloud",
	"tiger", "wolf", "dragon", "phoenix", "falcon",
	"comet", "galaxy", "planet", "nova", "meteor",
	"forest", "canyon", "valley", "peak", "storm",
}

// GenerateSessionName generates a friendly name from a session ID (UUID).
// Returns a deterministic name like "bold-cat" or "swift-eagle".
//
// Args:
//   - sessionID: UUID string (e.g., "73b5e210-ec1a-4294-96e4-c2aecb2e1063")
//
// Returns:
//   - Friendly name string (e.g., "bold-cat")
func GenerateSessionName(sessionID string) string {
	// Return "unknown-session" if no session ID
	if sessionID == "" || sessionID == "unknown" {
		return "unknown-session"
	}

	// Remove dashes and convert to lowercase
	cleanID := strings.ToLower(strings.ReplaceAll(sessionID, "-", ""))

	// Get first 8 chars for adjective seed, next 8 for noun seed
	if len(cleanID) < 16 {
		// Fallback for short IDs
		return "unknown-session"
	}

	adjSeed := cleanID[0:8]
	nounSeed := cleanID[8:16]

	// Convert hex to decimal for array indexing
	adjIndex := hexToInt(adjSeed) % len(adjectives)
	nounIndex := hexToInt(nounSeed) % len(nouns)

	return fmt.Sprintf("%s-%s", adjectives[adjIndex], nouns[nounIndex])
}

// hexToInt converts hex string to int (takes first 6 characters for safety)
func hexToInt(hex string) int {
	if len(hex) > 6 {
		hex = hex[0:6]
	}

	var result int
	if _, err := fmt.Sscanf(hex, "%x", &result); err != nil {
		return 0 // Return 0 on parse error
	}
	return result
}
