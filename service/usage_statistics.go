package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// UsageInputTokensForStatistics returns inclusive input without changing the
// provider's raw usage or the fresh/cache counters used for billing. A nil
// result preserves the log writer's legacy fallback; an explicit zero is known.
func UsageInputTokensForStatistics(info *relaycommon.RelayInfo, usage *dto.Usage) *int64 {
	if usage == nil {
		return nil
	}
	var input int64
	if snapshot := usage.BillingUsage; snapshot != nil {
		source := strings.TrimSpace(snapshot.Source)
		semantic := strings.TrimSpace(snapshot.Semantic)
		switch {
		case snapshot.OpenAIUsage != nil && (strings.EqualFold(source, dto.BillingUsageSourceOAIChat) ||
			strings.EqualFold(source, dto.BillingUsageSourceOAIResponses) || strings.EqualFold(semantic, dto.BillingUsageSemanticOpenAI)):
			raw := snapshot.OpenAIUsage
			prompt := raw.PromptTokens
			if prompt == 0 && raw.InputTokens > 0 {
				prompt = raw.InputTokens
			}
			input = common.SumTokenCountsForStatistics(int64(prompt))
			return &input
		case snapshot.ClaudeUsage != nil && (strings.EqualFold(source, dto.BillingUsageSourceClaudeMessages) ||
			strings.EqualFold(semantic, dto.BillingUsageSemanticAnthropic)):
			raw := snapshot.ClaudeUsage
			cache5m, cache1h := raw.GetCacheCreation5mTokens(), raw.GetCacheCreation1hTokens()
			if cache5m == 0 {
				cache5m = raw.ClaudeCacheCreation5mTokens
			}
			if cache1h == 0 {
				cache1h = raw.ClaudeCacheCreation1hTokens
			}
			write := max(common.SumTokenCountsForStatistics(int64(raw.CacheCreationInputTokens)),
				common.SumTokenCountsForStatistics(int64(cache5m), int64(cache1h)))
			input = common.SumTokenCountsForStatistics(int64(raw.InputTokens), int64(raw.CacheReadInputTokens), write)
			return &input
		case snapshot.GeminiUsageMetadata != nil && (strings.EqualFold(source, dto.BillingUsageSourceGeminiChat) ||
			strings.EqualFold(semantic, dto.BillingUsageSemanticGemini)):
			raw := snapshot.GeminiUsageMetadata
			input = common.SumTokenCountsForStatistics(int64(raw.PromptTokenCount), int64(raw.ToolUsePromptTokenCount))
			return &input
		}
	}

	// OpenRouter's Anthropic-labelled chat usage is still inclusive. Billing
	// subtracts its cache components later; statistics must keep the original sum.
	inclusiveOpenRouter := info != nil && info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeOpenRouter
	anthropic := usageSemanticFromUsage(info, usage) == dto.BillingUsageSemanticAnthropic ||
		usage.UsageSource == dto.BillingUsageSourceClaudeMessages || isLegacyClaudeDerivedOpenAIUsage(info, usage)
	if anthropic && !inclusiveOpenRouter {
		write := max(common.SumTokenCountsForStatistics(int64(usage.PromptTokensDetails.CacheCreationTokensTotal())),
			common.SumTokenCountsForStatistics(int64(usage.ClaudeCacheCreation5mTokens), int64(usage.ClaudeCacheCreation1hTokens)))
		input = common.SumTokenCountsForStatistics(int64(usage.PromptTokens), int64(usage.PromptTokensDetails.CachedTokens), write)
	} else {
		prompt := usage.PromptTokens
		if prompt == 0 && usage.InputTokens > 0 {
			prompt = usage.InputTokens
		}
		input = common.SumTokenCountsForStatistics(int64(prompt))
	}
	return &input
}
