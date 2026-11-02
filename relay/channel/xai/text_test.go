package xai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamUsagePreservesTokenDetailsForBilling(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	const usageJSON = `{"prompt_tokens":100,"completion_tokens":0,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":60,"cache_write_tokens":10,"audio_tokens":3},"completion_tokens_details":{"reasoning_tokens":8,"audio_tokens":2}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-test",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "grok-test"},
	}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: {\"id\":\"test\",\"model\":\"grok-test\",\"choices\":[],\"usage\":" + usageJSON + "}\n\ndata: [DONE]\n\n"))}
	usage, apiErr := xAIStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	var expected dto.Usage
	require.NoError(t, common.UnmarshalJsonStr(usageJSON, &expected))
	expected.CompletionTokens = 20
	assert.Equal(t, &expected, usage, "billing must receive the same token details as the upstream stream")
	line := strings.SplitN(w.Body.String(), "\n", 2)[0]
	require.True(t, strings.HasPrefix(line, "data: "))
	var forwarded dto.ChatCompletionsStreamResponse
	require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &forwarded))
	assert.Equal(t, &expected, forwarded.Usage, "clients and settlement must see the same converted usage")
}
