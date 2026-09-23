package advancedcustom

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptorCrossProtocolChatStreamUsage(t *testing.T) {
	for _, protocol := range []struct {
		format    types.RelayFormat
		path      string
		converter string
		mode      int
	}{
		{types.RelayFormatClaude, "/v1/messages", relayconvert.ConverterClaudeMessagesToOpenAIChat, relayconstant.RelayModeChatCompletions},
		{types.RelayFormatGemini, "/v1beta/models/gpt-test:generateContent", relayconvert.ConverterGeminiContentToOpenAIChat, relayconstant.RelayModeGemini},
		{types.RelayFormatOpenAIResponses, "/v1/responses", relayconvert.ConverterOpenAIResponsesToOpenAIChat, relayconstant.RelayModeResponses},
	} {
		for _, tc := range []struct {
			name      string
			stream    bool
			supported bool
		}{
			{"supported stream", true, true},
			{"non-stream", false, true},
			{"unsupported stream", true, false},
		} {
			t.Run(string(protocol.format)+"/"+tc.name, func(t *testing.T) {
				info := advancedCustomRelayInfo(&dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
					IncomingPath: protocol.path, UpstreamPath: "/v1/chat/completions", Converter: protocol.converter,
				}}})
				info.RelayFormat = protocol.format
				info.RelayMode = protocol.mode
				info.RequestURLPath = protocol.path
				info.IsStream = tc.stream
				info.SupportStreamOptions = tc.supported
				c := advancedCustomGinContext(protocol.path)
				adaptor := &Adaptor{}
				adaptor.Init(info)
				var converted any
				var err error
				switch protocol.format {
				case types.RelayFormatClaude:
					converted, err = adaptor.ConvertClaudeRequest(c, info, &dto.ClaudeRequest{
						Model: "gpt-test", Stream: &tc.stream,
						Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
					})
				case types.RelayFormatGemini:
					converted, err = adaptor.ConvertGeminiRequest(c, info, &dto.GeminiChatRequest{
						Contents: []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{Text: "hello"}}}},
					})
				case types.RelayFormatOpenAIResponses:
					converted, err = adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
						Model: "gpt-test", Stream: &tc.stream, Input: mustAdvancedCustomRawMessage(t, "hello"),
					})
				}
				require.NoError(t, err)
				payload, err := common.Marshal(converted)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, common.Unmarshal(payload, &body))
				assert.Equal(t, "gpt-test", body["model"])
				assert.Equal(t, tc.stream, body["stream"])
				assert.Equal(t, constant.ChannelTypeAdvancedCustom, info.ChannelType)
				if tc.stream && tc.supported {
					assert.Equal(t, map[string]any{"include_usage": true}, body["stream_options"])
				} else {
					assert.NotContains(t, body, "stream_options")
				}
			})
		}
	}
}
