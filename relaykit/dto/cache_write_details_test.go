package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheWriteLifetimeJSONAndBillingClone(t *testing.T) {
	var usage Usage
	require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":1000,"input_tokens_details":{"cache_write_tokens":300,"cache_write_tokens_details":{"ttl_30m_tokens":100,"ttl_12h_tokens":200}}}`), &usage))
	assert.Equal(t, 200, usage.InputTokensDetails.CacheWrite12hTokens())
	billing := NewOpenAIResponsesBillingUsage(&usage)
	require.NotNil(t, billing)
	usage.InputTokensDetails.CacheWriteTokensDetails.TTL12hTokens = 1
	assert.Equal(t, 200, billing.OpenAIUsage.InputTokensDetails.CacheWrite12hTokens())
	clone := CloneBillingUsage(billing)
	billing.OpenAIUsage.InputTokensDetails.CacheWriteTokensDetails.TTL12hTokens = 2
	assert.Equal(t, 200, clone.OpenAIUsage.InputTokensDetails.CacheWrite12hTokens())
	encoded, err := json.Marshal(clone.OpenAIUsage)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"ttl_12h_tokens":200`)
}

func TestCacheWriteLifetimeCannotExceedTotalOrBecomeNegative(t *testing.T) {
	for _, tc := range []struct {
		name              string
		total, long, want int
	}{
		{"negative total", -1, 20, 0}, {"negative subset", 30, -5, 0}, {"excess subset", 30, 40, 30}, {"partial subset", 30, 20, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			details := InputTokenDetails{CacheWriteTokens: tc.total, CacheWriteTokensDetails: &CacheWriteTokenDetails{TTL12hTokens: tc.long}}
			assert.Equal(t, tc.want, details.CacheWrite12hTokens())
		})
	}
}
