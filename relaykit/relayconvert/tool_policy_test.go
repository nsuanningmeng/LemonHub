package relayconvert

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestAllowedToolRequiredClaudeWire(t *testing.T) {
	var req dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"parallel_tool_calls":false,"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"function","function":{"name":"tool_b"}}]}},"tools":[{"type":"function","function":{"name":"tool_a","parameters":{"type":"object"}}},{"type":"function","function":{"name":"tool_b","strict":true,"parameters":{"type":"object"}}}]}`, &req))
	result, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, &req)
	require.NoError(t, err)
	raw, err := kitutil.Marshal(result.Value)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, kitutil.Unmarshal(raw, &wire))
	assert.Equal(t, map[string]any{"type": "tool", "name": "tool_b", "disable_parallel_tool_use": true}, wire["tool_choice"])
	tool := wire["tools"].([]any)[1].(map[string]any)
	assert.Equal(t, true, tool["strict"])
}
func TestUnsupportedToolConstraintsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		format types.RelayFormat
		body   string
	}{
		{types.RelayFormatClaude, `{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"private_tool","parameters":{}}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"function","function":{"name":"private_tool"}}]}}}`},
		{types.RelayFormatGemini, `{"model":"m","messages":[{"role":"user","content":"hi"}],"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"private_tool","parameters":{}}}]}`},
		{types.RelayFormatGemini, `{"model":"m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"private_tool","strict":true,"parameters":{}}}]}`},
	} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(tc.body, &req))
		_, err := ConvertRequest(context.Background(), nil, tc.format, &req)
		require.Error(t, err)
		var api *types.NewAPIError
		require.True(t, errors.As(err, &api))
		assert.Equal(t, 400, api.StatusCode)
		assert.True(t, types.IsSkipRetryError(api))
		assert.NotContains(t, err.Error(), "private_tool")
	}
}

func TestClaudeToolRestrictionsBridgeWire(t *testing.T) {
	var req dto.ClaudeRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"tool","name":"tool_b","disable_parallel_tool_use":true},"tools":[{"name":"tool_b","strict":true,"allowed_callers":["direct"],"input_schema":{"type":"object"}}]}`, &req))
	for _, target := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		result, err := ConvertRequest(context.Background(), nil, target, &req)
		require.NoError(t, err)
		raw, err := kitutil.Marshal(result.Value)
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, kitutil.Unmarshal(raw, &wire))
		assert.Equal(t, false, wire["parallel_tool_calls"])
		tool := wire["tools"].([]any)[0].(map[string]any)
		if target == types.RelayFormatOpenAI {
			assert.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": "tool_b"}}, wire["tool_choice"])
			assert.Equal(t, true, tool["function"].(map[string]any)["strict"])
		} else {
			assert.Equal(t, map[string]any{"type": "function", "name": "tool_b"}, wire["tool_choice"])
			assert.Equal(t, true, tool["strict"])
		}
	}
	_, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, &req)
	require.Error(t, err)
	var api *types.NewAPIError
	require.True(t, errors.As(err, &api))
	assert.Equal(t, 400, api.StatusCode)
}

func TestClaudeCallerRestrictionsFailBeforeBridge(t *testing.T) {
	for _, callers := range []string{`["private-execution-context"]`, `[]`, `null`, `"private-execution-context"`} {
		var req dto.ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"private-tool","allowed_callers":`+callers+`,"input_schema":{"type":"object"}}]}`, &req))
		for _, target := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatGemini} {
			_, err := ConvertRequest(context.Background(), nil, target, &req)
			require.Error(t, err)
			var api *types.NewAPIError
			require.True(t, errors.As(err, &api))
			assert.Equal(t, 400, api.StatusCode)
			assert.True(t, types.IsSkipRetryError(api))
			assert.NotContains(t, err.Error(), "private-")
		}
		native, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, &req)
		require.NoError(t, err)
		assert.Same(t, &req, native.Value)
	}
}

func TestOpenAIToolPolicyResponsesFamilyWire(t *testing.T) {
	for _, mode := range []string{"auto", "required"} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"`+mode+`","tools":[{"type":"function","function":{"name":"tool_b"}}]}},"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"tool_b","strict":false,"parameters":{"type":"object"}}}]}`, &req))
		result, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAIResponses, &req)
		require.NoError(t, err)
		responses := result.Value.(*dto.OpenAIResponsesRequest)
		var choice map[string]any
		require.NoError(t, kitutil.Unmarshal(responses.ToolChoice, &choice))
		assert.Equal(t, map[string]any{"type": "allowed_tools", "mode": mode, "tools": []any{map[string]any{"type": "function", "name": "tool_b"}}}, choice)
		var tools []map[string]any
		require.NoError(t, kitutil.Unmarshal(responses.Tools, &tools))
		assert.Equal(t, false, tools[0]["strict"])
		back, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, responses)
		require.NoError(t, err)
		chat := back.Value.(*dto.GeneralOpenAIRequest)
		wire, err := kitutil.Marshal(chat.ToolChoice)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"allowed_tools","allowed_tools":{"mode":"`+mode+`","tools":[{"type":"function","function":{"name":"tool_b"}}]}}`, string(wire))
		require.NotNil(t, chat.Tools[0].Function.Strict)
		assert.False(t, *chat.Tools[0].Function.Strict)
		if mode == "required" {
			converted, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, responses)
			require.NoError(t, err)
			raw, err := kitutil.Marshal(converted.Value)
			require.NoError(t, err)
			assert.Contains(t, string(raw), `"strict":false`)
			assert.Contains(t, string(raw), `"type":"tool"`)
		} else {
			_, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, responses)
			require.Error(t, err)
		}
	}
}

func TestRequiredToolSubsetGeminiWireAndDefaults(t *testing.T) {
	for _, names := range []string{`[{"type":"function","function":{"name":"tool_b"}}]`, `[{"type":"function","function":{"name":"tool_a"}},{"type":"function","function":{"name":"tool_b"}}]`} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"tool_a","strict":false,"parameters":{"type":"object"}}},{"type":"function","function":{"name":"tool_b","parameters":{"type":"object"}}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":`+names+`}}}`, &req))
		result, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, &req)
		require.NoError(t, err)
		gemini := result.Value.(*dto.GeminiChatRequest)
		assert.Equal(t, dto.FunctionCallingConfigMode("ANY"), gemini.ToolConfig.FunctionCallingConfig.Mode)
		want := []string{"tool_b"}
		if strings.Contains(names, "tool_a") {
			want = []string{"tool_a", "tool_b"}
		}
		assert.Equal(t, want, gemini.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
	}
	for _, choice := range []string{`null`, `"auto"`, `"none"`} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tool_choice":`+choice+`,"parallel_tool_calls":false}`, &req))
		_, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, &req)
		require.NoError(t, err)
	}
}

func TestResponsesStrictConstraintSurvivesViaChat(t *testing.T) {
	for _, strict := range []string{`true`, `false`, `"private-malformed"`} {
		var req dto.OpenAIResponsesRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_output_tokens":64,"input":"hi","tools":[{"type":"function","name":"tool_b","strict":`+strict+`,"parameters":{"type":"object"}}]}`, &req))
		_, err := ConvertRequestVia(context.Background(), nil, &req, types.RelayFormatOpenAI, types.RelayFormatGemini)
		if strict == `false` {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			var api *types.NewAPIError
			require.True(t, errors.As(err, &api))
			assert.Equal(t, 400, api.StatusCode)
			assert.True(t, types.IsSkipRetryError(api))
			assert.NotContains(t, err.Error(), "private-")
		}
	}
}
