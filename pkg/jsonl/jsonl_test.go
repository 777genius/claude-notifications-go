package jsonl

import (
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
