package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bailian multimodal embedding models are served only by the native
// multimodal-embedding endpoint, which takes input.contents; text embedding
// models keep compatible mode and the OpenAI body.
func TestEmbeddingRequestByModel(t *testing.T) {
	const (
		base       = "https://dashscope.invalid"
		nativePath = "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding"
	)
	tests := []struct {
		model    string
		request  string // OpenAI embedding request fields after model
		wantPath string
		wantBody string // "" means a conversion error
	}{
		{
			model:    "qwen3-vl-embedding",
			request:  `"input":"a cat"`,
			wantPath: nativePath,
			wantBody: `{"model":"qwen3-vl-embedding","input":{"contents":[{"text":"a cat"}]}}`,
		},
		{
			model:    "tongyi-embedding-vision-plus-2026-03-06",
			request:  `"input":["a cat","a dog"],"dimensions":1024`,
			wantPath: nativePath,
			wantBody: `{"model":"tongyi-embedding-vision-plus-2026-03-06","input":{"contents":[{"text":"a cat"},{"text":"a dog"}]},"parameters":{"dimension":1024}}`,
		},
		{
			model: "multimodal-embedding-v1",
			request: `"input":[{"type":"text","text":"a cat"},` +
				`{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}},` +
				`{"type":"video_url","video_url":{"url":"https://example.com/cat.mp4"}}]`,
			wantPath: nativePath,
			wantBody: `{"model":"multimodal-embedding-v1","input":{"contents":[{"text":"a cat"},` +
				`{"image":"data:image/png;base64,iVBORw0KGgo="},{"video":"https://example.com/cat.mp4"}]}}`,
		},
		{
			model:    "qwen3-vl-embedding",
			request:  `"input":[{"type":"image_url","image_url":{"url":""}}]`,
			wantPath: nativePath,
		},
		{
			model:    "text-embedding-v4",
			request:  `"input":"a cat","dimensions":1024`,
			wantPath: "/compatible-mode/v1/embeddings",
			wantBody: `{"model":"text-embedding-v4","input":"a cat","dimensions":1024}`,
		},
		{
			model:    "qwen3-vl-embedding",
			request:  `"input":[" \t ",{"type":"text","text":" \t "}]`,
			wantPath: nativePath,
			wantBody: `{"model":"qwen3-vl-embedding","input":{"contents":[{"text":" \t "},{"text":" \t "}]}}`,
		},
	}
	for _, tt := range tests {
		info := &relaycommon.RelayInfo{
			RelayMode:   constant.RelayModeEmbeddings,
			RelayFormat: types.RelayFormatEmbedding,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: base, UpstreamModelName: tt.model},
		}
		url, err := (&Adaptor{}).GetRequestURL(info)
		require.NoError(t, err, tt.request)
		assert.Equal(t, base+tt.wantPath, url, tt.request)

		var request dto.EmbeddingRequest
		require.NoError(t, common.UnmarshalJsonStr(`{"model":"`+tt.model+`",`+tt.request+`}`, &request))
		converted, err := (&Adaptor{}).ConvertEmbeddingRequest(nil, info, request)
		if tt.wantBody == "" {
			assert.Error(t, err, tt.request)
			continue
		}
		require.NoError(t, err, tt.request)
		wire, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, tt.wantBody, string(wire), tt.request)
	}
}

func TestMultimodalEmbeddingRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"null input", `null`},
		{"empty list", `[]`},
		{"empty string", `""`},
		{"empty string item", `[""]`},
		{"invalid item type", `[42]`},
		{"missing type", `[{"text":"a cat"}]`},
		{"invalid type", `[{"type":42,"text":"a cat"}]`},
		{"missing text", `[{"type":"text"}]`},
		{"null text", `[{"type":"text","text":null}]`},
		{"number text", `[{"type":"text","text":42}]`},
		{"object text", `[{"type":"text","text":{}}]`},
		{"empty text", `[{"type":"text","text":""}]`},
		{"mixed valid and invalid text", `["a cat",{"type":"text","text":false}]`},
		{"missing image", `[{"type":"image_url"}]`},
		{"invalid image object", `[{"type":"image_url","image_url":"https://example.com/cat.png"}]`},
		{"missing image URL", `[{"type":"image_url","image_url":{}}]`},
		{"invalid image URL", `[{"type":"image_url","image_url":{"url":42}}]`},
		{"empty image URL", `[{"type":"image_url","image_url":{"url":""}}]`},
		{"missing video", `[{"type":"video_url"}]`},
		{"invalid video object", `[{"type":"video_url","video_url":[]}]`},
		{"missing video URL", `[{"type":"video_url","video_url":{}}]`},
		{"invalid video URL", `[{"type":"video_url","video_url":{"url":false}}]`},
		{"null video URL", `[{"type":"video_url","video_url":{"url":null}}]`},
		{"empty video URL", `[{"type":"video_url","video_url":{"url":""}}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request dto.EmbeddingRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"qwen3-vl-embedding","input":`+tt.input+`}`, &request))
			converted, err := convertToMultimodalEmbeddingRequest(request)
			require.Error(t, err)
			assert.Nil(t, converted)
		})
	}
}

// The native response becomes an OpenAI embedding list. prompt_tokens is all
// billable input and image_tokens its visual part, priced at the image ratio;
// output_tokens is not priced and never billed. A response without usage
// bills the request estimate.
func TestMultimodalEmbeddingResponse(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		body      string
		estimate  int
		wantData  []dto.EmbeddingResponseItem
		wantUsage *dto.Usage // nil means an upstream error
	}{
		{
			name:  "visual tokens reported apart",
			model: "qwen3-vl-embedding",
			body: `{"output":{"embeddings":[{"index":0,"embedding":[0.1,-0.2],"type":"text"},{"index":1,"embedding":[0.3,0.4],"type":"image"}]},` +
				`"usage":{"input_tokens":43,"image_tokens":1247,"total_tokens":1290},"request_id":"r1"}`,
			wantData: []dto.EmbeddingResponseItem{
				{Object: "embedding", Index: 0, Embedding: []float64{0.1, -0.2}},
				{Object: "embedding", Index: 1, Embedding: []float64{0.3, 0.4}},
			},
			wantUsage: &dto.Usage{PromptTokens: 1290, TotalTokens: 1290,
				PromptTokensDetails: dto.InputTokenDetails{TextTokens: 43, ImageTokens: 1247}},
		},
		{
			name:  "input_tokens split in input_tokens_details",
			model: "tongyi-embedding-vision-plus",
			body: `{"output":{"embeddings":[{"index":0,"embedding":[0.5],"type":"fused"}]},` +
				`"usage":{"input_tokens":903,"input_tokens_details":{"image_tokens":896,"text_tokens":7},"output_tokens":3,"total_tokens":906},"request_id":"r2"}`,
			wantData: []dto.EmbeddingResponseItem{{Object: "embedding", Index: 0, Embedding: []float64{0.5}}},
			wantUsage: &dto.Usage{PromptTokens: 903, TotalTokens: 903,
				PromptTokensDetails: dto.InputTokenDetails{TextTokens: 7, ImageTokens: 896}},
		},
		{
			name:      "no usage bills the estimate",
			model:     "qwen3-vl-embedding",
			body:      `{"output":{"embeddings":[{"index":0,"embedding":[0.5],"type":"text"}]},"request_id":"r3"}`,
			estimate:  12,
			wantData:  []dto.EmbeddingResponseItem{{Object: "embedding", Index: 0, Embedding: []float64{0.5}}},
			wantUsage: &dto.Usage{PromptTokens: 12, TotalTokens: 12},
		},
		{
			name:  "error code in a 200 body",
			model: "qwen3-vl-embedding",
			body:  `{"code":"InvalidParameter","message":"The input is invalid.","request_id":"r4"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			info := &relaycommon.RelayInfo{
				RelayMode:   constant.RelayModeEmbeddings,
				RelayFormat: types.RelayFormatEmbedding,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tt.model},
			}
			info.SetEstimatePromptTokens(tt.estimate)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}

			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			if tt.wantUsage == nil {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
				assert.Equal(t, "InvalidParameter", string(apiErr.GetErrorCode()))
				assert.Empty(t, w.Body.String())
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tt.wantUsage, usage)

			wantJSON, err := common.Marshal(dto.EmbeddingResponse{Object: "list", Data: tt.wantData, Model: tt.model, Usage: *tt.wantUsage})
			require.NoError(t, err)
			assert.JSONEq(t, string(wantJSON), w.Body.String())
		})
	}
}

func TestMultimodalEmbeddingRejectsInvalidTokenAccounting(t *testing.T) {
	for _, usage := range []string{
		`{"input_tokens":-1,"image_tokens":1}`,
		`{"input_tokens":1,"image_tokens":-1}`,
		`{"input_tokens":9223372036854775807,"image_tokens":1}`,
		`{"input_tokens":3,"input_tokens_details":{"image_tokens":4}}`,
		`{"input_tokens":3,"input_tokens_details":{"text_tokens":4}}`,
		`{"input_tokens":3,"input_tokens_details":{"image_tokens":-1}}`,
		`{"input_tokens":3,"input_tokens_details":{"text_tokens":-1}}`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "qwen3-vl-embedding"}}
		info.SetEstimatePromptTokens(12)
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"output":{"embeddings":[{"index":0,"embedding":[0.1]}]},"usage":` + usage + `}`))}
		apiErr, gotUsage := MultimodalEmbeddingHandler(c, resp, info)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
		assert.Nil(t, gotUsage)
		assert.Empty(t, w.Body.String())
	}
}

