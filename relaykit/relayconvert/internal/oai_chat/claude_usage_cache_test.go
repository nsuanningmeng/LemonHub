package oaichat

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaudeResponseUsageSeparatesInclusiveCacheRead(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		prompt, read, write, creation, want int
	}{
		{"read only", 10, 4, 0, 0, 6},
		{"read and native write", 10, 4, 2, 0, 4},
		{"legacy creation remains additive", 10, 4, 0, 2, 6},
		{"no cache detail", 10, 0, 0, 0, 10},
		{"explicit zero cache", 10, 0, 0, 0, 10},
		{"cache exceeds prompt", 3, 4, 0, 0, 0},
		{"combined cache exceeds prompt", 5, 4, 3, 0, 0},
		{"zero usage", 0, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := &dto.Usage{PromptTokens: tc.prompt, CompletionTokens: 3, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: tc.read, CacheWriteTokens: tc.write, CachedCreationTokens: tc.creation}}
			converted := buildClaudeUsageFromOpenAIUsage(usage)
			require.NotNil(t, converted)
			assert.Equal(t, tc.want, converted.InputTokens)
			assert.Equal(t, tc.read, converted.CacheReadInputTokens)
			assert.Equal(t, 3, converted.OutputTokens)
			assert.Equal(t, tc.prompt, usage.PromptTokens, "canonical usage is not mutated")
		})
	}
	metadata := &dto.GeminiUsageMetadata{PromptTokenCount: 10, CachedContentTokenCount: 4, CandidatesTokenCount: 3}
	usage := &dto.Usage{PromptTokens: 10, CompletionTokens: 3, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 4}, BillingUsage: dto.NewGeminiChatBillingUsage(metadata)}
	converted := buildClaudeUsageFromOpenAIUsage(usage)
	require.NotNil(t, converted.BillingUsage)
	assert.Equal(t, dto.BillingUsageSourceGeminiChat, converted.BillingUsage.Source)
	assert.Equal(t, dto.BillingUsageSemanticGemini, converted.BillingUsage.Semantic)
	require.NotNil(t, converted.BillingUsage.GeminiUsageMetadata)
	converted.BillingUsage.GeminiUsageMetadata.PromptTokenCount = 99
	assert.Equal(t, 10, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount, "snapshot is cloned")
	native := &dto.ClaudeUsage{InputTokens: 6, CacheReadInputTokens: 4, OutputTokens: 3}
	existing := &dto.Usage{PromptTokens: 6, CompletionTokens: 3, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 4}, BillingUsage: dto.NewClaudeMessagesBillingUsage(native)}
	assert.Equal(t, 6, buildClaudeUsageFromOpenAIUsage(existing).InputTokens, "native Anthropic snapshot is already fresh and must not be subtracted again")
}
