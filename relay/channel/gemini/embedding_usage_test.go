package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestEmbeddingHandlersUseUpstreamUsageAndPreserveFallback(t *testing.T) {
	for _, mode := range []string{"openai", "native-single", "native-batch"} {
		for _, tc := range []struct {
			name, metadata string
			want           int
			reported       bool
		}{
			{"upstream", `,"usageMetadata":{"promptTokenCount":42,"promptTokenDetails":[{"modality":"TEXT","tokenCount":12},{"modality":"IMAGE","tokenCount":30}]}`, 42, true},
			{"missing", "", 7, false}, {"zero", `,"usageMetadata":{"promptTokenCount":0}`, 7, false},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
				info := &relaycommon.RelayInfo{OriginModelName: "public-embedding", RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-2"}}
				info.SetEstimatePromptTokens(7)
				raw := `{"embeddings":[{"values":[0.25,0.5]}]` + tc.metadata + `}`
				if mode == "native-single" {
					raw = `{"embedding":{"values":[0.25,0.5]}` + tc.metadata + `}`
				}
				info.IsGeminiBatchEmbedding = mode == "native-batch"
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				if mode == "openai" {
					usage, apiErr = GeminiEmbeddingHandler(c, info, resp)
				} else {
					usage, apiErr = NativeGeminiEmbeddingHandler(c, resp, info)
				}
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, tc.want, usage.PromptTokens)
				assert.Equal(t, tc.want, usage.TotalTokens)
				assert.Zero(t, usage.CompletionTokens)
				if tc.reported {
					require.NotNil(t, usage.BillingUsage)
					require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
					assert.Equal(t, 42, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
					assert.Equal(t, 30, usage.PromptTokensDetails.ImageTokens)
					assert.Equal(t, 12, usage.PromptTokensDetails.TextTokens)
				}
				if mode == "openai" {
					assert.Equal(t, int64(tc.want), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens").Int())
					assert.Equal(t, "[0.25,0.5]", gjson.GetBytes(recorder.Body.Bytes(), "data.0.embedding").Raw)
				} else {
					assert.JSONEq(t, raw, recorder.Body.String())
				}
			})
		}
	}
}

func TestEmbeddingHandlersRejectInvalidResultsBeforeReturningUsage(t *testing.T) {
	for _, mode := range []string{"openai", "native-single", "native-batch"} {
		for _, tc := range []struct{ name, vector, metadata string }{
			{"empty", "", ""},
			{"empty vector", `{"values":[]}`, ""},
			{"null vector", `null`, ""},
			{"negative total", `{"values":[1]}`, `,"usageMetadata":{"promptTokenCount":-1}`},
			{"negative modality", `{"values":[1]}`, `,"usageMetadata":{"promptTokenCount":4,"promptTokenDetails":[{"modality":"TEXT","tokenCount":-1}]}`},
			{"oversized modality", `{"values":[1]}`, `,"usageMetadata":{"promptTokenCount":4,"promptTokenDetails":[{"modality":"IMAGE","tokenCount":3},{"modality":"TEXT","tokenCount":3}]}`},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
				info := &relaycommon.RelayInfo{IsGeminiBatchEmbedding: mode == "native-batch", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-2"}}
				info.SetEstimatePromptTokens(7)
				raw := `{"embeddings":[` + tc.vector + `]` + tc.metadata + `}`
				if mode == "native-single" {
					vector := tc.vector
					if vector == "" {
						vector = "null"
					}
					raw = `{"embedding":` + vector + tc.metadata + `}`
				}
				resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				if mode == "openai" {
					usage, apiErr = GeminiEmbeddingHandler(c, info, resp)
				} else {
					usage, apiErr = NativeGeminiEmbeddingHandler(c, resp, info)
				}
				assert.Nil(t, usage)
				require.NotNil(t, apiErr)
				assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
				assert.Empty(t, recorder.Body.String())
			})
		}
	}
}
