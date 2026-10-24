package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheLifetimeExpressionsPreserveFallbackPricesAndContextLength(t *testing.T) {
	for _, tc := range []struct {
		name, expr   string
		anthropic    bool
		usage        dto.Usage
		cost, length float64
	}{
		{name: "openai explicit 12h", expr: "p + cr*0.1 + cc*1.25 + cc12h*2.5", usage: dto.Usage{PromptTokens: 1000, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100, CacheWriteTokens: 300, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL30mTokens: 100, TTL12hTokens: 200}}}, cost: 1235, length: 1000},
		{name: "openai fallback to short price", expr: "p + cr*0.1 + cc*1.25", usage: dto.Usage{PromptTokens: 1000, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100, CacheWriteTokens: 300, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL12hTokens: 200}}}, cost: 1235, length: 1000},
		{name: "openai fallback to input", expr: "p + cr*0.1", usage: dto.Usage{PromptTokens: 1000, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100, CacheWriteTokens: 300, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL12hTokens: 200}}}, cost: 1110, length: 1000},
		{name: "openai explicit 12h only", expr: "p + cc12h*2.5", usage: dto.Usage{PromptTokens: 1000, PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 300, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL12hTokens: 200}}}, cost: 1300, length: 1000},
		{name: "claude missing short split", anthropic: true, expr: "p + cr*0.1 + cc*1.25 + cc1h*2", usage: dto.Usage{PromptTokens: 100, UsageSemantic: "anthropic", PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200, CachedCreationTokens: 300}, ClaudeCacheCreation5mTokens: 50, ClaudeCacheCreation1hTokens: 100}, cost: 570, length: 600},
		{name: "claude total only", anthropic: true, expr: "p + cr*0.1 + cc*1.25", usage: dto.Usage{PromptTokens: 100, UsageSemantic: "anthropic", PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200, CachedCreationTokens: 300}}, cost: 495, length: 600},
		{name: "claude long write short fallback", anthropic: true, expr: "p + cr*0.1 + cc*1.25", usage: dto.Usage{PromptTokens: 100, UsageSemantic: "anthropic", PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200, CachedCreationTokens: 300}, ClaudeCacheCreation1hTokens: 100}, cost: 570, length: 600},
		{name: "claude all cache input fallback", anthropic: true, expr: "p", usage: dto.Usage{PromptTokens: 100, UsageSemantic: "anthropic", PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200, CachedCreationTokens: 300}, ClaudeCacheCreation1hTokens: 100}, cost: 660, length: 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usedVars := billingexpr.UsedVars(tc.expr)
			params := BuildTieredTokenParams(&tc.usage, tc.anthropic, usedVars)
			cost, _, err := billingexpr.RunExpr(tc.expr, params)
			require.NoError(t, err)
			assert.InDelta(t, tc.cost, cost, 0.000001)
			assert.Equal(t, tc.length, params.Len)
		})
	}
}

func TestOpenAICacheLifetimeRatioUsesBoundedSubset(t *testing.T) {
	for _, tc := range []struct {
		name string
		long int
		want int
	}{
		{"mixed", 200, 1235}, {"negative subset ignored", -1, 985}, {"subset capped at total", 400, 1360},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: "cache-test", StartTime: time.Now(), PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1, CacheCreationRatio: 1.25, CacheCreation12hRatio: 2.5, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			usage := &dto.Usage{PromptTokens: 1000, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100, CacheWriteTokens: 300, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL12hTokens: tc.long}}}
			summary := calculateTextQuotaSummary(taskBillingTestContext(), info, usage)
			assert.Equal(t, tc.want, summary.Quota)
		})
	}
}

func TestResponsesBillingEnvelopeRetainsDetachedCacheLifetime(t *testing.T) {
	original := &dto.Usage{InputTokens: 1000, InputTokensDetails: &dto.InputTokenDetails{CacheWriteTokens: 300, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL12hTokens: 200}}}
	billing := dto.NewOpenAIResponsesBillingUsage(original)
	require.NotNil(t, billing)
	original.InputTokensDetails.CacheWriteTokensDetails.TTL12hTokens = 1
	usage := usageFromOpenAIBillingUsage(billing)
	assert.Equal(t, 200, usage.PromptTokensDetails.CacheWrite12hTokens())
	usage.PromptTokensDetails.CacheWriteTokensDetails.TTL12hTokens = 2
	assert.Equal(t, 200, billing.OpenAIUsage.InputTokensDetails.CacheWrite12hTokens())
}
