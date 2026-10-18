package claude

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCacheOnlyStreamDoesNotEstimateFreshInput(t *testing.T) {
	original := model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = false
	t.Cleanup(func() { model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = original })
	for _, tc := range []struct {
		name, usage string
		complete    bool
		output      int
	}{
		{"cache creation refusal", `{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":20000,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":20000}}`, true, 0},
		{"cache read interrupted", `{"input_tokens":0,"output_tokens":1,"cache_read_input_tokens":50000}`, false, 0},
		{"explicit zero input successful output", `{"input_tokens":0,"output_tokens":1}`, true, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-4-8"}}
			info.SetEstimatePromptTokens(1200)
			state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
			start := fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_1","model":"claude-opus-4-8","usage":%s}}`, tc.usage)
			require.Nil(t, HandleStreamResponseData(c, info, state, start))
			if tc.complete {
				delta := fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":%d}}`, tc.output)
				require.Nil(t, HandleStreamResponseData(c, info, state, delta))
				require.Nil(t, HandleStreamResponseData(c, info, state, `{"type":"message_stop"}`))
			}
			HandleStreamFinalResponse(c, info, state)
			assert.Zero(t, state.Usage.PromptTokens)
			require.NotNil(t, state.Usage.BillingUsage)
			assert.Zero(t, state.Usage.BillingUsage.ClaudeUsage.InputTokens, "settlement must not charge fresh input for cached prompts")
			var originalUsage dto.ClaudeUsage
			require.NoError(t, common.UnmarshalJsonStr(tc.usage, &originalUsage))
			assert.Equal(t, originalUsage.CacheReadInputTokens, state.Usage.BillingUsage.ClaudeUsage.CacheReadInputTokens)
			assert.Equal(t, originalUsage.CacheCreationInputTokens, state.Usage.BillingUsage.ClaudeUsage.CacheCreationInputTokens)
			if tc.complete {
				assert.Equal(t, tc.output, state.Usage.CompletionTokens)
			}
		})
	}
}
