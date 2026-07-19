package dto

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
)

// BillingUsage is an in-process settlement snapshot. It names the converter /
// channel kind (oai_chat, claude_messages, gemini_chat, ...), which this fork
// deliberately hides from end users, so it must never appear in any usage
// object marshaled into a client response.
func TestBillingUsageNeverSerializedToClients(t *testing.T) {
	snapshot := NewOpenAIChatBillingUsage(&Usage{PromptTokens: 10, CompletionTokens: 5})

	cases := map[string]any{
		"openai_usage": &Usage{PromptTokens: 10, CompletionTokens: 5, BillingUsage: snapshot},
		"claude_usage": &ClaudeUsage{InputTokens: 10, OutputTokens: 5, BillingUsage: snapshot},
		"gemini_usage": &GeminiUsageMetadata{PromptTokenCount: 10, BillingUsage: snapshot},
	}
	for name, v := range cases {
		data, err := common.Marshal(v)
		require.NoError(t, err, name)
		require.NotContains(t, string(data), "billing_usage", name)
		require.NotContains(t, string(data), "oai_chat", name)
	}
}
