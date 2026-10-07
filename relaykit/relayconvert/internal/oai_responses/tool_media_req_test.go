package oairesponses

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesImageWireShapeForMessagesAndToolOutputs(t *testing.T) {
	for _, tt := range []struct {
		name string
		part map[string]any
		want string
	}{
		{"string", map[string]any{"type": "input_image", "image_url": "https://example.test/a.png"}, `{"url":"https://example.test/a.png"}`},
		{"data URL and detail", map[string]any{"type": "input_image", "image_url": "data:image/png;base64,YQ==", "detail": "high"}, `{"url":"data:image/png;base64,YQ==","detail":"high"}`},
		{"object keeps inner detail", map[string]any{"type": "input_image", "image_url": map[string]any{"url": "https://example.test/b.png", "detail": "low"}, "detail": "high"}, `{"url":"https://example.test/b.png","detail":"low"}`},
		{"object preserves sibling detail", map[string]any{"type": "input_image", "image_url": map[string]any{"url": "https://example.test/b.png"}, "detail": "high"}, `{"url":"https://example.test/b.png","detail":"high"}`},
		{"sibling URL", map[string]any{"type": "input_image", "url": "https://example.test/c.png", "detail": "auto"}, `{"url":"https://example.test/c.png","detail":"auto"}`},
		{"file ID compatibility", map[string]any{"type": "input_image", "file_id": "file_existing", "detail": "low"}, `{"file_id":"file_existing","detail":"low"}`},
		{"string keeps file ID", map[string]any{"type": "input_image", "image_url": "https://example.test/d.png", "file_id": "file_existing"}, `{"url":"https://example.test/d.png","file_id":"file_existing"}`},
		{"URL with nullable unused ID", map[string]any{"type": "input_image", "image_url": "https://example.test/e.png", "file_id": nil}, `{"url":"https://example.test/e.png"}`},
		{"ID with nullable unused URL", map[string]any{"type": "input_image", "image_url": nil, "file_id": "file_existing"}, `{"file_id":"file_existing"}`},
	} {
		for _, toolOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tool=%t", tt.name, toolOutput), func(t *testing.T) {
				items := []map[string]any{{"role": "user", "content": []any{tt.part}}}
				index := 0
				if toolOutput {
					items = []map[string]any{
						{"type": "function_call", "call_id": "call_image", "name": "view_image", "arguments": "{}"},
						{"type": "function_call_output", "call_id": "call_image", "output": []any{tt.part}},
					}
					index = 2
				}
				got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, items)})
				require.NoError(t, err)
				raw, err := kitutil.Marshal(got)
				require.NoError(t, err)
				image := gjson.GetBytes(raw, fmt.Sprintf("messages.%d.content.0.image_url", index))
				require.True(t, image.IsObject(), "Chat image_url must be an object on the wire: %s", raw)
				assert.JSONEq(t, tt.want, image.Raw)
				assert.False(t, image.Get("MimeType").Exists())
				if toolOutput {
					assert.Equal(t, "[image]", got.Messages[1].StringContent())
				}
			})
		}
	}
}

func TestResponsesToolMediaPreservesMixedUnknownBlocksAndTextOrder(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test", Input: mustRawMessage(t, []map[string]any{
			{"type": "function_call", "call_id": "call_1", "name": "view_image", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call_1", "output": []any{
				map[string]any{"type": "input_text", "text": "before"},
				map[string]any{"type": "custom_result", "value": "keep me"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64,YQ=="},
				map[string]any{"type": "input_text", "text": "after"},
				17,
			}},
		}),
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	assert.Equal(t, "call_1", got.Messages[1].ToolCallId)
	assert.Equal(t, "before\n{\"type\":\"custom_result\",\"value\":\"keep me\"}\nafter\n17", got.Messages[1].StringContent())
	assert.NotContains(t, got.Messages[1].StringContent(), "base64")
	raw, err := kitutil.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "data:image/png;base64,YQ==", gjson.GetBytes(raw, "messages.2.content.0.image_url.url").String())
}

