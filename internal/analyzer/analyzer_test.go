package analyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetStatusForPreToolUse(t *testing.T) {
	tests := []struct {
		toolName string
		expected Status
	}{
		{"ExitPlanMode", StatusPlanReady},
		{"AskUserQuestion", StatusQuestion},
		{"Write", StatusUnknown},
		{"", StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			status := GetStatusForPreToolUse(tt.toolName)
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	assert.True(t, contains(slice, "banana"))
	assert.False(t, contains(slice, "orange"))
	assert.False(t, contains([]string{}, "test"))
}
