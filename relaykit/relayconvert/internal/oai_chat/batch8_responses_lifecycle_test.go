package oaichat

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch8ResponsesTextAndToolLifecyclePayloads(t *testing.T) {
	state := NewChatToResponsesStreamState("resp1", "model")
	content := "answer"
	first, err := ChatCompletionsStreamChunkToResponsesEvents(&dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &content}}}}, state)
	require.NoError(t, err)
	var kinds []string
	for _, e := range first {
		kinds = append(kinds, e.Type)
	}
	assert.Equal(t, []string{"response.created", "response.output_item.added", "response.content_part.added", "response.output_text.delta"}, kinds)
	idx := 0
	_, err = ChatCompletionsStreamChunkToResponsesEvents(&dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{Index: &idx, ID: "call1", Type: "function", Function: dto.FunctionResponse{Name: "lookup", Arguments: `{"q":"x"}`}}}}}}}, state)
	require.NoError(t, err)
	final := FinalizeChatCompletionsStreamToResponses(state)
	var doneText, doneArguments, donePart map[string]any
	for _, e := range final {
		raw, err := kitutil.Marshal(e.Payload)
		require.NoError(t, err)
		var event map[string]any
		require.NoError(t, kitutil.Unmarshal(raw, &event))
		switch e.Type {
		case "response.output_text.done":
			doneText = event
		case "response.function_call_arguments.done":
			doneArguments = event
		case "response.content_part.done":
			donePart = event
		}
	}
	require.NotNil(t, donePart)
	assert.Equal(t, "answer", donePart["part"].(map[string]any)["text"])
	assert.Equal(t, "answer", doneText["text"])
	assert.Equal(t, `{"q":"x"}`, doneArguments["arguments"])
	assert.Equal(t, "lookup", doneArguments["name"])
}
