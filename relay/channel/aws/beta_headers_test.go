package aws

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBedrockBetaListPreservesLinesAndTrimsEmptyEntries(t *testing.T) {
	headers := make(http.Header)
	headers.Add("anthropic-beta", " computer-use-2025-01-24, ,context-1m-2025-08-07 ")
	headers.Add("anthropic-beta", " interleaved-thinking-2025-05-14 ")
	request, err := formatRequest(strings.NewReader(`{"max_tokens":128,"messages":[]}`), headers)
	require.NoError(t, err)
	var betas []string
	require.NoError(t, common.Unmarshal(request.AnthropicBeta, &betas))
	assert.Equal(t, []string{"computer-use-2025-01-24", "context-1m-2025-08-07", "interleaved-thinking-2025-05-14"}, betas)

	request, err = formatRequest(strings.NewReader(`{"max_tokens":128,"messages":[]}`), http.Header{})
	require.NoError(t, err)
	assert.Empty(t, request.AnthropicBeta)
}
