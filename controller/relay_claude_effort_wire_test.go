package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeEffortOpenAIProductionWire(t *testing.T) {
	for _, tc := range []struct{ name, effort, override, want string }{
		{"low", "low", "", "low"}, {"medium", "medium", "", "medium"}, {"high", "high", "", "high"},
		{"operator_override_last", "high", `{"reasoning_effort":"low"}`, "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			var wire map[string]interface{}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(raw, &wire))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chatcmpl-local","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			t.Cleanup(upstream.Close)
			base := upstream.URL
			channel := model.Channel{Id: 671510, Type: constant.ChannelTypeOpenAI, Key: "fixture-key", Status: common.ChannelStatusEnabled, Name: "effort-local", BaseURL: &base, Models: "gpt-4o", Group: "default"}
			if tc.override != "" {
				channel.ParamOverride = &tc.override
			}
			require.NoError(t, db.Create(&channel).Error)
			body := `{"model":"gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"output_config":{"effort":"` + tc.effort + `"}}`
			response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/messages", body, types.RelayFormatClaude)
			require.Equal(t, 200, response.Code, response.Body.String())
			assert.Equal(t, tc.want, wire["reasoning_effort"])
			assert.NotContains(t, wire, "output_config")
			assert.Equal(t, "gpt-4o", wire["model"], "suffix priority is not modified by this fixture")
			assert.True(t, strings.Contains(response.Body.String(), "OK"))
		})
	}
}
