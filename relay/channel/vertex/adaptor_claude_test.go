package vertex

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVertexClaudeInputUsesSelectedProviderProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		mode        int
	}{
		{"Gemini", "gemini-2.5-flash", RequestModeGemini},
		{"native Claude", "claude-3-5-sonnet-20241022", RequestModeClaude},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, OriginModelName: "client-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}}
			var request dto.ClaudeRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"client-model","max_tokens":16,"system":"system text","messages":[{"role":"user","content":"hello"}],"temperature":0}`), &request))
			a := Adaptor{}
			a.Init(info)
			require.Equal(t, tc.mode, a.RequestMode)
			converted, err := a.ConvertClaudeRequest(c, info, &request)
			require.NoError(t, err)
			raw, err := common.Marshal(converted)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, common.Unmarshal(raw, &wire))
			if tc.mode == RequestModeGemini {
				require.IsType(t, &dto.GeminiChatRequest{}, converted)
				assert.Contains(t, wire, "contents")
				assert.Contains(t, wire, "generationConfig")
				assert.NotContains(t, wire, "anthropic_version")
				assert.NotContains(t, wire, "messages")
				assert.NotContains(t, wire, "max_tokens")
				assert.Equal(t, float64(16), wire["generationConfig"].(map[string]any)["maxOutputTokens"])
				assert.Equal(t, float64(0), wire["generationConfig"].(map[string]any)["temperature"])
			} else {
				assert.Equal(t, "vertex-2023-10-16", wire["anthropic_version"])
				assert.Contains(t, wire, "messages")
			}
			assert.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.RelayFormat, "downstream remains Claude Messages")
			assert.Equal(t, "client-model", request.Model, "conversion does not mutate client DTO")
		})
	}
}

func TestVertexClaudeUnsupportedAndInvalidRequestsFailBeforeDispatch(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama-3.3"}}
	a := Adaptor{}
	a.Init(info)
	for _, tc := range []struct {
		name    string
		info    *relaycommon.RelayInfo
		request *dto.ClaudeRequest
	}{
		{"unsupported open source", info, &dto.ClaudeRequest{Model: "private_model"}},
		{"nil request", info, nil},
		{"nil relay information", nil, &dto.ClaudeRequest{Model: "private_model"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.ConvertClaudeRequest(c, tc.info, tc.request)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, 400, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.NotContains(t, err.Error(), "private_model")
		})
	}
}
