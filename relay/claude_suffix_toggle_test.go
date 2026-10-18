package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeNativeEffortSuffixHonorsAdapterToggle(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	oldEnabled := settings.ThinkingAdapterEnabled
	oldPassthrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = oldEnabled
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldPassthrough
	})
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	service.InitHttpClient()
	for _, enabled := range []bool{false, true} {
		name := "disabled keeps upstream model verbatim"
		if enabled {
			name = "enabled renders adaptive effort"
		}
		t.Run(name, func(t *testing.T) {
			settings.ThinkingAdapterEnabled = enabled
			var got []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(`{"error":{"message":"fixture stop","type":"invalid_request_error"}}`))
			}))
			defer upstream.Close()
			c, info := passthroughTestContext(t, nil, true, upstream.URL, nil)
			info.OriginModelName = "claude-opus-4-8-high"
			info.Request = &dto.ClaudeRequest{Model: info.OriginModelName, MaxTokens: common.GetPointer(uint(1024)), Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}}}
			common.SetContextKey(c, constant.ContextKeyOriginalModel, info.OriginModelName)
			apiErr := ClaudeHelper(c, info)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusTeapot, apiErr.StatusCode)
			if enabled {
				assert.Equal(t, "claude-opus-4-8", gjson.GetBytes(got, "model").String())
				assert.Equal(t, "adaptive", gjson.GetBytes(got, "thinking.type").String())
				assert.Equal(t, "high", gjson.GetBytes(got, "output_config.effort").String())
			} else {
				assert.Equal(t, "claude-opus-4-8-high", gjson.GetBytes(got, "model").String())
				assert.False(t, gjson.GetBytes(got, "thinking").Exists())
				assert.False(t, gjson.GetBytes(got, "output_config").Exists())
			}
		})
	}
}
