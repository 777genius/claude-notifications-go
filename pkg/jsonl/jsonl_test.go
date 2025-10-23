package jsonl

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	jsonl := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","name":"Write"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read"}]}}`

	messages, err := Parse(strings.NewReader(jsonl))
	require.NoError(t, err)
	assert.Len(t, messages, 3)

	assert.Equal(t, "user", messages[0].Type)
	assert.Equal(t, "assistant", messages[1].Type)
	assert.Equal(t, "assistant", messages[2].Type)
}

func TestParseInvalidLines(t *testing.T) {
	jsonl := `{"type":"user"}
invalid json line
{"type":"assistant"}`

	messages, err := Parse(strings.NewReader(jsonl))
	require.NoError(t, err)
	// Should skip invalid line
	assert.Len(t, messages, 2)
}

func TestGetLastAssistantMessages(t *testing.T) {
	messages := []Message{
		{Type: "user"},
		{Type: "assistant"},
		{Type: "user"},
		{Type: "assistant"},
		{Type: "assistant"},
	}

	last := GetLastAssistantMessages(messages, 2)
	assert.Len(t, last, 2)
	assert.Equal(t, "assistant", last[0].Type)
	assert.Equal(t, "assistant", last[1].Type)

	// Request more than available
	last = GetLastAssistantMessages(messages, 10)
	assert.Len(t, last, 3)
}

func TestExtractTools(t *testing.T) {
	messages := []Message{
		{
			Message: MessageContent{
				Content: []Content{
					{Type: "text", Text: "hello"},
					{Type: "tool_use", Name: "Write"},
				},
			},
		},
		{
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Read"},
					{Type: "tool_use", Name: "Edit"},
				},
			},
		},
	}

	tools := ExtractTools(messages)
	assert.Len(t, tools, 3)
	assert.Equal(t, "Write", tools[0].Name)
	assert.Equal(t, 0, tools[0].Position)
	assert.Equal(t, "Read", tools[1].Name)
	assert.Equal(t, 1, tools[1].Position)
	assert.Equal(t, "Edit", tools[2].Name)
	assert.Equal(t, 1, tools[2].Position)
}

func TestGetLastTool(t *testing.T) {
	tools := []ToolUse{
		{Position: 0, Name: "Write"},
		{Position: 1, Name: "Read"},
	}

	lastTool := GetLastTool(tools)
	assert.Equal(t, "Read", lastTool)

	// Empty tools
	lastTool = GetLastTool([]ToolUse{})
	assert.Equal(t, "", lastTool)
}

func TestFindToolPosition(t *testing.T) {
	tools := []ToolUse{
		{Position: 0, Name: "Write"},
		{Position: 1, Name: "Read"},
		{Position: 2, Name: "Write"},
	}

	// Should return last occurrence
	pos := FindToolPosition(tools, "Write")
	assert.Equal(t, 2, pos)

	pos = FindToolPosition(tools, "Read")
	assert.Equal(t, 1, pos)

	pos = FindToolPosition(tools, "NonExistent")
	assert.Equal(t, -1, pos)
}

func TestCountToolsAfterPosition(t *testing.T) {
	tools := []ToolUse{
		{Position: 0, Name: "Write"},
		{Position: 1, Name: "Read"},
		{Position: 2, Name: "Edit"},
		{Position: 3, Name: "Bash"},
	}

	count := CountToolsAfterPosition(tools, 1)
	assert.Equal(t, 2, count)

	count = CountToolsAfterPosition(tools, 5)
	assert.Equal(t, 0, count)
}

func TestExtractTextFromMessages(t *testing.T) {
	messages := []Message{
		{
			Message: MessageContent{
				Content: []Content{
					{Type: "text", Text: "hello"},
					{Type: "tool_use", Name: "Write"},
				},
			},
		},
		{
			Message: MessageContent{
				Content: []Content{
					{Type: "text", Text: "world"},
				},
			},
		},
	}

	texts := ExtractTextFromMessages(messages)
	assert.Len(t, texts, 2)
	assert.Equal(t, "hello", texts[0])
	assert.Equal(t, "world", texts[1])
}

func TestFilterMessagesAfterTimestamp(t *testing.T) {
	messages := []Message{
		{Type: "user", Timestamp: "2025-01-01T10:00:00Z"},
		{Type: "assistant", Timestamp: "2025-01-01T10:01:00Z"}, // Before last user
		{Type: "user", Timestamp: "2025-01-01T10:05:00Z"},      // Last user message
		{Type: "assistant", Timestamp: "2025-01-01T10:06:00Z"}, // After - should include
		{Type: "assistant", Timestamp: "2025-01-01T10:07:00Z"}, // After - should include
	}

	filtered := FilterMessagesAfterTimestamp(messages, "2025-01-01T10:05:00Z")

	assert.Len(t, filtered, 2) // Only 2 messages after last user
	assert.Equal(t, "2025-01-01T10:06:00Z", filtered[0].Timestamp)
	assert.Equal(t, "2025-01-01T10:07:00Z", filtered[1].Timestamp)
}

func TestFilterMessagesAfterTimestamp_NoUserMessage(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Timestamp: "2025-01-01T10:01:00Z"},
		{Type: "assistant", Timestamp: "2025-01-01T10:02:00Z"},
	}

	// Empty timestamp should return all assistant messages
	filtered := FilterMessagesAfterTimestamp(messages, "")

	assert.Len(t, filtered, 2)
}

func TestFilterMessagesAfterTimestamp_InvalidTimestamp(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Timestamp: "2025-01-01T10:01:00Z"},
		{Type: "assistant", Timestamp: "2025-01-01T10:02:00Z"},
	}

	// Invalid timestamp should return all assistant messages
	filtered := FilterMessagesAfterTimestamp(messages, "invalid")

	assert.Len(t, filtered, 2)
}

// === Tests for review_complete helper functions ===

func TestCountToolsByNames(t *testing.T) {
	tests := []struct {
		name      string
		tools     []ToolUse
		names     []string
		wantCount int
	}{
		{
			name:      "single_match",
			tools:     []ToolUse{{Name: "Read"}, {Name: "Write"}},
			names:     []string{"Read"},
			wantCount: 1,
		},
		{
			name:      "multiple_matches_same_tool",
			tools:     []ToolUse{{Name: "Read"}, {Name: "Read"}, {Name: "Write"}},
			names:     []string{"Read"},
			wantCount: 2,
		},
		{
			name:      "multiple_matches_different_tools",
			tools:     []ToolUse{{Name: "Read"}, {Name: "Read"}, {Name: "Grep"}},
			names:     []string{"Read", "Grep"},
			wantCount: 3,
		},
		{
			name:      "mixed_read_like_tools",
			tools:     []ToolUse{{Name: "Read"}, {Name: "Grep"}, {Name: "Glob"}, {Name: "Read"}},
			names:     []string{"Read", "Grep", "Glob"},
			wantCount: 4,
		},
		{
			name:      "no_matches",
			tools:     []ToolUse{{Name: "Write"}, {Name: "Edit"}},
			names:     []string{"Read", "Grep"},
			wantCount: 0,
		},
		{
			name:      "empty_tools",
			tools:     []ToolUse{},
			names:     []string{"Read"},
			wantCount: 0,
		},
		{
			name:      "empty_names",
			tools:     []ToolUse{{Name: "Read"}},
			names:     []string{},
			wantCount: 0,
		},
		{
			name:      "partial_match",
			tools:     []ToolUse{{Name: "Read"}, {Name: "Write"}, {Name: "Grep"}},
			names:     []string{"Read", "Glob"},
			wantCount: 1, // Only Read matches
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountToolsByNames(tt.tools, tt.names)
			assert.Equal(t, tt.wantCount, got, "CountToolsByNames(%v, %v)", tt.tools, tt.names)
		})
	}
}

func TestHasAnyActiveTool(t *testing.T) {
	activeTools := []string{"Write", "Edit", "Bash", "NotebookEdit", "SlashCommand", "KillShell"}

	tests := []struct {
		name  string
		tools []ToolUse
		want  bool
	}{
		{
			name:  "has_single_active_tool",
			tools: []ToolUse{{Name: "Read"}, {Name: "Write"}},
			want:  true,
		},
		{
			name:  "has_multiple_active_tools",
			tools: []ToolUse{{Name: "Read"}, {Name: "Write"}, {Name: "Edit"}},
			want:  true,
		},
		{
			name:  "only_passive_tools",
			tools: []ToolUse{{Name: "Read"}, {Name: "Grep"}, {Name: "Glob"}},
			want:  false,
		},
		{
			name:  "active_tool_first",
			tools: []ToolUse{{Name: "Write"}, {Name: "Read"}},
			want:  true,
		},
		{
			name:  "active_tool_last",
			tools: []ToolUse{{Name: "Read"}, {Name: "Bash"}},
			want:  true,
		},
		{
			name:  "notebook_edit_is_active",
			tools: []ToolUse{{Name: "Read"}, {Name: "NotebookEdit"}},
			want:  true,
		},
		{
			name:  "slash_command_is_active",
			tools: []ToolUse{{Name: "Read"}, {Name: "SlashCommand"}},
			want:  true,
		},
		{
			name:  "kill_shell_is_active",
			tools: []ToolUse{{Name: "KillShell"}},
			want:  true,
		},
		{
			name:  "empty_tools",
			tools: []ToolUse{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasAnyActiveTool(tt.tools, activeTools)
			assert.Equal(t, tt.want, got, "HasAnyActiveTool(%v)", tt.tools)
		})
	}
}

func TestExtractRecentText(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Message: MessageContent{
			Content: []Content{{Type: "text", Text: "First message"}},
		}},
		{Type: "assistant", Message: MessageContent{
			Content: []Content{{Type: "text", Text: "Second message"}},
		}},
		{Type: "assistant", Message: MessageContent{
			Content: []Content{{Type: "text", Text: "Third message"}},
		}},
		{Type: "assistant", Message: MessageContent{
			Content: []Content{
				{Type: "text", Text: "Fourth message"},
				{Type: "text", Text: "with multiple texts"},
			},
		}},
	}

	tests := []struct {
		name  string
		count int
		want  string
	}{
		{
			name:  "last_one",
			count: 1,
			want:  "Fourth message with multiple texts",
		},
		{
			name:  "last_two",
			count: 2,
			want:  "Third message Fourth message with multiple texts",
		},
		{
			name:  "last_three",
			count: 3,
			want:  "Second message Third message Fourth message with multiple texts",
		},
		{
			name:  "all_four",
			count: 4,
			want:  "First message Second message Third message Fourth message with multiple texts",
		},
		{
			name:  "more_than_available",
			count: 10,
			want:  "First message Second message Third message Fourth message with multiple texts",
		},
		{
			name:  "zero",
			count: 0,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRecentText(messages, tt.count)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractRecentText_EmptyMessages(t *testing.T) {
	messages := []Message{}
	result := ExtractRecentText(messages, 5)
	assert.Equal(t, "", result)
}

func TestExtractRecentText_NoTextContent(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Message: MessageContent{
			Content: []Content{{Type: "tool_use", Name: "Read"}},
		}},
	}
	result := ExtractRecentText(messages, 1)
	assert.Equal(t, "", result)
}

func TestExtractRecentText_MixedContent(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Message: MessageContent{
			Content: []Content{
				{Type: "tool_use", Name: "Read"},
				{Type: "text", Text: "Analysis of file"},
				{Type: "tool_use", Name: "Grep"},
				{Type: "text", Text: "Found issues"},
			},
		}},
	}
	result := ExtractRecentText(messages, 1)
	assert.Equal(t, "Analysis of file Found issues", result)
}

// === Tests for ParseFile ===

func TestParseFile_Success(t *testing.T) {
	// Create temp JSONL file
	tmpFile, err := os.CreateTemp("", "test-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write JSONL data
	jsonlData := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"timestamp":"2025-01-01T10:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"timestamp":"2025-01-01T10:00:01Z"}`

	_, err = tmpFile.WriteString(jsonlData)
	require.NoError(t, err)
	tmpFile.Close()

	// Parse file
	messages, err := ParseFile(tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "user", messages[0].Type)
	assert.Equal(t, "assistant", messages[1].Type)
}

