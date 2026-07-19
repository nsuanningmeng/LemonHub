package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// TieredResultWrapper wraps billingexpr.TieredResult for use at the service layer.
type TieredResultWrapper = billingexpr.TieredResult

// BuildTieredTokenParams constructs billingexpr.TokenParams from a dto.Usage,
// normalizing P and C so they mean "tokens not separately priced by the
// expression". Sub-categories (cache, image, audio) are only subtracted
// when the expression references them via their own variable.
//
// GPT-format APIs report prompt_tokens / completion_tokens as totals that
// include all sub-categories (cache, image, audio). Claude-format APIs
// report them as text-only. This function normalizes to text-only when
// sub-categories are separately priced.
func BuildTieredTokenParams(usage *dto.Usage, isClaudeUsageSemantic bool, usedVars map[string]bool) billingexpr.TokenParams {
	// Every count below is upstream-controlled (parsed from provider JSON as a
	// signed int). A negative count in the expression env could drive the cost
	// negative and turn settlement into a credit, so floor them all at zero.
	p := nonNegativeTokenCount(usage.PromptTokens)
	c := nonNegativeTokenCount(usage.CompletionTokens)
	cr := nonNegativeTokenCount(usage.PromptTokensDetails.CachedTokens)
	cc5m := nonNegativeTokenCount(usage.PromptTokensDetails.CacheCreationTokensTotal())
	cc1h := float64(0)

	if usage.UsageSemantic == "anthropic" {
		cc1h = nonNegativeTokenCount(usage.ClaudeCacheCreation1hTokens)
		cc5m = nonNegativeTokenCount(usage.ClaudeCacheCreation5mTokens)
	}

	img := nonNegativeTokenCount(usage.PromptTokensDetails.ImageTokens)
	ai := nonNegativeTokenCount(usage.PromptTokensDetails.AudioTokens)
	imgO := nonNegativeTokenCount(usage.CompletionTokenDetails.ImageTokens)
	ao := nonNegativeTokenCount(usage.CompletionTokenDetails.AudioTokens)

	// len = total input context length for tier condition evaluation.
	// Non-Claude: prompt_tokens already includes everything.
	// Claude: input_tokens is text-only, so add cache read + cache creation.
	inputLen := p
	if isClaudeUsageSemantic {
		inputLen = p + cr + cc5m + cc1h
	}

	if !isClaudeUsageSemantic {
		if usedVars["cr"] {
			p -= cr
		}
		if usedVars["cc"] {
			p -= cc5m
		}
		if usedVars["cc1h"] {
			p -= cc1h
		}
		if usedVars["img"] {
			p -= img
		}
		if usedVars["ai"] {
			p -= ai
		}
		if usedVars["img_o"] {
			c -= imgO
		}
		if usedVars["ao"] {
			c -= ao
		}
	}

	// OpenAI cache-write usage reports unadjusted prefix counts, so cr + cc can
	// exceed the prompt and drive the remainder negative. Clamp at zero.
	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}

	return billingexpr.TokenParams{
		P:    p,
		C:    c,
		Len:  inputLen,
		CR:   cr,
		CC:   cc5m,
		CC1h: cc1h,
		Img:  img,
		ImgO: imgO,
		AI:   ai,
		AO:   ao,
	}
}

// TryTieredSettle checks if the request uses tiered_expr billing and, if so,
// computes the actual quota using the frozen BillingSnapshot. Returns:
//   - ok=true, quota, result  when tiered billing applies
//   - ok=false, 0, nil        when it doesn't (caller should fall through to existing logic)
func TryTieredSettle(relayInfo *relaycommon.RelayInfo, params billingexpr.TokenParams) (ok bool, quota int, result *billingexpr.TieredResult) {
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil || snap.BillingMode != "tiered_expr" {
		return false, 0, nil
	}

	requestInput := billingexpr.RequestInput{}
	if relayInfo.BillingRequestInput != nil {
		requestInput = *relayInfo.BillingRequestInput
	}

	tr, err := billingexpr.ComputeTieredQuotaWithRequest(snap, params, requestInput)
	if err != nil {
		quota = relayInfo.FinalPreConsumedQuota
		if quota <= 0 {
			quota = snap.EstimatedQuotaAfterGroup
		}
		return true, quota, nil
	}

	// Surface any int32 saturation from settlement onto RelayInfo so the
	// consume log records it under admin_info, regardless of which caller
	// (text, audio, WSS) consumes the returned quota. First non-nil wins.
	noteQuotaClamp(relayInfo, tr.Clamp)

	// A settlement charge is never a credit: refunds happen only through the
	// actual-vs-preconsumed delta, so a negative expression result must not
	// leave here (it would credit the account in SettleBilling).
	if tr.ActualQuotaAfterGroup < 0 {
		common.SysError(fmt.Sprintf("tiered settle produced negative quota %d for model %s, flooring to 0", tr.ActualQuotaAfterGroup, relayInfo.OriginModelName))
		tr.ActualQuotaAfterGroup = 0
	}

	return true, tr.ActualQuotaAfterGroup, &tr
}

// nonNegativeTokenCount floors an upstream-reported token count at zero before
// it can reach a billing expression; counts are parsed from provider JSON as
// signed ints and a negative value could turn the computed cost into a credit.
func nonNegativeTokenCount(n int) float64 {
	if n < 0 {
		return 0
	}
	return float64(n)
}
