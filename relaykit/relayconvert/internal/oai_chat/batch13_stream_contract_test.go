package oaichat_test

import (
	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func newB13State(t *testing.T) *relayconvert.ResponseStreamState {
	t.Helper()
	s, e := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{ID: "resp", Model: "public"})
	require.NoError(t, e)
	return s
}
func convertB13(t *testing.T, s *relayconvert.ResponseStreamState, raw string) ([]relayconvert.ResponseResult, error) {
	t.Helper()
	var chunk dto.ChatCompletionsStreamResponse
	require.NoError(t, kitutil.Unmarshal([]byte(raw), &chunk))
	return relayconvert.ConvertStreamResponseChunk(context.Background(), nil, s, &chunk)
}
func TestB13FinishProtectsClosedItemsAndAllowsUsageTail(t *testing.T) {
	for _, delta := range []string{`{"content":"private"}`, `{"reasoning_content":"private"}`, `{"tool_calls":[{"index":0,"function":{"arguments":"private"}}]}`} {
		s := newB13State(t)
		events, e := convertB13(t, s, `{"choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":"tool_calls"}]}`)
		require.NoError(t, e)
		kinds := []string{}
		for _, r := range events {
			event := r.Value.(relayconvert.ChatToResponsesStreamEvent)
			kinds = append(kinds, event.Type)
			if event.Type == "response.reasoning_summary_text.done" {
				require.NotNil(t, event.Payload.Text)
				assert.Equal(t, "think", *event.Payload.Text)
				assert.Nil(t, event.Payload.Part)
			}
		}
		assert.Contains(t, kinds, "response.reasoning_summary_part.added")
		assert.Contains(t, kinds, "response.reasoning_summary_part.done")
		_, e = convertB13(t, s, `{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"reasoning_tokens":8,"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":2}}}`)
		require.NoError(t, e)
		_, e = convertB13(t, s, `{"choices":[{"index":0,"delta":`+delta+`}]}`)
		require.Error(t, e)
		assert.NotContains(t, e.Error(), "private")
		final, e := relayconvert.FinalizeStreamResponse(context.Background(), nil, s)
		require.NoError(t, e)
		for _, r := range final {
			assert.NotEqual(t, "response.completed", r.Value.(relayconvert.ChatToResponsesStreamEvent).Type)
		}
	}
	s := newB13State(t)
	_, e := convertB13(t, s, `{"choices":[{"index":0,"delta":{"content":"first"},"finish_reason":"stop"},{"index":0,"delta":{"content":"private"}}]}`)
	require.Error(t, e)
}
func TestB13NamedToolPublicationPreservesArgumentsAndIndexes(t *testing.T) {
	s := newB13State(t)
	events, e := convertB13(t, s, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":9,"function":{"name":"\u2003","arguments":"ignored"}},{"index":1,"function":{"arguments":"{\"q\":"}}]}}]}`)
	require.NoError(t, e)
	for _, r := range events {
		assert.NotEqual(t, "response.output_item.added", r.Value.(relayconvert.ChatToResponsesStreamEvent).Type)
	}
	events, e = convertB13(t, s, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_ok","function":{"name":"lookup","arguments":"1}"}}]}}]}`)
	require.NoError(t, e)
	require.Len(t, events, 2)
	added := events[0].Value.(relayconvert.ChatToResponsesStreamEvent)
	assert.Equal(t, "lookup", added.Payload.Item.Name)
	assert.Equal(t, 0, *added.Payload.OutputIndex)
	assert.Equal(t, `{"q":1}`, events[1].Value.(relayconvert.ChatToResponsesStreamEvent).Payload.Delta)
	_, e = convertB13(t, s, `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
	require.NoError(t, e)
	final, e := relayconvert.FinalizeStreamResponse(context.Background(), nil, s)
	require.NoError(t, e)
	response := final[len(final)-1].Value.(relayconvert.ChatToResponsesStreamEvent).Payload.Response
	require.Len(t, response.Output, 1)
	assert.Equal(t, "call_ok", response.Output[0].CallId)
	var args string
	require.NoError(t, kitutil.Unmarshal(response.Output[0].Arguments, &args))
	assert.Equal(t, `{"q":1}`, args)
	input, err := kitutil.Marshal(response.Output)
	require.NoError(t, err)
	replayed, err := relayconvert.ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &dto.OpenAIResponsesRequest{Model: "public", Input: input})
	require.NoError(t, err)
	request := replayed.Value.(*dto.GeneralOpenAIRequest)
	require.Len(t, request.Messages, 1)
	calls := request.Messages[0].ParseToolCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "call_ok", calls[0].ID)
	assert.Equal(t, "lookup", calls[0].Function.Name)
	assert.Equal(t, args, calls[0].Function.Arguments)
	for _, identity := range []string{`"id":"private-change"`, `"function":{"name":"private-change"}`} {
		s := newB13State(t)
		_, e := convertB13(t, s, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"ok","function":{"name":"lookup","arguments":"{}"}}]}}]}`)
		require.NoError(t, e)
		_, e = convertB13(t, s, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,`+identity+`}]}}]}`)
		require.Error(t, e)
		assert.NotContains(t, e.Error(), "private")
	}
}
func TestB13NonstreamBlankToolsAndCompleteOutputDetails(t *testing.T) {
	var chat dto.OpenAITextResponse
	require.NoError(t, kitutil.Unmarshal([]byte(`{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"\u2003"}},{"type":"function","function":{"name":"lookup","arguments":"{}"}},{"type":"custom","custom":{"input":"custom"}}]}}],"usage":{"prompt_tokens":0,"completion_tokens":0,"reasoning_tokens":7,"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":2}}}`), &chat))
	result, e := relayconvert.ConvertResponse(context.Background(), nil, types.RelayFormatOpenAIResponses, &chat)
	require.NoError(t, e)
	response := result.Value.(*dto.OpenAIResponsesResponse)
	require.Len(t, response.Output, 2)
	assert.Equal(t, "lookup", response.Output[0].Name)
	assert.Equal(t, "custom", response.Output[1].Type)
	require.NotNil(t, response.Usage.OutputTokensDetails)
	assert.Zero(t, response.Usage.OutputTokensDetails.ReasoningTokens)
	assert.Equal(t, 2, response.Usage.OutputTokensDetails.AudioTokens)
}
func TestB13MultipleChoicesAreExplicitErrors(t *testing.T) {
	s := newB13State(t)
	_, e := convertB13(t, s, `{"choices":[{"index":0,"delta":{"content":"first"}},{"index":1,"delta":{"content":"second"}}]}`)
	require.Error(t, e)
}

func TestB13LateIdentityWaitsAndMissingIDFallsBackOnlyAtFinish(t *testing.T) {
	for _, late := range []bool{true, false} {
		s := newB13State(t)
		events, err := convertB13(t, s, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"{}"}}]}}]}`)
		require.NoError(t, err)
		for _, r := range events {
			assert.NotEqual(t, "response.output_item.added", r.Value.(relayconvert.ChatToResponsesStreamEvent).Type)
		}
		if late {
			events, err = convertB13(t, s, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"real_late_id"}]}}]}`)
			require.NoError(t, err)
			require.Len(t, events, 2)
			assert.Equal(t, "real_late_id", events[0].Value.(relayconvert.ChatToResponsesStreamEvent).Payload.Item.CallId)
		}
		_, err = convertB13(t, s, `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		require.NoError(t, err)
		final, err := relayconvert.FinalizeStreamResponse(context.Background(), nil, s)
		require.NoError(t, err)
		response := final[len(final)-1].Value.(relayconvert.ChatToResponsesStreamEvent).Payload.Response
		require.Len(t, response.Output, 1)
		assert.NotEmpty(t, response.Output[0].CallId)
		if late {
			assert.Equal(t, "real_late_id", response.Output[0].CallId)
		}
	}
}

