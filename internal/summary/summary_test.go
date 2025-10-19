package summary

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/777genius/claude-notifications/internal/analyzer"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/pkg/jsonl"
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
		name     string
		input    string
		expected string
	}{
		{
			name:     "Headers",
			input:    "# Header\nSome text",
			expected: "Header Some text",
		},
		{
			name:     "Bullet lists",
			input:    "- Item 1\n- Item 2",
			expected: "Item 1 Item 2",
		},
		{
			name:     "Bold with **",
			input:    "This is **bold text** here",
			expected: "This is bold text here",
		},
		{
			name:     "Bold with __",
			input:    "This is __bold text__ here",
			expected: "This is bold text here",
		},
		{
			name:     "Italic with *",
			input:    "This is *italic text* here",
			expected: "This is italic text here",
		},
		{
			name:     "Italic with _",
			input:    "This is _italic text_ here",
			expected: "This is italic text here",
		},
		{
			name:     "Links",
			input:    "Check [this link](https://example.com) out",
			expected: "Check this link out",
		},
		{
			name:     "Images",
			input:    "See ![cat image](https://example.com/cat.jpg) here",
			expected: "See cat image here",
		},
		{
			name:     "Strikethrough",
			input:    "This is ~~deleted~~ text",
			expected: "This is deleted text",
		},
		{
			name:     "Code blocks",
			input:    "Some text\n```python\nprint('hello')\n```\nMore text",
			expected: "Some text More text",
		},
		{
			name:     "Inline code",
			input:    "`code` and text",
			expected: "code and text",
		},
		{
			name:     "Blockquotes",
			input:    "> This is a quote\nNormal text",
			expected: "This is a quote Normal text",
		},
		{
			name:     "Multiple markdown",
			input:    "# Title\n**Bold** and *italic* with [link](url)",
			expected: "Title Bold and italic with link",
		},
		{
			name:     "Multiple spaces",
			input:    "Multiple    spaces",
			expected: "Multiple spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("CleanMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected string
	}{
		{
			name:     "Short text",
			text:     "Short text",
			maxLen:   100,
			expected: "Short text",
		},
		{
			name:     "Truncate at sentence boundary",
			text:     "This is first sentence. This is second sentence. This is third sentence.",
			maxLen:   50,
			expected: "This is first sentence.",
		},
		{
			name:     "Truncate with exclamation",
			text:     "Hello world! This is great! How are you doing today?",
			maxLen:   30,
			expected: "Hello world!",
		},
		{
			name:     "Truncate with question mark",
			text:     "What is this? Something else here with more text.",
			maxLen:   25,
			expected: "What is this?",
		},
		{
			name:     "No sentence boundary - truncate at word",
			text:     "This is a long text that should be truncated at word boundary",
			maxLen:   30,
			expected: "This is a long text that...",
		},
		{
			name:     "Very long word",
			text:     strings.Repeat("a", 200),
			maxLen:   50,
			expected: strings.Repeat("a", 47) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.text, tt.maxLen)
			if len(result) > tt.maxLen {
				t.Errorf("truncateText() returned text longer than maxLen: %d > %d", len(result), tt.maxLen)
			}
			if result != tt.expected {
				t.Errorf("truncateText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractFirstSentence(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "Long first sentence",
			text:     "First sentence is long enough. Second sentence.",
			expected: "First sentence is long enough.",
		},
		{
			name:     "Short first sentence - include second",
			text:     "Short! This is longer second sentence.",
			expected: "Short! This is longer second sentence.",
		},
		{
			name:     "Question with answer",
			text:     "Question? This is a detailed answer that follows.",
			expected: "Question? This is a detailed answer that follows.",
		},
		{
			name:     "User case: Идеально",
			text:     "Идеально! Все тесты исправлены! Создам краткий отчет.",
			expected: "Идеально! Все тесты исправлены!",
		},
		{
			name:     "Very long sentence - only first",
			text:     "This is a long first sentence that is already over twenty characters. Second sentence.",
			expected: "This is a long first sentence that is already over twenty characters.",
		},
		{
			name:     "No punctuation",
			text:     strings.Repeat("a", 150),
			expected: strings.Repeat("a", 100),
		},
		{
			name:     "Single short sentence",
			text:     "Done!",
			expected: "Done!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFirstSentence(tt.text)
			if result != tt.expected {
				t.Errorf("extractFirstSentence() = %q, want %q", result, tt.expected)
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

// === Tests for GenerateFromTranscript ===

func TestGenerateFromTranscript_TaskComplete(t *testing.T) {
	// Create temp transcript with task_complete scenario
	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	messages := buildTestTranscript([]string{"Write", "Edit", "Bash"}, "Created auth module", time.Now())
	writeTranscript(t, transcriptPath, messages)

	cfg := config.DefaultConfig()
	result := GenerateFromTranscript(transcriptPath, analyzer.StatusTaskComplete, cfg)

	// Should contain action summary
	if !strings.Contains(result, "Created") || !strings.Contains(result, "Edited") {
		t.Errorf("TaskComplete summary should mention actions, got: %s", result)
	}
}

func TestGenerateFromTranscript_Question(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	// Build transcript with AskUserQuestion
	now := time.Now()
	messages := []jsonl.Message{
		{
			Type:      "user",
			Timestamp: now.Add(-10 * time.Second).Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{{Type: "text", Text: "Help me"}},
			},
		},
		{
			Type:      "assistant",
			Timestamp: now.Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{
						Type: "tool_use",
						Name: "AskUserQuestion",
						Input: map[string]interface{}{
							"questions": []interface{}{
								map[string]interface{}{
									"question": "Which library should we use?",
								},
							},
						},
					},
				},
			},
		},
	}

	writeTranscript(t, transcriptPath, messages)

	cfg := config.DefaultConfig()
	result := GenerateFromTranscript(transcriptPath, analyzer.StatusQuestion, cfg)

	if !strings.Contains(result, "Which library") {
		t.Errorf("Question summary should contain question text, got: %s", result)
	}
}

func TestGenerateFromTranscript_PlanReady(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	now := time.Now()
	messages := []jsonl.Message{
		{
			Type:      "user",
			Timestamp: now.Add(-10 * time.Second).Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{{Type: "text", Text: "Create auth"}},
			},
		},
		{
			Type:      "assistant",
			Timestamp: now.Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{
						Type: "tool_use",
						Name: "ExitPlanMode",
						Input: map[string]interface{}{
							"plan": "1. Create user model\n2. Add authentication\n3. Test endpoints",
						},
					},
				},
			},
		},
	}

	writeTranscript(t, transcriptPath, messages)

	cfg := config.DefaultConfig()
	result := GenerateFromTranscript(transcriptPath, analyzer.StatusPlanReady, cfg)

	if !strings.Contains(result, "Create user model") {
		t.Errorf("Plan summary should contain plan text, got: %s", result)
	}
}

