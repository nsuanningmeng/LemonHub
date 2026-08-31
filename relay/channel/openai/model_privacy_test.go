package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mappedOpenAIResponseInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "client-model",
		RelayFormat:     types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "private-upstream-model",
			IsModelMapped:     true,
		},
	}
}

func TestOpenAIModelMappingIsHiddenInNonStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := mappedOpenAIResponseInfo()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"private-upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3},"provider_extension":true}`,
		)),
	}

	usage, apiErr := OpenaiHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), `"model":"client-model"`)
	assert.NotContains(t, recorder.Body.String(), "private-upstream-model")
	assert.Contains(t, recorder.Body.String(), `"provider_extension":true`)
	assert.Equal(t, "private-upstream-model", info.UpstreamModelName)
}

func TestOpenAIModelMappingIsHiddenInStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := mappedOpenAIResponseInfo()
	chunk := dto.ChatCompletionsStreamResponse{
		Id:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 1,
		Model:   "private-upstream-model",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{Index: 0}},
	}
	data, err := common.Marshal(chunk)
	require.NoError(t, err)

	require.NoError(t, sendStreamData(c, info, string(data), false, false))

	assert.Contains(t, recorder.Body.String(), `"model":"client-model"`)
	assert.NotContains(t, recorder.Body.String(), "private-upstream-model")
	assert.Equal(t, "private-upstream-model", info.UpstreamModelName)
}
