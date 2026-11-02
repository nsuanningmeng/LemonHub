package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplicitCacheCreationTokensPreserveBillingAndWireUsage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details string
		want    int
	}{
		{"explicit only", `"cache_creation_input_tokens":300`, 300},
		{"explicit alias larger", `"cache_write_tokens":250,"cache_creation_input_tokens":400`, 400},
		{"native alias larger", `"cache_write_tokens":400,"cache_creation_input_tokens":250`, 400},
		{"legacy alias larger", `"cached_creation_tokens":400,"cache_creation_input_tokens":250`, 400},
		{"negative aliases", `"cached_creation_tokens":-2,"cache_write_tokens":-3,"cache_creation_input_tokens":-1`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage Usage
			require.NoError(t, kitutil.Unmarshal([]byte(`{"prompt_tokens_details":{`+tc.details+`}}`), &usage))
			assert.Equal(t, tc.want, usage.PromptTokensDetails.CacheCreationTokensTotal())
			// A usage packet containing only this provider detail still has
			// authoritative accounting evidence and must survive cloning.
			billing := NewOpenAIChatBillingUsage(&usage)
			require.NotNil(t, billing)
			require.NotNil(t, billing.OpenAIUsage)
			assert.Equal(t, usage.PromptTokensDetails, billing.OpenAIUsage.PromptTokensDetails)
			encoded, err := kitutil.Marshal(billing.OpenAIUsage)
			require.NoError(t, err)
			var roundTrip Usage
			require.NoError(t, kitutil.Unmarshal(encoded, &roundTrip))
			assert.Equal(t, usage.PromptTokensDetails, roundTrip.PromptTokensDetails)
			assert.Zero(t, roundTrip.ClaudeCacheCreation5mTokens)
			assert.Zero(t, roundTrip.ClaudeCacheCreation1hTokens)
		})
	}
	encoded, err := kitutil.Marshal(InputTokenDetails{})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "cache_creation_input_tokens")
}