func TestGenerateFromTranscript_ReviewComplete(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	messages := buildTestTranscript([]string{"Read", "Read", "Grep"}, "Analyzed the codebase", time.Now())
	writeTranscript(t, transcriptPath, messages)

	cfg := config.DefaultConfig()
	result := GenerateFromTranscript(transcriptPath, analyzer.StatusReviewComplete, cfg)

	// Should contain either "Reviewed" or the extracted text
	if result == "" {
		t.Errorf("Review summary should not be empty")
	}
	// Just verify it's not empty and doesn't crash
	if len(result) < 5 {
		t.Errorf("Review summary too short: %s", result)
	}
}

func TestGenerateFromTranscript_NonexistentFile(t *testing.T) {
	cfg := config.DefaultConfig()
	result := GenerateFromTranscript("/nonexistent/path.jsonl", analyzer.StatusTaskComplete, cfg)

	// Should fallback to default message
	if !strings.Contains(result, "Task Completed") {
		t.Errorf("Should return default message for nonexistent file, got: %s", result)
	}
}

func TestGenerateFromTranscript_EmptyTranscript(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/empty.jsonl"

	// Create empty file
	writeTranscript(t, transcriptPath, []jsonl.Message{})

	cfg := config.DefaultConfig()
	result := GenerateFromTranscript(transcriptPath, analyzer.StatusTaskComplete, cfg)

	// Should fallback to default message
	if !strings.Contains(result, "Task Completed") {
		t.Errorf("Should return default message for empty transcript, got: %s", result)
	}
}

func TestGenerateFromTranscript_SessionLimitReached(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/session_limit.jsonl"

	// Create transcript with session limit message
	messages := []jsonl.Message{
		{
			Type:      "user",
			Timestamp: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{Type: "text", Text: "Continue working"},
				},
			},
		},
		{
			Type:      "assistant",
			Timestamp: time.Now().Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{
					{Type: "text", Text: "Session limit reached. Please start a new conversation."},
				},
			},
		},
	}

	writeTranscript(t, transcriptPath, messages)

	cfg := config.DefaultConfig()
	result := GenerateFromTranscript(transcriptPath, analyzer.StatusSessionLimitReached, cfg)

	expected := "Session limit reached. Please start a new conversation."
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// === Tests for GenerateSimple ===

func TestGenerateSimple(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		status   analyzer.Status
		expected string
	}{
		{analyzer.StatusTaskComplete, "Task Completed"},
		{analyzer.StatusQuestion, "Claude Has Questions"},
		{analyzer.StatusPlanReady, "Plan Ready"},
		{analyzer.StatusReviewComplete, "Review Complete"},
		{analyzer.StatusSessionLimitReached, "Session Limit Reached"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := GenerateSimple(tt.status, cfg)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("GenerateSimple(%s) = %s, want to contain %s", tt.status, result, tt.expected)
			}
		})
	}
}

// === Helper functions ===

func buildTestTranscript(tools []string, responseText string, timestamp time.Time) []jsonl.Message {
	var content []jsonl.Content

	// Add tools
	for _, tool := range tools {
		content = append(content, jsonl.Content{
			Type: "tool_use",
			Name: tool,
		})
	}

	// Add text
	content = append(content, jsonl.Content{
		Type: "text",
		Text: responseText,
	})

	return []jsonl.Message{
		{
			Type:      "user",
			Timestamp: timestamp.Add(-10 * time.Second).Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: []jsonl.Content{{Type: "text", Text: "User request"}},
			},
		},
		{
			Type:      "assistant",
			Timestamp: timestamp.Format(time.RFC3339),
			Message: jsonl.MessageContent{
				Content: content,
			},
		},
	}
}

func writeTranscript(t *testing.T, path string, messages []jsonl.Message) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create transcript: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			t.Fatalf("failed to encode message: %v", err)
		}
	}
}
