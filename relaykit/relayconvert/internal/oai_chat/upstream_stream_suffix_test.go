package oaichat

import (
	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaudeStreamKeepsAnswerSharingReasoningChunk(t *testing.T) {
	for _, first := range []bool{true, false} {
		t.Run(map[bool]string{true: "first", false: "later"}[first], func(t *testing.T) {
			info := &convmeta.Values{SendResponseCount: 1}
			if !first {
				var initial dto.ChatCompletionsStreamResponse
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"choices":[{"delta":{"reasoning_content":"before"}}]}`, &initial))
				StreamResponseOpenAI2Claude(&initial, info)
				info.IncrSendResponseCount()
			}
			var chunk dto.ChatCompletionsStreamResponse
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"choices":[{"delta":{"reasoning_content":"last thought","content":"first answer"}}]}`, &chunk))
			events := StreamResponseOpenAI2Claude(&chunk, info)
			var kinds []string
			var text, thought string
			for _, event := range events {
				if event.Type == "message_start" {
					continue
				}
				kind := event.Type
				if event.ContentBlock != nil {
					kind += ":" + event.ContentBlock.Type
				}
				if event.Delta != nil {
					kind += ":" + event.Delta.Type
					if event.Delta.Text != nil {
						text += *event.Delta.Text
					}
					if event.Delta.Thinking != nil {
						thought += *event.Delta.Thinking
					}
				}
				kinds = append(kinds, kind)
			}
			assert.Equal(t, "first answer", text)
			assert.Equal(t, "last thought", thought)
			expected := []string{"content_block_delta:thinking_delta", "content_block_stop", "content_block_start:text", "content_block_delta:text_delta"}
			if first {
				expected = append([]string{"content_block_start:thinking"}, expected...)
			}
			assert.Equal(t, expected, kinds)
			assert.Equal(t, convmeta.LastMessageTypeText, info.ClaudeConvertInfo.LastMessagesType)
		})
	}
}

func TestClaudeEffortSuffixHonorsDisabledAdapter(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		info := &convmeta.Values{Options: &convmeta.Options{Claude: convmeta.ClaudeOptions{ThinkingAdapterEnabled: enabled}}}
		req := dto.GeneralOpenAIRequest{Model: "claude-opus-4-6-high", MaxTokens: kitutil.GetPointer[uint](4096), Messages: []dto.Message{{Role: "user", Content: "hi"}}}
		out, err := OpenAIChatRequestToClaudeMessages(context.Background(), info, req)
		require.NoError(t, err)
		if enabled {
			assert.Equal(t, "claude-opus-4-6", out.Model)
			require.NotNil(t, out.Thinking)
			assert.JSONEq(t, `{"effort":"high"}`, string(out.OutputConfig))
		} else {
			assert.Equal(t, req.Model, out.Model)
			assert.Nil(t, out.Thinking)
			assert.Empty(t, out.OutputConfig)
		}
	}
}
