package dto

import (
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageProviderReasoningCounterSurvivesRawJSON(t *testing.T) {
	for _, raw := range []string{`{"prompt_tokens":86,"completion_tokens":417,"total_tokens":503,"reasoning_tokens":398}`, `{"prompt_tokens":86,"reasoning_tokens":0}`} {
		t.Run(raw, func(t *testing.T) {
			var u Usage
			require.NoError(t, kitutil.Unmarshal([]byte(raw), &u))
			encoded, err := kitutil.Marshal(u)
			require.NoError(t, err)
			var out map[string]any
			require.NoError(t, kitutil.Unmarshal(encoded, &out))
			var source map[string]any
			require.NoError(t, kitutil.Unmarshal([]byte(raw), &source))
			assert.Equal(t, source["reasoning_tokens"], out["reasoning_tokens"])
			billing := NewOpenAIChatBillingUsage(&u)
			require.NotNil(t, billing)
			wire, err := kitutil.Marshal(billing.OpenAIUsage)
			require.NoError(t, err)
			require.NoError(t, kitutil.Unmarshal(wire, &out))
			assert.Equal(t, source["reasoning_tokens"], out["reasoning_tokens"])
		})
	}
}

func TestUsageChatReasoningCounterPresencePriority(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      int
	}{
		{"standard", "{\"completion_tokens_details\":{\"reasoning_tokens\":17}}", 17},
		{"standard_zero_wins", "{\"completion_tokens_details\":{\"reasoning_tokens\":0},\"reasoning_tokens\":398}", 0},
		{"standard_positive_wins", "{\"completion_tokens_details\":{\"reasoning_tokens\":17},\"reasoning_tokens\":398}", 17},
		{"missing_standard_counter", "{\"completion_tokens_details\":{\"audio_tokens\":3},\"reasoning_tokens\":398}", 398},
		{"standard_null_fallback", "{\"completion_tokens_details\":{\"reasoning_tokens\":null},\"reasoning_tokens\":398}", 398},
		{"details_null_fallback", "{\"completion_tokens_details\":null,\"reasoning_tokens\":398}", 398},
		{"top_zero", "{\"reasoning_tokens\":0}", 0},
		{"top_null", "{\"reasoning_tokens\":null}", 0},
		{"missing", "{}", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var u Usage
			require.NoError(t, kitutil.Unmarshal([]byte(tc.raw), &u))
			assert.Equal(t, tc.want, u.ChatReasoningTokens())
		})
	}
	var nilUsage *Usage
	assert.Zero(t, nilUsage.ChatReasoningTokens())
}
func TestUsageReasoningWrongTypesRemainInvalid(t *testing.T) {
	for _, raw := range []string{`{"reasoning_tokens":"17"}`, `{"reasoning_tokens":{}}`, `{"completion_tokens_details":{"reasoning_tokens":"17"}}`, `{"completion_tokens_details":{"reasoning_tokens":true}}`} {
		var u Usage
		assert.Error(t, kitutil.Unmarshal([]byte(raw), &u))
	}
}
func TestReasoningUsageDoesNotInterceptAnonymousResponseEnvelope(t *testing.T) {
	var response SimpleResponse
	require.NoError(t, kitutil.Unmarshal([]byte(`{"usage":{"prompt_tokens":86,"completion_tokens":417,"total_tokens":503,"reasoning_tokens":398,"completion_tokens_details":{"audio_tokens":3,"text_tokens":4,"image_tokens":5}},"error":{"code":"SOURCE_CODE"}}`), &response))
	assert.Equal(t, 503, response.Usage.TotalTokens)
	assert.Equal(t, 398, response.Usage.ChatReasoningTokens())
	assert.Equal(t, 3, response.Usage.CompletionTokenDetails.AudioTokens)
	assert.Equal(t, 4, response.Usage.CompletionTokenDetails.TextTokens)
	assert.Equal(t, 5, response.Usage.CompletionTokenDetails.ImageTokens)
	require.NotNil(t, response.Error)
}
func TestBillingReasoningCountersDoNotAliasCanonicalSnapshots(t *testing.T) {
	var source Usage
	require.NoError(t, kitutil.Unmarshal([]byte(`{"prompt_tokens":86,"completion_tokens":417,"total_tokens":503,"reasoning_tokens":398,"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":3},"usage_source":"original","cost":0.25}`), &source))
	snapshot := NewOpenAIChatBillingUsage(&source)
	require.NotNil(t, snapshot)
	clone := CloneBillingUsage(snapshot)
	require.NotNil(t, clone)
	*source.ReasoningTokens = 999
	*clone.OpenAIUsage.ReasoningTokens = 888
	clone.OpenAIUsage.CompletionTokenDetails.AudioTokens = 9
	assert.Equal(t, 398, *snapshot.OpenAIUsage.ReasoningTokens)
	assert.Zero(t, snapshot.OpenAIUsage.ChatReasoningTokens())
	assert.Zero(t, clone.OpenAIUsage.ChatReasoningTokens())
	assert.Equal(t, 3, snapshot.OpenAIUsage.CompletionTokenDetails.AudioTokens)
	assert.Equal(t, 503, snapshot.OpenAIUsage.TotalTokens)
	assert.Equal(t, "original", snapshot.OpenAIUsage.UsageSource)
	assert.Equal(t, 0.25, snapshot.OpenAIUsage.Cost)
	assert.Equal(t, BillingUsageSourceOAIChat, clone.Source)
}
