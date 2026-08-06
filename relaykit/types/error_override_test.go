package types

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The user message override contract: tagging an error must not change what
// internal consumers (error logs, auto-ban, violation-fee detection, retry
// decisions) read; only ApplyUserMessageOverride rewrites the user-visible
// message across every response format.
func TestUserMessageOverrideTagDoesNotChangeOriginalError(t *testing.T) {
	t.Parallel()

	err := WithOpenAIError(OpenAIError{
		Message: "upstream quota exceeded for key sk-abc",
		Type:    "insufficient_quota",
		Code:    "insufficient_quota",
	}, http.StatusTooManyRequests)

	err.SetUserMessageOverride("渠道繁忙，请稍后重试")

	require.Equal(t, "upstream quota exceeded for key sk-abc", err.Error())
	assert.Equal(t, "upstream quota exceeded for key sk-abc", err.ToOpenAIError().Message)
	assert.Equal(t, "upstream quota exceeded for key sk-abc", err.ToClaudeError().Message)

	overrideText, ok := err.UserMessageOverride()
	require.True(t, ok)
	assert.Equal(t, "渠道繁忙，请稍后重试", overrideText)
}

func TestApplyUserMessageOverrideRewritesAllFormats(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *NewAPIError
	}{
		{
			name: "upstream openai error",
			err: WithOpenAIError(OpenAIError{
				Message:  "bad response status code 500, body: internal error at https://api.openai.com",
				Type:     "server_error",
				Code:     "server_error",
				Metadata: json.RawMessage(`{"provider":"openai"}`),
			}, http.StatusInternalServerError),
		},
		{
			name: "upstream claude error",
			err: WithClaudeError(ClaudeError{
				Type:    "overloaded_error",
				Message: "Anthropic servers are overloaded",
			}, http.StatusServiceUnavailable),
		},
		{
			name: "local-shaped channel error",
			err:  NewError(errors.New("do request failed: dial tcp 1.2.3.4:443"), ErrorCodeDoRequestFailed),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.err.ApplyUserMessageOverride("服务暂不可用 (request id: req-1)")

			assert.Equal(t, "服务暂不可用 (request id: req-1)", tc.err.Error())
			assert.Equal(t, "服务暂不可用 (request id: req-1)", tc.err.ToOpenAIError().Message)
			assert.Equal(t, "服务暂不可用 (request id: req-1)", tc.err.ToClaudeError().Message)
			assert.Empty(t, tc.err.ToOpenAIError().Metadata, "upstream metadata must not leak through override")
		})
	}
}

// Upstream error codes are as identifying as the message text (e.g. a pool
// upstream returning code "no_available_accounts"); override must neutralize
// type/code/param on upstream-sourced errors.
func TestApplyUserMessageOverrideNeutralizesUpstreamCodes(t *testing.T) {
	t.Parallel()

	err := WithOpenAIError(OpenAIError{
		Message: "No available OAuth accounts in pool",
		Type:    "server_error",
		Param:   "oauth_pool",
		Code:    "no_available_accounts",
	}, http.StatusServiceUnavailable)

	err.ApplyUserMessageOverride("服务繁忙，请稍后重试")

	oai := err.ToOpenAIError()
	assert.Equal(t, "服务繁忙，请稍后重试", oai.Message)
	assert.Equal(t, string(ErrorTypeUpstreamError), oai.Type)
	assert.Equal(t, string(ErrorTypeUpstreamError), oai.Code)
	assert.Empty(t, oai.Param)
	claude := err.ToClaudeError()
	assert.Equal(t, string(ErrorTypeUpstreamError), claude.Type)
}

// Fixed override text is admin-authored and may intentionally contain a URL
// (e.g. a status page); it must not be mangled by sensitive-info masking.
func TestApplyUserMessageOverrideSkipsSensitiveMasking(t *testing.T) {
	t.Parallel()

	err := WithOpenAIError(OpenAIError{
		Message: "raw upstream error",
		Type:    "server_error",
	}, http.StatusInternalServerError)

	err.ApplyUserMessageOverride("服务维护中，详情见 https://status.example.com")

	assert.Equal(t, "服务维护中，详情见 https://status.example.com", err.ToOpenAIError().Message)
	assert.Equal(t, "服务维护中，详情见 https://status.example.com", err.ToClaudeError().Message)
}
