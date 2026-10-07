package service

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// TextBillingExemptionReason is shared by request settlement and channel-test
// estimates. Only the Claude parser can supply the non-serialized evidence;
// an admin rejection string or zero output count alone cannot waive a charge.
func TextBillingExemptionReason(usage *dto.Usage) string {
	if !model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled || usage == nil {
		return ""
	}
	billing := usage.BillingUsage
	if billing == nil || !billing.ClaudePreOutputRefusal || billing.Estimated ||
		billing.Source != dto.BillingUsageSourceClaudeMessages || billing.Semantic != dto.BillingUsageSemanticAnthropic ||
		billing.ClaudeUsage == nil || billing.ClaudeUsage.OutputTokens != 0 {
		return ""
	}
	return "claude_pre_output_refusal"
}
