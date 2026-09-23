package openai

import (
	"strconv"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// responsesUsageAccumulator estimates only observed generation when upstream
// omits usage. Lifecycle events alone are not evidence of billable work.
// The handler owns it and settles its result once through the normal billing path.
type responsesUsageAccumulator struct {
	info      *relaycommon.RelayInfo
	usage     *dto.Usage
	generated bool
	aliases   map[string]*responsesOutputEstimate
	outputs   []*responsesOutputEstimate
}

type responsesOutputEstimate struct {
	delta    strings.Builder
	snapshot string
}

func (a *responsesUsageAccumulator) output(index *int, id string) *responsesOutputEstimate {
	if a.aliases == nil {
		a.aliases = make(map[string]*responsesOutputEstimate)
	}
	// Older compatible providers sometimes omit the index for a single item.
	indexKey := "index:0"
	if index != nil {
		indexKey = "index:" + strconv.Itoa(*index)
	} else if id != "" {
		indexKey = "id:" + id
	}
	var output *responsesOutputEstimate
	if id != "" {
		output = a.aliases["id:"+id]
	}
	if output == nil {
		output = a.aliases[indexKey]
	}
	if output == nil {
		output = &responsesOutputEstimate{}
		a.outputs = append(a.outputs, output)
	}
	a.aliases[indexKey] = output
	if id != "" {
		a.aliases["id:"+id] = output
	}
	return output
}

func (a *responsesUsageAccumulator) observeItem(item *dto.ResponsesOutput, index *int) {
	if item == nil {
		return
	}
	var text strings.Builder
	for _, content := range item.Content {
		text.WriteString(content.Text)
		text.WriteString(content.Refusal)
	}
	for _, summary := range item.Summary {
		text.WriteString(summary.Text)
	}
	if item.Type == dto.BuildInCallFunctionCall {
		text.WriteString(item.ArgumentsString())
	}
	if text.Len() > 0 {
		a.output(index, item.ID).snapshot = text.String()
		a.generated = true
	}
	if item.Status == "" || item.Status == "completed" {
		switch item.Type {
		case dto.BuildInCallWebSearchCall, dto.BuildInCallFileSearchCall, dto.BuildInCallFunctionCall:
			a.generated = true
		}
	}
}

func (a *responsesUsageAccumulator) observeResponse(response *dto.OpenAIResponsesResponse) {
	if response == nil {
		return
	}
	if response.Usage != nil {
		// Presence, not a nonzero token count, distinguishes authoritative usage
		// from missing usage. In particular, a real output_tokens:0 stays zero.
		usage := *response.Usage
		usage.PromptTokens = usage.InputTokens
		usage.CompletionTokens = usage.OutputTokens
		if usage.InputTokensDetails != nil {
			details := *usage.InputTokensDetails
			usage.InputTokensDetails = &details
			usage.PromptTokensDetails = details
		}
		if usage.OutputTokensDetails != nil {
			details := *usage.OutputTokensDetails
			usage.OutputTokensDetails = &details
			usage.CompletionTokenDetails = details
		}
		usage.BillingUsage = dto.CloneBillingUsage(response.Usage.BillingUsage)
		if usage.BillingUsage == nil {
			usage.BillingUsage = dto.NewOpenAIResponsesBillingUsage(&usage)
		}
		a.usage = &usage
	}
	for i := range response.Output {
		a.observeItem(&response.Output[i], &i)
	}
}

func (a *responsesUsageAccumulator) observe(event *dto.ResponsesStreamResponse) {
	switch event.Type {
	case "response.completed", "response.done", "response.failed", "response.error", "error", "response.incomplete", "response.cancelled", "response.canceled":
		a.observeResponse(event.Response)
	case "response.output_text.delta", "response.function_call_arguments.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.refusal.delta":
		if event.Delta != "" {
			a.output(event.OutputIndex, event.ItemID).delta.WriteString(event.Delta)
			a.generated = true
		}
	case dto.ResponsesOutputTypeItemDone:
		a.observeItem(event.Item, event.OutputIndex)
	}
}

func (a *responsesUsageAccumulator) finish() *dto.Usage {
	if a.usage != nil {
		return a.usage
	}
	var outputText strings.Builder
	for _, output := range a.outputs {
		// Item-done and terminal snapshots repeat the deltas for that output.
		// Take the fuller observed representation once, never sum both.
		if len(output.snapshot) >= output.delta.Len() {
			outputText.WriteString(output.snapshot)
		} else {
			outputText.WriteString(output.delta.String())
		}
	}
	model := a.info.GetUpstreamModelName()
	if model == "" {
		model = a.info.OriginModelName
	}
	usage := &dto.Usage{CompletionTokens: service.CountTextToken(outputText.String(), model)}
	if a.generated {
		usage.PromptTokens = a.info.GetEstimatePromptTokens()
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.BillingUsage = dto.NewOpenAIResponsesBillingUsage(usage)
	if usage.BillingUsage != nil {
		usage.BillingUsage.Estimated = true
	}
	a.usage = usage
	return usage
}
