package relayconvert

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeExplicitEffortOpenAIWire(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high"} {
		t.Run(effort, func(t *testing.T) {
			var req dto.ClaudeRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"deepseek-v4-flash","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],"thinking":{"type":"adaptive"},"output_config":{"effort":"`+effort+`"}}`, &req))
			converted, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &req)
			require.NoError(t, err)
			raw, err := kitutil.Marshal(converted.Value)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, kitutil.Unmarshal(raw, &wire))
			assert.Equal(t, effort, wire["reasoning_effort"])
			assert.Equal(t, float64(1024), wire["max_tokens"])
			assert.Equal(t, "deepseek-v4-flash", wire["model"])
			assert.NotContains(t, wire, "output_config")
			assert.NotContains(t, wire, "thinking")
			assert.NotContains(t, wire, "verbosity")
			compat, err := ClaudeMessagesRequestToOpenAIChat(req, nil)
			require.NoError(t, err)
			assert.Equal(t, effort, compat.ReasoningEffort)
		})
	}
}

func TestClaudeEffortIsNeverInferredFromThinkingBudget(t *testing.T) {
	for _, thinking := range []string{`null`, `{"type":"disabled"}`, `{"type":"adaptive"}`, `{"type":"enabled","budget_tokens":8192}`} {
		var req dto.ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],"thinking":`+thinking+`}`, &req))
		info := &convmeta.Values{ReasoningEffort: "high", OriginModelName: "m-thinking"}
		out, err := ClaudeMessagesRequestToOpenAIChat(req, info)
		require.NoError(t, err)
		assert.Empty(t, out.ReasoningEffort)
		assert.Equal(t, "m-thinking", out.Model)
		wire, err := kitutil.Marshal(out)
		require.NoError(t, err)
		assert.NotContains(t, string(wire), "reasoning_effort")
	}
}

func TestClaudeUnrepresentableEffortFailsPrivately(t *testing.T) {
	for _, config := range []string{`{"effort":"max"}`, `{"effort":"private-unknown"}`, `{"effort":42}`, `{"effort":{"private":"value"}}`, `"private-config"`} {
		var req dto.ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],"output_config":`+config+`}`, &req))
		_, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &req)
		require.Error(t, err)
		var api *types.NewAPIError
		require.True(t, errors.As(err, &api))
		assert.Equal(t, 400, api.StatusCode)
		assert.True(t, types.IsSkipRetryError(api))
		assert.NotContains(t, err.Error(), "private")
		native, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, &req)
		require.NoError(t, err)
		assert.Same(t, &req, native.Value)
	}
}

func TestClaudeOpenRouterEffortDialectRemainsUnchanged(t *testing.T) {
	for _, tc := range []struct{ thinking, wantReasoning string }{
		{`{"type":"adaptive"}`, `{"enabled":true}`},
		{`{"type":"enabled","budget_tokens":8192}`, `{"enabled":true,"max_tokens":8192}`},
		{`{"type":"disabled"}`, `{"enabled":false}`},
	} {
		var req dto.ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],"thinking":`+tc.thinking+`,"output_config":{"effort":"medium"}}`, &req))
		info := &convmeta.Values{OriginModelName: "m-thinking", Options: &convmeta.Options{OpenRouterDialect: true}}
		result, err := ConvertRequest(context.Background(), info, types.RelayFormatOpenAI, &req)
		require.NoError(t, err)
		out := result.Value.(*dto.GeneralOpenAIRequest)
		assert.Empty(t, out.ReasoningEffort)
		assert.JSONEq(t, `"medium"`, string(out.Verbosity))
		assert.JSONEq(t, tc.wantReasoning, string(out.Reasoning))
		assert.Equal(t, "m", out.Model)
	}
}

func TestClaudeExplicitEffortPreservesContentAndToolConstraints(t *testing.T) {
	var req dto.ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":1024,"system":"system body","messages":[{"role":"user","content":"hello"}],"tool_choice":{"type":"tool","name":"run","disable_parallel_tool_use":true},"tools":[{"name":"run","strict":true,"input_schema":{"type":"object"}}],"output_config":{"effort":"low"}}`, &req))
	before, err := kitutil.Marshal(req)
	require.NoError(t, err)
	info := &convmeta.Values{ReasoningEffort: "high"}
	out, err := ClaudeMessagesRequestToOpenAIChat(req, info)
	require.NoError(t, err)
	assert.Equal(t, "low", out.ReasoningEffort)
	require.Len(t, out.Messages, 2)
	assert.Equal(t, "system body", out.Messages[0].StringContent())
	assert.Equal(t, "hello", out.Messages[1].StringContent())
	require.NotNil(t, out.ParallelTooCalls)
	assert.False(t, *out.ParallelTooCalls)
	require.NotNil(t, out.Tools[0].Function.Strict)
	assert.True(t, *out.Tools[0].Function.Strict)
	assert.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": "run"}}, out.ToolChoice)
	after, err := kitutil.Marshal(req)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