func TestResponsesToolMediaFlushesAfterWholeResultBatchAtEachBoundary(t *testing.T) {
	for _, tt := range []struct {
		name   string
		suffix []map[string]any
		roles  []string
	}{
		{"EOF", nil, []string{"assistant", "tool", "tool", "user"}},
		{"user message", []map[string]any{{"role": "user", "content": "continue"}}, []string{"assistant", "tool", "tool", "user", "user"}},
		{"assistant message", []map[string]any{{"role": "assistant", "content": "done"}}, []string{"assistant", "tool", "tool", "user", "assistant"}},
		{"next tool round", []map[string]any{
			{"type": "function_call", "call_id": "c", "name": "view_c", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "c", "output": []any{map[string]any{"type": "input_image", "image_url": "https://example.test/c.png"}}},
		}, []string{"assistant", "tool", "tool", "user", "assistant", "tool", "user"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := []map[string]any{
				{"type": "function_call", "call_id": "a", "name": "view_a", "arguments": "{}"},
				{"type": "function_call", "call_id": "b", "name": "view_b", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "b", "output": []any{map[string]any{"type": "input_image", "image_url": "https://example.test/b.png"}}},
				{"type": "function_call_output", "call_id": "a", "output": []any{map[string]any{"type": "input_image", "image_url": "https://example.test/a.png"}}},
			}
			items = append(items, tt.suffix...)
			got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, items)})
			require.NoError(t, err)
			roles := make([]string, 0, len(got.Messages))
			for _, message := range got.Messages {
				roles = append(roles, message.Role)
			}
			require.Equal(t, tt.roles, roles)
			calls := got.Messages[0].ParseToolCalls()
			require.Len(t, calls, 2)
			assert.Equal(t, "a", calls[0].ID)
			assert.Equal(t, "view_a", calls[0].Function.Name)
			assert.Equal(t, "b", calls[1].ID)
			assert.Equal(t, "view_b", calls[1].Function.Name)
			assert.Equal(t, "b", got.Messages[1].ToolCallId)
			assert.Equal(t, "a", got.Messages[2].ToolCallId)
			raw, err := kitutil.Marshal(got)
			require.NoError(t, err)
			images := gjson.GetBytes(raw, "messages.3.content").Array()
			require.Len(t, images, 2)
			assert.Equal(t, "https://example.test/b.png", images[0].Get("image_url.url").String())
			assert.Equal(t, "https://example.test/a.png", images[1].Get("image_url.url").String())
			if tt.name == "next tool round" {
				assert.Equal(t, "c", got.Messages[4].ParseToolCalls()[0].ID)
				assert.Equal(t, "c", got.Messages[5].ToolCallId)
				assert.Equal(t, "https://example.test/c.png", gjson.GetBytes(raw, "messages.6.content.0.image_url.url").String())
			}
		})
	}
}

func TestResponsesToolMediaWaitsForPlainTextSiblingResult(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, []map[string]any{
		{"type": "function_call", "call_id": "a", "name": "image", "arguments": "{}"},
		{"type": "function_call", "call_id": "b", "name": "text", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "a", "output": []any{map[string]any{"type": "input_image", "image_url": "https://example.test/a.png"}}},
		{"type": "function_call_output", "call_id": "b", "output": "plain sibling result"},
	})})
	require.NoError(t, err)
	require.Len(t, got.Messages, 4)
	assert.Equal(t, "tool", got.Messages[2].Role)
	assert.Equal(t, "b", got.Messages[2].ToolCallId)
	assert.Equal(t, "plain sibling result", got.Messages[2].StringContent())
	assert.Equal(t, "user", got.Messages[3].Role)
}