func TestB13ReasoningSummaryWireAndChatRoundTrip(t *testing.T) {
	var chat dto.OpenAITextResponse
	require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"public","choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":"thought"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"completion_tokens_details":{"reasoning_tokens":1}}}`), &chat))
	nonstream, err := relayconvert.ConvertResponse(context.Background(), nil, types.RelayFormatOpenAIResponses, &chat)
	require.NoError(t, err)
	s := newB13State(t)
	events, err := convertB13(t, s, `{"choices":[{"index":0,"delta":{"reasoning_content":"thought","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"completion_tokens_details":{"reasoning_tokens":1}}}`)
	require.NoError(t, err)
	for _, r := range events {
		event := r.Value.(relayconvert.ChatToResponsesStreamEvent)
		if event.Type == "response.output_item.done" && event.Payload.Item.Type == "reasoning" {
			require.Len(t, event.Payload.Item.Summary, 1)
			assert.Empty(t, event.Payload.Item.Content)
		}
	}
	final, err := relayconvert.FinalizeStreamResponse(context.Background(), nil, s)
	require.NoError(t, err)
	for _, resp := range []*dto.OpenAIResponsesResponse{nonstream.Value.(*dto.OpenAIResponsesResponse), final[len(final)-1].Value.(relayconvert.ChatToResponsesStreamEvent).Payload.Response} {
		for i := range resp.Output {
			if resp.Output[i].Type == "reasoning" {
				require.Len(t, resp.Output[i].Summary, 1)
				assert.Equal(t, "thought", resp.Output[i].Summary[0].Text)
				assert.Empty(t, resp.Output[i].Content)
				resp.Output[i].Content = []dto.ResponsesOutputContent{{Type: "summary_text", Text: "legacy-duplicate"}}
			}
		}
		back, err := relayconvert.ConvertResponse(context.Background(), nil, types.RelayFormatOpenAI, resp)
		require.NoError(t, err)
		actual := back.Value.(*dto.OpenAITextResponse)
		require.Len(t, actual.Choices, 1)
		assert.Equal(t, "thought", actual.Choices[0].Message.GetReasoningContent())
		assert.Equal(t, "answer", actual.Choices[0].Message.StringContent())
		assert.Equal(t, 5, actual.Usage.TotalTokens)
		assert.Equal(t, 1, actual.Usage.ChatReasoningTokens())
	}
	legacy := &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "reasoning", Content: []dto.ResponsesOutputContent{{Type: "summary_text", Text: "legacy"}}}}}
	back, err := relayconvert.ConvertResponse(context.Background(), nil, types.RelayFormatOpenAI, legacy)
	require.NoError(t, err)
	assert.Equal(t, "legacy", back.Value.(*dto.OpenAITextResponse).Choices[0].Message.GetReasoningContent())
}
