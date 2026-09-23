package relayconvert

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRequestClaudeToChatToolResultNames(t *testing.T) {
	req := &dto.ClaudeRequest{
		Model: "claude-test",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: []dto.ClaudeMediaMessage{
				{Type: "tool_result", ToolUseId: "call_1", Content: "before call"},
			}},
			{Role: "assistant", Content: []dto.ClaudeMediaMessage{
				{Type: "tool_use", Id: "call_1", Name: "first", Input: map[string]any{}},
			}},
			{Role: "user", Content: []dto.ClaudeMediaMessage{
				{Type: "tool_result", ToolUseId: "call_1", Content: "after call"},
				{Type: "tool_result", ToolUseId: "missing", Content: "unknown"},
				{Type: "tool_result", ToolUseId: "call_1", Name: "explicit", Content: "named"},
				{Type: "tool_result", Content: "missing ID"},
			}},
			{Role: "assistant", Content: []dto.ClaudeMediaMessage{
				{Type: "tool_use", Id: "call_1", Name: "later", Input: map[string]any{}},
			}},
		},
	}

	result, err := ConvertRequestByID(nil, nil, ConverterClaudeMessagesToOpenAIChat, req)
	require.NoError(t, err)
	chatReq, ok := result.Value.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chatReq.Messages, 7)
	for _, tc := range []struct {
		index   int
		id      string
		name    string
		content string
	}{
		{0, "call_1", "first", "before call"},
		{2, "call_1", "first", "after call"},
		{3, "missing", "", "unknown"},
		{4, "call_1", "explicit", "named"},
		{5, "", "", "missing ID"},
	} {
		message := chatReq.Messages[tc.index]
		assert.Equal(t, "tool", message.Role)
		assert.Equal(t, tc.id, message.ToolCallId)
		require.NotNil(t, message.Name)
		assert.Equal(t, tc.name, *message.Name)
		assert.Equal(t, tc.content, message.StringContent())
	}
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	assert.Equal(t, "assistant", chatReq.Messages[6].Role)
}
