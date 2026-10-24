package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamPreservesCacheWriteLifetimeInBilling(t *testing.T) {
	usage, _, _, body := runResponsesUsageStream(t, `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1000,"output_tokens":1,"total_tokens":1001,"input_tokens_details":{"cache_write_tokens":300,"cache_write_tokens_details":{"ttl_30m_tokens":100,"ttl_12h_tokens":200}}}}}`)
	assert.Equal(t, 200, usage.PromptTokensDetails.CacheWrite12hTokens())
	require.NotNil(t, usage.BillingUsage)
	assert.Equal(t, 200, usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CacheWrite12hTokens())
	assert.Contains(t, body, `"ttl_12h_tokens":200`)
}

func TestResponsesCompactionPreservesCacheWriteLifetime(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1000,"output_tokens":1,"total_tokens":1001,"input_tokens_details":{"cache_write_tokens":300,"cache_write_tokens_details":{"ttl_12h_tokens":200}}}}`))}
	usage, apiErr := OaiResponsesCompactionHandler(c, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 200, usage.PromptTokensDetails.CacheWrite12hTokens())
}
