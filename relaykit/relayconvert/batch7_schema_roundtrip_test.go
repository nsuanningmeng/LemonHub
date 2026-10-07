package relayconvert

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch7SchemaRequestRoutes(t *testing.T) {
	schema := `{"type":"object","properties":{"pin":{"const":"alice"},"value":{"oneOf":[{"type":"string"},{"type":"integer"}],"type":["string","integer"]}},"additionalProperties":false,"$defs":{"entry":{"type":"array","prefixItems":[{"const":1},{"const":2}]}}}`
	for _, source := range []string{"claude", "responses", "gemini"} {
		t.Run(source, func(t *testing.T) {
			var input any
			switch source {
			case "claude":
				r := &dto.ClaudeRequest{}
				require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":`+schema+`}]}`), r))
				input = r
			case "responses":
				r := &dto.OpenAIResponsesRequest{}
				require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"m","input":"hi","tools":[{"type":"function","name":"lookup","parameters":`+schema+`}]}`), r))
				input = r
			case "gemini":
				r := &dto.GeminiChatRequest{}
				require.NoError(t, kitutil.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"lookup","parametersJsonSchema":`+schema+`}]}]}`), r))
				input = r
			}
			before, err := kitutil.Marshal(input)
			require.NoError(t, err)
			var result *RequestResult
			if source == "gemini" {
				result, err = ConvertRequestVia(context.Background(), nil, input, types.RelayFormatOpenAI, types.RelayFormatGemini)
			} else {
				result, err = ConvertRequest(context.Background(), nil, types.RelayFormatGemini, input)
			}
			require.NoError(t, err)
			wire, err := kitutil.Marshal(result.Value)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, kitutil.Unmarshal(wire, &body))
			fn := body["tools"].([]any)[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
			require.Contains(t, fn, "parametersJsonSchema")
			assert.NotContains(t, fn, "parameters")
			actual, err := kitutil.Marshal(fn["parametersJsonSchema"])
			require.NoError(t, err)
			assert.JSONEq(t, schema, string(actual))
			after, err := kitutil.Marshal(input)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after))
		})
	}
}

func TestBatch7NativeGeminiSchemaRead(t *testing.T) {
	for _, field := range []string{"parameters", "parametersJsonSchema"} {
		var req dto.GeminiChatRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"lookup","`+field+`":{"type":"object","properties":{"pin":{"const":"alice"}}}}]}]}`), &req))
		result, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &req)
		require.NoError(t, err)
		out := result.Value.(*dto.GeneralOpenAIRequest)
		require.Len(t, out.Tools, 1)
		wire, err := kitutil.Marshal(out)
		require.NoError(t, err)
		assert.Contains(t, string(wire), `"const":"alice"`)
		assert.NotContains(t, string(wire), "parametersJsonSchema")
	}
	var req dto.GeminiChatRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"tools":[{"functionDeclarations":[{"name":"private-tool","parameters":{},"parametersJsonSchema":{}}]}]}`), &req))
	_, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, &req)
	var api *types.NewAPIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 400, api.StatusCode)
	assert.True(t, types.IsSkipRetryError(api))
	assert.NotContains(t, err.Error(), "private-tool")
	native, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, &req)
	require.NoError(t, err)
	assert.Same(t, &req, native.Value)
}

type batch7RouteDiagnosticMeta struct {
	*convmeta.Values
	Diagnostics []convmeta.ConversionDiagnostic
}

func (m *batch7RouteDiagnosticMeta) RecordConversionDiagnostic(d convmeta.ConversionDiagnostic) {
	m.Diagnostics = append(m.Diagnostics, d)
}

func TestBatch7DeveloperDirectResponsesRoute(t *testing.T) {
	var req dto.OpenAIResponsesRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"m","instructions":"private-instructions","input":[{"role":"developer","content":[{"type":"input_text","text":"private-developer"}]},{"role":"user","content":"hi"}]}`), &req))
	meta := &batch7RouteDiagnosticMeta{Values: &convmeta.Values{}}
	result, err := ConvertRequest(context.Background(), meta, types.RelayFormatGemini, &req)
	require.NoError(t, err)
	out := result.Value.(*dto.GeminiChatRequest)
	require.NotNil(t, out.SystemInstructions)
	assert.Equal(t, "private-instructions\nprivate-developer", out.SystemInstructions.Parts[0].Text)
	require.Len(t, meta.Diagnostics, 1)
	d := meta.Diagnostics[0]
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), d.Source)
	assert.Equal(t, "input.role", d.Field)
	wire, err := kitutil.Marshal(d)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "private-")
	nativeMeta := &batch7RouteDiagnosticMeta{Values: &convmeta.Values{}}
	_, err = ConvertRequest(context.Background(), nativeMeta, types.RelayFormatOpenAIResponses, &req)
	require.NoError(t, err)
	assert.Empty(t, nativeMeta.Diagnostics)
}

func TestBatch7DirectResponsesGeminiStrictAndParallelRejection(t *testing.T) {
	for _, fragment := range []string{`"tools":[{"type":"function","name":"private-name","strict":true,"parameters":{"type":"object"}}]`, `"tools":[{"type":"function","name":"private-name","strict":false,"parameters":{"type":"object"}}],"parallel_tool_calls":false`} {
		var req dto.OpenAIResponsesRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"m","input":"hi",`+fragment+`}`), &req))
		_, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, &req)
		var api *types.NewAPIError
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 400, api.StatusCode)
		assert.True(t, types.IsSkipRetryError(api))
		assert.NotContains(t, err.Error(), "private-name")
	}
	var req dto.OpenAIResponsesRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"m","input":"hi","tools":[{"type":"function","name":"lookup","strict":false,"parameters":{"type":"object","properties":{}}}]}`), &req))
	_, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, &req)
	require.NoError(t, err)
}
