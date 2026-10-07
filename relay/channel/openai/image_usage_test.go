package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImageUsageBillsOutputImagesAtTheirExpressionPrice(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, endpoint := range []struct {
		name  string
		path  string
		mode  int
		event string
	}{
		{"generations", "/v1/images/generations", relayconstant.RelayModeImagesGenerations, "image_generation.completed"},
		{"edits", "/v1/images/edits", relayconstant.RelayModeImagesEdits, "image_edit.completed"},
	} {
		for _, format := range []struct {
			name        string
			contentType string
			stream      bool
		}{
			{"JSON", "application/json", false},
			{"SSE", "text/event-stream", true},
			{"JSON upstream for streaming client", "application/json", true},
		} {
			t.Run(endpoint.name+"/"+format.name, func(t *testing.T) {
				const reportedUsage = `"usage":{"input_tokens":15,"output_tokens":1352,"total_tokens":1367,"input_tokens_details":{"text_tokens":15,"image_tokens":0},"output_tokens_details":{"image_tokens":1120,"text_tokens":232}}`
				body := `{"created":1710000000,"data":[{"b64_json":"image"}],` + reportedUsage + `}`
				if format.contentType == "text/event-stream" {
					body = `data: {"type":"` + endpoint.event + `","b64_json":"image",` + reportedUsage + "}\n\ndata: [DONE]\n\n"
				}
				c, recorder, resp, info := newImageTestContext(t, body, format.contentType, format.stream)
				c.Request = httptest.NewRequest(http.MethodPost, endpoint.path, nil)
				info.RelayMode = endpoint.mode

				result, apiErr := (&Adaptor{}).DoResponse(c, resp, info)

				require.Nil(t, apiErr)
				usage, ok := result.(*dto.Usage)
				require.True(t, ok)
				require.NotNil(t, usage)
				assert.Equal(t, 1352, usage.CompletionTokens)
				assert.Equal(t, 1120, usage.CompletionTokenDetails.ImageTokens)
				assert.Equal(t, 232, usage.CompletionTokenDetails.TextTokens)
				const imageExpr = `tier("base", p * 2 + c * 12 + img_o * 120)`
				params := service.BuildTieredTokenParams(usage, false, billingexpr.UsedVars(imageExpr))
				assert.Equal(t, float64(232), params.C, "separately priced image tokens must leave the text bucket")
				assert.Equal(t, float64(1120), params.ImgO)
				cost, _, err := billingexpr.RunExpr(imageExpr, params)
				require.NoError(t, err)
				assert.Equal(t, float64(137214), cost, "15*2 + 232*12 + 1120*120")

				const textExpr = `tier("base", p * 2 + c * 12)`
				params = service.BuildTieredTokenParams(usage, false, billingexpr.UsedVars(textExpr))
				cost, _, err = billingexpr.RunExpr(textExpr, params)
				require.NoError(t, err)
				assert.Equal(t, float64(16254), cost, "without img_o, all 1352 output tokens retain the base price")
				if format.stream {
					assert.Contains(t, recorder.Body.String(), `"image_tokens":1120`)
					assert.Contains(t, recorder.Body.String(), "data: [DONE]")
				} else {
					assert.Equal(t, body, recorder.Body.String(), "normalization must not rewrite the upstream JSON")
				}
			})
		}
	}
}

func TestOpenAIImageUsagePreservesCanonicalOutputDetailsAndExplicitZero(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	for _, tt := range []struct {
		name    string
		details string
		want    dto.OutputTokenDetails
	}{
		{
			name:    "missing native details retain normalized details",
			details: `"completion_tokens_details":{"image_tokens":100,"text_tokens":20,"audio_tokens":3,"reasoning_tokens":4}`,
			want:    dto.OutputTokenDetails{ImageTokens: 100, TextTokens: 20, AudioTokens: 3, ReasoningTokens: 4},
		},
		{
			name:    "null native details retain normalized details",
			details: `"output_tokens_details":null,"completion_tokens_details":{"image_tokens":100}`,
			want:    dto.OutputTokenDetails{ImageTokens: 100},
		},
		{
			name:    "native details fill absent fields without replacing canonical zero or counts",
			details: `"output_tokens_details":{"image_tokens":100,"text_tokens":20,"audio_tokens":3,"reasoning_tokens":4},"completion_tokens_details":{"image_tokens":0,"reasoning_tokens":8}`,
			want:    dto.OutputTokenDetails{ImageTokens: 0, TextTokens: 20, AudioTokens: 3, ReasoningTokens: 8},
		},
		{
			name:    "null canonical fields fall back to native details",
			details: `"output_tokens_details":{"image_tokens":100},"completion_tokens_details":{"image_tokens":null}`,
			want:    dto.OutputTokenDetails{ImageTokens: 100},
		},
		{
			name:    "explicit native zero does not imply all output is image tokens",
			details: `"output_tokens_details":{"image_tokens":0,"text_tokens":120}`,
			want:    dto.OutputTokenDetails{ImageTokens: 0, TextTokens: 120},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"data":[{"b64_json":"image"}],"usage":{"input_tokens":15,"output_tokens":1352,` + tt.details + `}}`
			c, _, resp, info := newImageTestContext(t, body, "application/json", false)

			usage, apiErr := OpenaiImageHandler(c, info, resp)

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, tt.want, usage.CompletionTokenDetails)
		})
	}
}