func TestResponsesToolOutputWithoutMediaPreservesLegacyContent(t *testing.T) {
	for _, tt := range []struct {
		name   string
		output any
		want   string
	}{
		{"nil", nil, ""},
		{"string", "unchanged", "unchanged"},
		{"ordinary object", map[string]any{"ok": true}, `{"ok":true}`},
		{"numeric array", []any{1, 2}, `[1,2]`},
		{"unknown block", []any{map[string]any{"type": "custom_result", "value": "keep"}}, `[{"type":"custom_result","value":"keep"}]`},
		{"pure text blocks", []any{map[string]any{"type": "input_text", "text": "first"}, map[string]any{"type": "input_text", "text": "second"}}, `[{"text":"first","type":"input_text"},{"text":"second","type":"input_text"}]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, []map[string]any{
				{"type": "function_call", "call_id": "a", "name": "lookup", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "a", "output": tt.output},
			})})
			require.NoError(t, err)
			require.Len(t, got.Messages, 2)
			assert.Equal(t, "tool", got.Messages[1].Role)
			assert.Equal(t, tt.want, got.Messages[1].StringContent())
		})
	}
}

func TestResponsesToolMediaReusesFileAudioVideoMappings(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, []map[string]any{
		{"type": "function_call", "call_id": "a", "name": "media", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "a", "output": []any{
			map[string]any{"type": "input_text", "text": "attachments"},
			map[string]any{"type": "input_file", "filename": "a.pdf", "file_data": "data:application/pdf;base64,YQ=="},
			map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "YQ==", "format": "wav"}},
			map[string]any{"type": "input_video", "video_url": map[string]any{"url": "https://example.test/v.mp4"}},
		}},
	})})
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	assert.Equal(t, "attachments", got.Messages[1].StringContent())
	raw, err := kitutil.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "data:application/pdf;base64,YQ==", gjson.GetBytes(raw, "messages.2.content.0.file.file_data").String())
	assert.Equal(t, "a.pdf", gjson.GetBytes(raw, "messages.2.content.0.file.filename").String())
	assert.Equal(t, "wav", gjson.GetBytes(raw, "messages.2.content.1.input_audio.format").String())
	assert.Equal(t, "https://example.test/v.mp4", gjson.GetBytes(raw, "messages.2.content.2.video_url").String())
}

func TestResponsesRecognizedMediaRejectsMalformedPayload(t *testing.T) {
	for _, tt := range []struct {
		name string
		part map[string]any
	}{
		{"image number", map[string]any{"type": "input_image", "image_url": 17}},
		{"image empty", map[string]any{"type": "input_image", "image_url": ""}},
		{"image invalid URL object", map[string]any{"type": "input_image", "image_url": map[string]any{"url": true}}},
		{"image invalid detail", map[string]any{"type": "input_image", "image_url": "https://example.test/a.png", "detail": 17}},
		{"image all sources null", map[string]any{"type": "input_image", "image_url": nil, "file_id": nil}},
		{"file all sources null", map[string]any{"type": "input_file", "file_data": nil, "file_id": nil}},
		{"file missing payload", map[string]any{"type": "input_file", "filename": "a.pdf"}},
		{"audio missing format", map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "YQ=="}}},
		{"video empty URL", map[string]any{"type": "input_video", "video_url": map[string]any{"url": ""}}},
	} {
		for _, tool := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tool=%t", tt.name, tool), func(t *testing.T) {
				items := []map[string]any{{"role": "user", "content": []any{tt.part}}}
				if tool {
					items = []map[string]any{{"type": "function_call_output", "call_id": "a", "output": []any{tt.part}}}
				}
				_, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, items)})
				require.Error(t, err, "recognized malformed media must not reach the upstream as invalid wire data")
			})
		}
	}
}

func TestResponsesFileWireOmitsNullableUnusedSources(t *testing.T) {
	part := map[string]any{"type": "input_file", "filename": "a.pdf", "file_data": "data:application/pdf;base64,YQ==", "file_id": nil, "file_url": nil}
	for _, tool := range []bool{false, true} {
		t.Run(fmt.Sprintf("tool=%t", tool), func(t *testing.T) {
			items := []map[string]any{{"role": "user", "content": []any{part}}}
			index := 0
			if tool {
				items = []map[string]any{{"type": "function_call_output", "call_id": "a", "output": []any{part}}}
				index = 1
			}
			got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "gpt-test", Input: mustRawMessage(t, items)})
			require.NoError(t, err)
			wire, err := kitutil.Marshal(got)
			require.NoError(t, err)
			file := gjson.GetBytes(wire, fmt.Sprintf("messages.%d.content.0.file", index))
			assert.JSONEq(t, `{"filename":"a.pdf","file_data":"data:application/pdf;base64,YQ=="}`, file.Raw)
		})
	}
}
