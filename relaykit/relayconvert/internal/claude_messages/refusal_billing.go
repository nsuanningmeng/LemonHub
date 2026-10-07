package claudemessages

import (
	"math"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
)

// Evidence belongs to one parsed response, never to the reusable request
// context: a retry cannot inherit an earlier attempt's refusal.
type claudeRefusalEvidence struct {
	started, delta, stopped bool
	refusal, invalid        bool
}

// JSON readers can disagree about duplicate keys (including fields absent
// from our DTO). Ambiguous billing evidence must never waive a charge.
func hasUniqueRefusalFields(root gjson.Result) bool {
	for _, path := range []string{"", "message", "usage", "message.usage", "delta",
		"usage.server_tool_use", "usage.cache_creation", "message.usage.server_tool_use", "message.usage.cache_creation"} {
		object := root
		if path != "" {
			object = root.Get(path)
		}
		if !object.IsObject() {
			continue
		}
		seen := make(map[string]bool)
		unique := true
		object.ForEach(func(key, _ gjson.Result) bool {
			if seen[key.Str] {
				unique = false
				return false
			}
			seen[key.Str] = true
			return true
		})
		if !unique {
			return false
		}
	}
	return true
}

// validRefusalUsage distinguishes an explicit zero from an absent usage field.
// Cache counts remain reference data; tool/fallback work prevents exemption.
func validRefusalUsage(usage gjson.Result, parsed *dto.ClaudeUsage, requireInput bool) bool {
	if !usage.IsObject() || parsed == nil || parsed.OutputTokens != 0 || parsed.InputTokens < 0 ||
		parsed.CacheReadInputTokens < 0 || parsed.CacheCreationInputTokens < 0 ||
		parsed.ClaudeCacheCreation5mTokens < 0 || parsed.ClaudeCacheCreation1hTokens < 0 ||
		parsed.GetCacheCreation5mTokens() < 0 || parsed.GetCacheCreation1hTokens() < 0 ||
		(parsed.ServerToolUse != nil && parsed.ServerToolUse.WebSearchRequests != 0) {
		return false
	}
	output := usage.Get("output_tokens")
	if output.Type != gjson.Number || output.Num != 0 {
		return false
	}
	if requireInput && usage.Get("input_tokens").Type != gjson.Number {
		return false
	}
	valid := true
	usage.ForEach(func(key, count gjson.Result) bool {
		if key.Str == "iterations" && count.Exists() && count.Raw != "[]" && count.Raw != "null" {
			valid = false
		}
		if count.Type == gjson.Number && (count.Num < 0 || math.Trunc(count.Num) != count.Num) {
			valid = false
		}
		return valid
	})
	for _, path := range []string{"server_tool_use", "cache_creation"} {
		details := usage.Get(path)
		if !details.Exists() || details.Type == gjson.Null {
			continue
		}
		if !details.IsObject() {
			return false
		}
		details.ForEach(func(_, count gjson.Result) bool {
			if count.Type != gjson.Number || count.Num < 0 || math.Trunc(count.Num) != count.Num ||
				(path == "server_tool_use" && count.Num != 0) {
				valid = false
			}
			return valid
		})
	}
	return valid
}

// ObserveRefusalResponse inspects presence in the original JSON, before any
// usage estimation or response conversion can erase that distinction.
func (info *ClaudeResponseInfo) ObserveRefusalResponse(response *dto.ClaudeResponse, data string) {
	root := gjson.Parse(data)
	content := root.Get("content")
	info.refusal = claudeRefusalEvidence{
		started: true, delta: true, stopped: true,
		refusal: response.StopReason == "refusal" && root.Get("stop_reason").Str == "refusal",
		// Check parsed values as well as original presence. Duplicate JSON
		// keys must not make evidence disagree with the content we deliver.
		invalid: !hasUniqueRefusalFields(root) || len(response.Content) != 0 || response.Completion != "" || !content.IsArray() || len(content.Array()) != 0 ||
			root.Get("completion").Str != "" || !validRefusalUsage(root.Get("usage"), response.Usage, true),
	}
}

func (info *ClaudeResponseInfo) ObserveRefusalStreamEvent(response *dto.ClaudeResponse, data string) {
	root := gjson.Parse(data)
	state := &info.refusal
	state.invalid = state.invalid || !hasUniqueRefusalFields(root) || root.Get("type").Str != response.Type
	switch root.Get("type").Str {
	case "message_start":
		content := root.Get("message.content")
		if response.Message == nil {
			state.invalid = true
			return
		}
		parsedContent, contentPresent := response.Message.Content.([]any)
		state.invalid = state.invalid || state.started || state.delta || state.stopped ||
			!contentPresent || len(parsedContent) != 0 || !content.IsArray() || len(content.Array()) != 0 ||
			!validRefusalUsage(root.Get("message.usage"), response.Message.Usage, true)
		state.started = true
	case "message_delta":
		state.invalid = state.invalid || !state.started || state.delta || state.stopped || !validRefusalUsage(root.Get("usage"), response.Usage, false)
		state.delta = true
		state.refusal = response.Delta != nil && response.Delta.StopReason != nil && *response.Delta.StopReason == "refusal" && root.Get("delta.stop_reason").Str == "refusal"
	case "message_stop":
		state.invalid = state.invalid || !state.started || !state.delta || state.stopped
		state.stopped = true
	case "ping":
		// Keep-alive events do not produce output.
	default:
		// Any content block (including empty text, thinking and tools), error,
		// or unknown event prevents us from proving a pre-output refusal.
		state.invalid = true
	}
}

// HasPreOutputRefusalEvidence reports what the original response proves, before
// a host decides whether its configured billing policy should use estimates.
func (info *ClaudeResponseInfo) HasPreOutputRefusalEvidence() bool {
	state := info.refusal
	return !state.invalid && state.started && state.delta && state.stopped && state.refusal
}

// FinalizeRefusalBilling only adds evidence; it never edits reference tokens.
func (info *ClaudeResponseInfo) FinalizeRefusalBilling() {
	if info.Usage == nil {
		return
	}
	if info.Usage.BillingUsage != nil {
		info.Usage.BillingUsage.ClaudePreOutputRefusal = false
	}
	if !info.HasPreOutputRefusalEvidence() {
		return
	}
	if info.Usage.BillingUsage == nil {
		// A complete, explicitly all-zero usage has no normal billing snapshot.
		info.Usage.BillingUsage = &dto.BillingUsage{
			Source: dto.BillingUsageSourceClaudeMessages, Semantic: dto.BillingUsageSemanticAnthropic,
			ClaudeUsage: &dto.ClaudeUsage{},
		}
	}
	info.Usage.BillingUsage.ClaudePreOutputRefusal = true
}
