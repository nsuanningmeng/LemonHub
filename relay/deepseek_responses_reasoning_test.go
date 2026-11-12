package relay

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type deepSeekResponsesCaptureTransport struct{ body []byte }

func (r *deepSeekResponsesCaptureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	r.body = body
	return &http.Response{StatusCode: 418, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"fixture stop"}}`)), Request: request}, nil
}

func TestDeepSeekResponsesFinalizesAfterOverridesAndSkipsPassthrough(t *testing.T) {
	oldGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldGlobal })
	service.InitHttpClient()
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	for _, tc := range []struct {
		name, initial, override                 string
		passthrough, global, want, replaceInput bool
	}{
		{name: "native", initial: "high", want: true},
		{name: "override replaces history", initial: "high", override: "high", replaceInput: true, want: true},
		{name: "override enables", initial: "none", override: "high", want: true},
		{name: "override disables", initial: "high", override: "none"},
		{name: "channel passthrough", initial: "high", passthrough: true},
		{name: "global passthrough", initial: "high", global: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = tc.global
			const input = `[{"type":"function_call","call_id":"c1","name":"lookup","arguments":"{}"}]`
			initialInput := input
			if tc.replaceInput {
				initialInput = `"hello"`
			}
			raw := []byte(`{"model":"deepseek-flash","reasoning":{"effort":"` + tc.initial + `"},"input":` + initialInput + `}`)
			var overrides map[string]any
			if tc.override != "" {
				overrides = map[string]any{"operations": []any{map[string]any{"mode": "set", "path": "reasoning.effort", "value": tc.override}, map[string]any{"mode": "set", "path": "vendor_extension", "value": "retained"}}}
			}
			if tc.replaceInput {
				var replacement any
				require.NoError(t, common.UnmarshalJsonStr(input, &replacement))
				overrides["operations"] = append(overrides["operations"].([]any), map[string]any{"mode": "set", "path": "input", "value": replacement})
			}
			c, info := passthroughTestContext(t, raw, false, "https://api.deepseek.com", overrides)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeDeepSeek)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "deepseek-flash")
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passthrough})
			c.Request.URL.Path = "/v1/responses"
			request := &dto.OpenAIResponsesRequest{}
			require.NoError(t, common.Unmarshal(raw, request))
			info.Request = request
			info.OriginModelName = "deepseek-flash"
			info.RequestURLPath = "/v1/responses"
			info.RelayMode = relayconstant.RelayModeResponses
			info.RelayFormat = types.RelayFormatOpenAIResponses
			capture := &deepSeekResponsesCaptureTransport{}
			client.Transport = capture
			apiErr := ResponsesHelper(c, info)
			require.NotNil(t, apiErr)
			require.Equal(t, 418, apiErr.StatusCode, apiErr.Error())
			if tc.want {
				assert.Equal(t, "reasoning", gjson.GetBytes(capture.body, "input.0.type").String())
				assert.Equal(t, " ", gjson.GetBytes(capture.body, "input.0.content.0.text").String())
				assert.Equal(t, "function_call", gjson.GetBytes(capture.body, "input.1.type").String())
			} else {
				assert.JSONEq(t, input, gjson.GetBytes(capture.body, "input").Raw)
			}
			if tc.override != "" {
				assert.Equal(t, tc.override, gjson.GetBytes(capture.body, "reasoning.effort").String())
				assert.Equal(t, "retained", gjson.GetBytes(capture.body, "vendor_extension").String())
			}
			assert.Equal(t, initialInput, string(request.Input), "conversion must not modify the caller's history")
		})
	}
}
