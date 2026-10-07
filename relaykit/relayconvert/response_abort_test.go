package relayconvert

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResponsesAbortIsOneFailedTerminalWithoutSuccessfulFinalization(t *testing.T) {
	for _, from := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		t.Run(string(from), func(t *testing.T) {
			state, err := NewResponseStreamState(from, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "resp_public", Model: "public-model"})
			require.NoError(t, err)
			text := "partial"
			var chunk any = &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &text}}}}
			if from == types.RelayFormatClaude {
				chunk = &dto.ClaudeResponse{Type: "content_block_delta", Delta: &dto.ClaudeMediaMessage{Type: "text_delta", Text: &text}}
			}
			_, err = ConvertStreamResponseChunk(nil, nil, state, chunk)
			require.NoError(t, err)
			events, err := AbortStreamResponse(state)
			require.NoError(t, err)
			require.Len(t, events, 1)
			event := events[0].Value.(ChatToResponsesStreamEvent)
			require.Equal(t, "response.failed", event.Type)
			require.Equal(t, "resp_public", event.Payload.Response.ID)
			require.Equal(t, "public-model", event.Payload.Response.Model)
			require.JSONEq(t, `"failed"`, string(event.Payload.Response.Status))
			require.Empty(t, event.Payload.Response.Output)
			again, err := AbortStreamResponse(state)
			require.NoError(t, err)
			require.Empty(t, again)
			final, err := FinalizeStreamResponse(nil, nil, state)
			require.NoError(t, err)
			require.Empty(t, final)
		})
	}
}

func TestResponsesAbortAfterSuccessfulFinalizeDoesNotAddFailure(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_public", "public-model")
	final := FinalizeChatCompletionsStreamToResponses(state)
	require.Len(t, final, 1)
	require.Equal(t, "response.completed", final[0].Type)
	require.Empty(t, AbortChatCompletionsStreamToResponses(state))
}
