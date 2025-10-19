package summary

import (
	"strings"
	"testing"
	"time"

	"github.com/belief/claude-notifications/internal/analyzer"
	"github.com/belief/claude-notifications/internal/config"
	"github.com/belief/claude-notifications/pkg/jsonl"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "Took 30s"},
		{90 * time.Second, "Took 1m 30s"},
		{120 * time.Second, "Took 2m"},
		{3661 * time.Second, "Took 1h 1m"},
		{3600 * time.Second, "Took 1h"},
		{7200 * time.Second, "Took 2h"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %s, want %s", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestBuildActionsString(t *testing.T) {
	tests := []struct {
		name       string
		toolCounts map[string]int
		duration   string
		expected   string
	}{
		{
			name:       "All actions with duration",
			toolCounts: map[string]int{"Write": 3, "Edit": 2, "Bash": 1},
			duration:   "Took 2m 15s",
			expected:   "Created 3 files. Edited 2 files. Ran 1 command. Took 2m 15s",
		},
		{
			name:       "Only write",
			toolCounts: map[string]int{"Write": 1},
			duration:   "",
			expected:   "Created 1 file",
		},
		{
			name:       "Multiple edits",
			toolCounts: map[string]int{"Edit": 5},
			duration:   "Took 30s",
			expected:   "Edited 5 files. Took 30s",
		},
		{
			name:       "No tools",
			toolCounts: map[string]int{},
			duration:   "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildActionsString(tt.toolCounts, tt.duration)
			if result != tt.expected {
				t.Errorf("buildActionsString() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestCleanMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "# Header\nSome text",
			expected: "Header Some text",
		},
		{
			input:    "- Item 1\n- Item 2",
			expected: "Item 1 Item 2",
		},
		{
			input:    "`code` and **bold**",
			expected: "code and **bold**", // Only backticks removed, not **
		},
		{
			input:    "Multiple    spaces",
			expected: "Multiple spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CleanMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("CleanMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		text     string
		maxLen   int
		expected string
	}{
		{
			text:     "Short text",
			maxLen:   100,
			expected: "Short text",
		},
		{
			text:     strings.Repeat("a", 200),
			maxLen:   50,
			expected: strings.Repeat("a", 47) + "...",
		},
		{
			text:     "This is a long text that should be truncated at word boundary",
			maxLen:   30,
			expected: "This is a long text that...",
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := truncateText(tt.text, tt.maxLen)
			if len(result) > tt.maxLen {
				t.Errorf("truncateText() returned text longer than maxLen: %d > %d", len(result), tt.maxLen)
			}
			if !strings.HasPrefix(result, tt.expected[:10]) {
				t.Errorf("truncateText() = %s, want prefix %s", result, tt.expected[:10])
			}
		})
	}
}

func TestExtractFirstSentence(t *testing.T) {
	tests := []struct {
		text     string
		expected string
	}{
		{
			text:     "First sentence. Second sentence.",
			expected: "First sentence",
		},
		{
			text:     "Question? Answer.",
			expected: "Question",
		},
		{
			text:     "Exclamation! More text.",
			expected: "Exclamation",
		},
		{
			text:     strings.Repeat("a", 150),
			expected: strings.Repeat("a", 100),
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := extractFirstSentence(tt.text)
			if result != tt.expected {
				t.Errorf("extractFirstSentence() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestExtractAskUserQuestion(t *testing.T) {
	// Test with mock messages
	now := time.Now()
	recentTime := now.Format(time.RFC3339)
	oldTime := now.Add(-120 * time.Second).Format(time.RFC3339)

	tests := []struct {
		name           string
		messages       []jsonl.Message
		expectQuestion string
		expectRecent   bool
	}{
		{
			name: "Recent AskUserQuestion",
			messages: []jsonl.Message{
				{
					Type:      "assistant",
					Timestamp: recentTime,
					Message: jsonl.MessageContent{
						Content: []jsonl.Content{
							{
								Type: "tool_use",
								Name: "AskUserQuestion",
								Input: map[string]interface{}{
									"questions": []interface{}{
										map[string]interface{}{
											"question": "Which API should we use?",
										},
									},
								},
							},
						},
					},
				},
				{
					Type:      "assistant",
					Timestamp: now.Add(10 * time.Second).Format(time.RFC3339),
					Message: jsonl.MessageContent{
						Content: []jsonl.Content{
							{Type: "text", Text: "Some text"},
						},
					},
				},
			},
			expectQuestion: "Which API should we use?",
			expectRecent:   true,
		},
		{
			name: "Old AskUserQuestion",
			messages: []jsonl.Message{
				{
					Type:      "assistant",
					Timestamp: oldTime,
					Message: jsonl.MessageContent{
						Content: []jsonl.Content{
							{
								Type: "tool_use",
								Name: "AskUserQuestion",
								Input: map[string]interface{}{
									"questions": []interface{}{
										map[string]interface{}{
											"question": "Old question",
										},
									},
								},
							},
						},
					},
				},
				{
					Type:      "assistant",
					Timestamp: recentTime,
					Message: jsonl.MessageContent{
						Content: []jsonl.Content{
							{Type: "text", Text: "Recent text"},
						},
					},
				},
			},
			expectQuestion: "Old question",
			expectRecent:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question, isRecent := extractAskUserQuestion(tt.messages)
			if question != tt.expectQuestion {
				t.Errorf("extractAskUserQuestion() question = %s, want %s", question, tt.expectQuestion)
			}
			if isRecent != tt.expectRecent {
				t.Errorf("extractAskUserQuestion() isRecent = %v, want %v", isRecent, tt.expectRecent)
			}
		})
	}
}

func TestCountToolsByType(t *testing.T) {
	baseTime := time.Now()
	userTime := baseTime.Format(time.RFC3339)
	afterTime := baseTime.Add(10 * time.Second).Format(time.RFC3339)
	beforeTime := baseTime.Add(-10 * time.Second).Format(time.RFC3339)

	messages := []jsonl.Message{
		{
			Type:      "user",
			Timestamp: userTime,
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{Type: "text", Text: "Do something"},
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: beforeTime,
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{Type: "tool_use", Name: "Read"}, // Before user - should NOT count
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: afterTime,
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{Type: "tool_use", Name: "Write"},
					{Type: "tool_use", Name: "Edit"},
					{Type: "tool_use", Name: "Write"},
				},
			},
		},
	}

	counts := countToolsByType(messages)

	if counts["Write"] != 2 {
		t.Errorf("Write count = %d, want 2", counts["Write"])
	}
	if counts["Edit"] != 1 {
		t.Errorf("Edit count = %d, want 1", counts["Edit"])
	}
	if counts["Read"] != 0 {
		t.Errorf("Read count = %d, want 0 (before user message)", counts["Read"])
	}
}

func TestGetDefaultMessage(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		status   string
		expected string
	}{
		{"task_complete", "Task Completed"},
		{"question", "Claude Has Questions"},
		{"plan_ready", "Plan Ready for Review"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := GetDefaultMessage(analyzer.Status(tt.status), cfg)
			// Default message removes emoji, so check if expected text is contained
			if !strings.Contains(result, tt.expected) {
				t.Errorf("GetDefaultMessage(%s) = %s, want to contain %s", tt.status, result, tt.expected)
			}
		})
	}
}
