package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mappedNativeGeminiInfo(isStream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "client-model",
		IsStream:        isStream,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "private-upstream-model",
			IsModelMapped:     true,
		},
	}
}

func TestNativeGeminiModelMappingIsHiddenInResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	payload := `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2},"modelVersion":"private-upstream-model"}`

	t.Run("non-stream", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/client-model:generateContent", nil)
		info := mappedNativeGeminiInfo(false)
		resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload))}

		usage, apiErr := GeminiTextGenerationHandler(c, info, resp)

		require.Nil(t, apiErr)
		require.NotNil(t, usage)
		assert.Contains(t, recorder.Body.String(), `"modelVersion":"client-model"`)
		assert.NotContains(t, recorder.Body.String(), "private-upstream-model")
		assert.Equal(t, "private-upstream-model", info.UpstreamModelName)
	})

	t.Run("stream", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/client-model:streamGenerateContent", nil)
		info := mappedNativeGeminiInfo(true)
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n")),
		}

		usage, apiErr := GeminiTextGenerationStreamHandler(c, info, resp)

		require.Nil(t, apiErr)
		require.NotNil(t, usage)
		assert.Contains(t, recorder.Body.String(), `"modelVersion":"client-model"`)
		assert.NotContains(t, recorder.Body.String(), "private-upstream-model")
		assert.Equal(t, "private-upstream-model", info.UpstreamModelName)
	})
}
