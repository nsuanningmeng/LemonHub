package zhipu_4v

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZhipuRequestURLs(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		format  types.RelayFormat
		mode    int
		wantURL string
		wantErr bool
	}{
		{name: "default Responses", format: types.RelayFormatOpenAIResponses, mode: relayconstant.RelayModeResponses, wantURL: "https://open.bigmodel.cn/api/v1/responses"},
		{name: "custom Responses prefix", baseURL: "https://gateway.example/zhipu", format: types.RelayFormatOpenAIResponses, mode: relayconstant.RelayModeResponses, wantURL: "https://gateway.example/zhipu/api/v1/responses"},
		{name: "Coding Plan Responses unsupported", baseURL: "glm-coding-plan", format: types.RelayFormatOpenAIResponses, mode: relayconstant.RelayModeResponses, wantErr: true},
		{name: "international Coding Plan Responses unsupported", baseURL: "glm-coding-plan-international", format: types.RelayFormatOpenAIResponses, mode: relayconstant.RelayModeResponses, wantErr: true},
		{name: "Coding Plan chat", baseURL: "glm-coding-plan", format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, wantURL: "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions"},
		{name: "international Coding Plan chat", baseURL: "glm-coding-plan-international", format: types.RelayFormatOpenAI, mode: relayconstant.RelayModeChatCompletions, wantURL: "https://api.z.ai/api/coding/paas/v4/chat/completions"},
		{name: "Coding Plan Claude", baseURL: "glm-coding-plan", format: types.RelayFormatClaude, wantURL: "https://open.bigmodel.cn/api/anthropic/v1/messages"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayFormat: test.format,
				RelayMode:   test.mode,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: test.baseURL},
			}
			actual, err := (&Adaptor{}).GetRequestURL(info)
			if test.wantErr {
				require.ErrorContains(t, err, "Responses API is not supported")
				assert.Empty(t, actual, "a virtual Coding Plan name must never become an upstream URL")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantURL, actual)
		})
	}
}

func TestZhipuChatPreservesReasoningEffortAndExplicitZeroParameters(t *testing.T) {
	for _, effort := range []string{"", "high"} {
		t.Run("effort="+effort, func(t *testing.T) {
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"glm-4.5","messages":[{"role":"user","content":"hello"}],"max_tokens":0,"temperature":0,"top_p":0,"stream":false}`, &request))
			request.ReasoningEffort = effort

			out, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &request)
			require.NoError(t, err)
			encoded, err := common.Marshal(out)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(encoded, &payload))
			if effort == "" {
				assert.NotContains(t, payload, "reasoning_effort")
			} else {
				assert.Equal(t, effort, payload["reasoning_effort"])
			}
			assert.Equal(t, float64(0), payload["max_tokens"])
			assert.Equal(t, float64(0), payload["temperature"])
			assert.Equal(t, float64(0), payload["top_p"])
			assert.Equal(t, false, payload["stream"])
		})
	}
}

func TestZhipuResponsesPreservesRequestPayload(t *testing.T) {
	const body = `{"model":"glm-4.5","input":[{"role":"user","content":"hello"}],"reasoning":{"effort":"high"},"max_output_tokens":0,"temperature":0,"stream":false,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalJsonStr(body, &request))

	out, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, request)
	require.NoError(t, err)
	encoded, err := common.Marshal(out)
	require.NoError(t, err)
	assert.JSONEq(t, body, string(encoded))
}