func TestParseFile_NonexistentFile(t *testing.T) {
	messages, err := ParseFile("/nonexistent/file.jsonl")

	assert.Error(t, err)
	assert.Nil(t, messages)
}

func TestParseFile_MalformedJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write invalid JSON
	_, err = tmpFile.WriteString(`{"type":"user"}
invalid json line
{"type":"assistant"}`)
	require.NoError(t, err)
	tmpFile.Close()

	messages, err := ParseFile(tmpFile.Name())

	// Should skip invalid lines, no error
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestParseFile_LargeFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-large-*.jsonl")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write 1000 lines
	for i := 0; i < 1000; i++ {
		line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"line"}]}}` + "\n"
		_, err = tmpFile.WriteString(line)
		require.NoError(t, err)
	}
	tmpFile.Close()

	messages, err := ParseFile(tmpFile.Name())

	require.NoError(t, err)
	assert.Len(t, messages, 1000)
}

// === Tests for FindLastToolUse ===

func TestFindLastToolUse_Found(t *testing.T) {
	messages := []Message{
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Write", Input: map[string]interface{}{"file": "test.go"}},
				},
			},
		},
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Read", Input: map[string]interface{}{"file": "main.go"}},
				},
			},
		},
	}

	tool := FindLastToolUse(messages, "Write")
	require.NotNil(t, tool)
	assert.Equal(t, "Write", tool.Name)
	assert.Equal(t, "test.go", tool.Input["file"])
}

func TestFindLastToolUse_NotFound(t *testing.T) {
	messages := []Message{
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Write"},
				},
			},
		},
	}

	tool := FindLastToolUse(messages, "NonExistent")
	assert.Nil(t, tool)
}

func TestFindLastToolUse_MultipleOccurrences(t *testing.T) {
	messages := []Message{
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Read", Input: map[string]interface{}{"file": "first.go"}},
				},
			},
		},
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Read", Input: map[string]interface{}{"file": "second.go"}},
				},
			},
		},
	}

	tool := FindLastToolUse(messages, "Read")
	require.NotNil(t, tool)
	// Should return LAST occurrence
	assert.Equal(t, "second.go", tool.Input["file"])
}

// === Tests for ExtractToolInput ===

func TestExtractToolInput_Found(t *testing.T) {
	messages := []Message{
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{
						Type: "tool_use",
						Name: "AskUserQuestion",
						Input: map[string]interface{}{
							"questions": []interface{}{
								map[string]interface{}{"question": "Test?"},
							},
						},
					},
				},
			},
		},
	}

	input := ExtractToolInput(messages, "AskUserQuestion")
	assert.NotEmpty(t, input)
	assert.Contains(t, input, "questions")
}

func TestExtractToolInput_NotFound(t *testing.T) {
	messages := []Message{
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Write"},
				},
			},
		},
	}

	input := ExtractToolInput(messages, "NonExistent")
	assert.Empty(t, input)
}

func TestExtractToolInput_NoInput(t *testing.T) {
	messages := []Message{
		{
			Type: "assistant",
			Message: MessageContent{
				Content: []Content{
					{Type: "tool_use", Name: "Read"}, // No Input field
				},
			},
		},
	}

	input := ExtractToolInput(messages, "Read")
	// Function returns the Input field directly, which may be nil
	// Just verify it doesn't crash
	_ = input
}

// === Tests for GetLastUserTimestamp ===

func TestGetLastUserTimestamp_Found(t *testing.T) {
	messages := []Message{
		{Type: "user", Timestamp: "2025-01-01T10:00:00Z", Message: MessageContent{
			Content: []Content{{Type: "text", Text: "First"}},
		}},
		{Type: "assistant", Timestamp: "2025-01-01T10:00:01Z"},
		{Type: "user", Timestamp: "2025-01-01T10:00:05Z", Message: MessageContent{
			Content: []Content{{Type: "text", Text: "Second"}},
		}},
	}

	timestamp := GetLastUserTimestamp(messages)
	assert.Equal(t, "2025-01-01T10:00:05Z", timestamp)
}

func TestGetLastUserTimestamp_NoUserMessages(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Timestamp: "2025-01-01T10:00:01Z"},
		{Type: "assistant", Timestamp: "2025-01-01T10:00:02Z"},
	}

	timestamp := GetLastUserTimestamp(messages)
	assert.Equal(t, "", timestamp)
}

func TestGetLastUserTimestamp_OnlyToolResults(t *testing.T) {
	messages := []Message{
		{Type: "user", Timestamp: "2025-01-01T10:00:00Z", Message: MessageContent{
			Content: []Content{{Type: "tool_result"}}, // Not text type
		}},
	}

	timestamp := GetLastUserTimestamp(messages)
	assert.Equal(t, "", timestamp)
}

func TestGetLastUserTimestamp_StringContent(t *testing.T) {
	// Test that string content (normal user text messages) is properly detected
	messages := []Message{
		{Type: "user", Timestamp: "2025-01-01T10:00:00Z", Message: MessageContent{
			ContentString: "First message",
		}},
		{Type: "assistant", Timestamp: "2025-01-01T10:00:01Z"},
		{Type: "user", Timestamp: "2025-01-01T10:00:05Z", Message: MessageContent{
			ContentString: "Second message",
		}},
	}

	timestamp := GetLastUserTimestamp(messages)
	assert.Equal(t, "2025-01-01T10:00:05Z", timestamp)
}

func TestMessageContent_UnmarshalJSON_StringContent(t *testing.T) {
	// Test parsing of user message with string content (normal text)
	jsonStr := `{
		"type": "user",
		"message": {
			"role": "user",
			"content": "Hello, this is a test message"
		},
		"timestamp": "2025-01-01T10:00:00Z"
	}`

	var msg Message
	err := json.Unmarshal([]byte(jsonStr), &msg)
	assert.NoError(t, err)
	assert.Equal(t, "user", msg.Type)
	assert.Equal(t, "user", msg.Message.Role)
	assert.Equal(t, "Hello, this is a test message", msg.Message.ContentString)
	assert.Equal(t, 0, len(msg.Message.Content))
}

func TestMessageContent_UnmarshalJSON_ArrayContent(t *testing.T) {
	// Test parsing of user message with array content (tool_result)
	jsonStr := `{
		"type": "user",
		"message": {
			"role": "user",
			"content": [
				{
					"type": "tool_result",
					"tool_use_id": "toolu_123",
					"content": "No files found"
				}
			]
		},
		"timestamp": "2025-01-01T10:00:00Z"
	}`

	var msg Message
	err := json.Unmarshal([]byte(jsonStr), &msg)
	assert.NoError(t, err)
	assert.Equal(t, "user", msg.Type)
	assert.Equal(t, "user", msg.Message.Role)
	assert.Equal(t, "", msg.Message.ContentString)
	assert.Equal(t, 1, len(msg.Message.Content))
	assert.Equal(t, "tool_result", msg.Message.Content[0].Type)
}

func TestMessageContent_UnmarshalJSON_ArrayTextContent(t *testing.T) {
	// Test parsing of user message with array content type="text" (interrupted tool use)
	jsonStr := `{
		"type": "user",
		"message": {
			"role": "user",
			"content": [
				{
					"type": "text",
					"text": "[Request interrupted by user for tool use]"
				}
			]
		},
		"timestamp": "2025-01-01T10:00:00Z"
	}`

	var msg Message
	err := json.Unmarshal([]byte(jsonStr), &msg)
	assert.NoError(t, err)
	assert.Equal(t, "user", msg.Type)
	assert.Equal(t, "", msg.Message.ContentString)
	assert.Equal(t, 1, len(msg.Message.Content))
	assert.Equal(t, "text", msg.Message.Content[0].Type)
	assert.Equal(t, "[Request interrupted by user for tool use]", msg.Message.Content[0].Text)
}

// === Tests for GetLastAssistantTimestamp ===

func TestGetLastAssistantTimestamp_Found(t *testing.T) {
	messages := []Message{
		{Type: "assistant", Timestamp: "2025-01-01T10:00:01Z"},
		{Type: "user", Timestamp: "2025-01-01T10:00:02Z"},
		{Type: "assistant", Timestamp: "2025-01-01T10:00:03Z"},
	}

	timestamp := GetLastAssistantTimestamp(messages)
	assert.Equal(t, "2025-01-01T10:00:03Z", timestamp)
}

func TestGetLastAssistantTimestamp_NoAssistantMessages(t *testing.T) {
	messages := []Message{
		{Type: "user", Timestamp: "2025-01-01T10:00:00Z"},
		{Type: "user", Timestamp: "2025-01-01T10:00:01Z"},
	}

	timestamp := GetLastAssistantTimestamp(messages)
	assert.Equal(t, "", timestamp)
}

func TestGetLastAssistantTimestamp_EmptyMessages(t *testing.T) {
	messages := []Message{}

	timestamp := GetLastAssistantTimestamp(messages)
	assert.Equal(t, "", timestamp)
}
