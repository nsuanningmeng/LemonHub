package dto

import (
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasoningAddedOutputHasRequiredEmptySummary(t *testing.T) {
	for _, summary := range [][]ResponsesReasoningSummaryPart{nil, {}} {
		event := ResponsesStreamResponse{Type: ResponsesOutputTypeItemAdded, Item: &ResponsesOutput{Type: "reasoning", ID: "reasoning-1", Status: "in_progress", Summary: summary}}
		wire, err := kitutil.Marshal(event)
		require.NoError(t, err)
		var object map[string]any
		require.NoError(t, kitutil.Unmarshal(wire, &object))
		item := object["item"].(map[string]any)
		value, present := item["summary"]
		assert.True(t, present)
		assert.Equal(t, []any{}, value)
		assert.NotContains(t, string(wire), "summary_text")
	}
}
func TestNonReasoningOutputWireRemainsUnchanged(t *testing.T) {
	output := ResponsesOutput{Type: "function_call", ID: "call", Status: "in_progress", Name: "tool"}
	wire, err := kitutil.Marshal(output)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"function_call","id":"call","status":"in_progress","role":"","content":null,"quality":"","size":"","name":"tool"}`, string(wire))
	assert.NotContains(t, string(wire), "summary")
}
func TestReasoningOutputPreservesReportedSummaryParts(t *testing.T) {
	output := ResponsesOutput{Type: "reasoning", ID: "reasoning-1", Summary: []ResponsesReasoningSummaryPart{{Type: "summary_text", Text: "reported"}}}
	wire, err := kitutil.Marshal(output)
	require.NoError(t, err)
	var decoded ResponsesOutput
	require.NoError(t, kitutil.Unmarshal(wire, &decoded))
	assert.Equal(t, output.Summary, decoded.Summary)
}