func TestMultimodalEmbeddingRejectsEmptyResultsWithoutBilling(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"null response", `null`},
		{"empty object", `{}`},
		{"missing output with usage", `{"usage":{"input_tokens":10}}`},
		{"null output", `{"output":null}`},
		{"missing embeddings", `{"output":{}}`},
		{"null embeddings", `{"output":{"embeddings":null}}`},
		{"empty embeddings", `{"output":{"embeddings":[]},"usage":{"input_tokens":10}}`},
		{"missing vector", `{"output":{"embeddings":[{"index":0}]}}`},
		{"null embedding item", `{"output":{"embeddings":[null]}}`},
		{"null vector", `{"output":{"embeddings":[{"index":0,"embedding":null}]}}`},
		{"empty vector", `{"output":{"embeddings":[{"index":0,"embedding":[]}]}}`},
		{"mixed valid and empty vectors", `{"output":{"embeddings":[{"index":0,"embedding":[0.5]},{"index":1,"embedding":[]}]},"usage":{"input_tokens":10}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "qwen3-vl-embedding"}}
			info.SetEstimatePromptTokens(12)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}

			apiErr, usage := MultimodalEmbeddingHandler(c, resp, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
			assert.Nil(t, usage)
			assert.Empty(t, w.Body.String())
			assert.False(t, c.Writer.Written(), "invalid upstream results must not write a success response")
		})
	}
}

func TestMultimodalEmbeddingKeepsMappedModelPrivate(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-embedding",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "qwen3-vl-embedding-private", IsModelMapped: true},
	}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"output":{"embeddings":[{"index":0,"embedding":[0.5]}]},"usage":{"input_tokens":5}}`))}
	apiErr, usage := MultimodalEmbeddingHandler(c, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.PromptTokens)
	var got dto.EmbeddingResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "public-embedding", got.Model)
	assert.NotContains(t, w.Body.String(), "qwen3-vl-embedding-private")
}
