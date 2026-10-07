package advancedcustom

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAdaptorResponsesMediaProducesStrictChatWireAfterToolBatch(t *testing.T) {
	info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
		IncomingPath: "/v1/responses", UpstreamPath: "/v1/chat/completions", Converter: relayconvert.ConverterOpenAIResponsesToOpenAIChat,
	}}})
	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(advancedCustomGinContext("/v1/responses"), info, dto.OpenAIResponsesRequest{
		Model: "gpt-test", Input: mustAdvancedCustomRawMessage(t, []map[string]any{
			{"role": "user", "content": []any{map[string]any{"type": "input_image", "image_url": "https://example.test/input.png", "detail": "high"}}},
			{"type": "function_call", "call_id": "a", "name": "view_image", "arguments": "{}"},
			{"type": "function_call", "call_id": "b", "name": "lookup", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "a", "output": []any{
				map[string]any{"type": "input_text", "text": "screenshot"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64,YQ==", "detail": "low"},
			}},
			{"type": "function_call_output", "call_id": "b", "output": "done"},
		}),
	})
	require.NoError(t, err)
	wire, err := common.Marshal(converted)
	require.NoError(t, err)
	messages := gjson.GetBytes(wire, "messages").Array()
	require.Len(t, messages, 5)
	assert.True(t, messages[0].Get("content.0.image_url").IsObject())
	assert.Equal(t, "high", messages[0].Get("content.0.image_url.detail").String())
	assert.Equal(t, "assistant", messages[1].Get("role").String())
	assert.Equal(t, "a", messages[2].Get("tool_call_id").String())
	assert.Equal(t, "screenshot", messages[2].Get("content").String())
	assert.Equal(t, "b", messages[3].Get("tool_call_id").String())
	assert.Equal(t, "done", messages[3].Get("content").String())
	assert.Equal(t, "user", messages[4].Get("role").String())
	assert.True(t, messages[4].Get("content.0.image_url").IsObject())
	assert.Equal(t, "data:image/png;base64,YQ==", messages[4].Get("content.0.image_url.url").String())
	assert.Equal(t, "low", messages[4].Get("content.0.image_url.detail").String())
	assert.NotContains(t, messages[2].Get("content").String(), "base64")
	assert.NotContains(t, string(wire), "MimeType")
}
