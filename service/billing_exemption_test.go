package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestTextBillingExemptionRequiresTrustedClaudeSnapshot(t *testing.T) {
	old := *model_setting.GetGlobalSettings()
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = true
	t.Cleanup(func() { *model_setting.GetGlobalSettings() = old })
	tests := []struct {
		name     string
		billing  *dto.BillingUsage
		expected string
	}{
		{"no evidence", nil, ""},
		{"zero output alone", dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{InputTokens: 412}), ""},
		{"wrong provider", &dto.BillingUsage{ClaudePreOutputRefusal: true, Source: dto.BillingUsageSourceOAIChat, Semantic: dto.BillingUsageSemanticAnthropic, ClaudeUsage: &dto.ClaudeUsage{}}, ""},
		{"wrong semantics", &dto.BillingUsage{ClaudePreOutputRefusal: true, Source: dto.BillingUsageSourceClaudeMessages, Semantic: dto.BillingUsageSemanticOpenAI, ClaudeUsage: &dto.ClaudeUsage{}}, ""},
		{"estimated usage", &dto.BillingUsage{ClaudePreOutputRefusal: true, Estimated: true, Source: dto.BillingUsageSourceClaudeMessages, Semantic: dto.BillingUsageSemanticAnthropic, ClaudeUsage: &dto.ClaudeUsage{}}, ""},
		{"contradictory output", &dto.BillingUsage{ClaudePreOutputRefusal: true, Source: dto.BillingUsageSourceClaudeMessages, Semantic: dto.BillingUsageSemanticAnthropic, ClaudeUsage: &dto.ClaudeUsage{OutputTokens: 1}}, ""},
		{"complete evidence", &dto.BillingUsage{ClaudePreOutputRefusal: true, Source: dto.BillingUsageSourceClaudeMessages, Semantic: dto.BillingUsageSemanticAnthropic, ClaudeUsage: &dto.ClaudeUsage{InputTokens: 412}}, "claude_pre_output_refusal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, TextBillingExemptionReason(&dto.Usage{BillingUsage: tt.billing}))
		})
	}
	model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = false
	assert.Empty(t, TextBillingExemptionReason(&dto.Usage{BillingUsage: tests[len(tests)-1].billing}), "default policy retains upstream-compatible billing")
}
