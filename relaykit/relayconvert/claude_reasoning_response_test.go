package relayconvert_test

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeResponseConversionPreservesReasoning(t *testing.T) {
	reasoning := "consider the input"
	input := &dto.OpenAITextResponse{
		Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{
			Role: "assistant", Content: "answer", ReasoningContent: &reasoning,
		}, FinishReason: "stop"}},
		Usage: dto.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
	}
	result, err := relayconvert.ConvertResponse(context.Background(), nil, types.RelayFormatClaude, input)
	require.NoError(t, err)
	out, ok := result.Value.(*dto.ClaudeResponse)
	require.True(t, ok)
	require.Len(t, out.Content, 2)
	assert.Equal(t, "thinking", out.Content[0].Type)
	require.NotNil(t, out.Content[0].Thinking)
	assert.Equal(t, reasoning, *out.Content[0].Thinking)
	assert.Empty(t, out.Content[0].Signature)
	assert.Equal(t, "text", out.Content[1].Type)
	assert.Equal(t, "answer", out.Content[1].GetText())
	assert.Equal(t, 10, out.Usage.InputTokens)
	assert.Equal(t, 2, out.Usage.OutputTokens)
	assert.Equal(t, 12, result.Usage.TotalTokens)
}
