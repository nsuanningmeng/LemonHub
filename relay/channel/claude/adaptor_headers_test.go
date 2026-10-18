package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeHeadersPreserveEveryClientBetaValue(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Add("anthropic-beta", "computer-use-2025-01-24")
	c.Request.Header.Add("anthropic-beta", "context-1m-2025-08-07,interleaved-thinking-2025-05-14")
	info := &relaycommon.RelayInfo{OriginModelName: "claude-sonnet-4-5", ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "fixture-key"}}
	headers := make(http.Header)
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &headers, info))
	assert.Equal(t, "computer-use-2025-01-24,context-1m-2025-08-07,interleaved-thinking-2025-05-14", headers.Get("anthropic-beta"))

	c.Request.Header.Del("anthropic-beta")
	headers = make(http.Header)
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &headers, info))
	assert.Empty(t, headers.Get("anthropic-beta"), "do not opt clients into any beta")
}
