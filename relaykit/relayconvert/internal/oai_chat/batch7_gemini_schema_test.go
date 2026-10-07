package oaichat

import (
	"context"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch7GeminiFullSchemaWire(t *testing.T) {
	for _, schema := range []string{`{"type":"object","properties":{"pin":{"const":"alice"},"value":{"oneOf":[{"type":"string"},{"type":"integer"}]}},"additionalProperties":false}`, `{"type":"object","properties":{},"additionalProperties":false}`} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":`+schema+`}}]}`), &req))
		before, err := kitutil.Marshal(req)
		require.NoError(t, err)
		out, err := OpenAIChatRequestToGeminiGenerateContent(context.Background(), req, nil)
		require.NoError(t, err)
		wire, err := kitutil.Marshal(out)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, kitutil.Unmarshal(wire, &body))
		tool := body["tools"].([]any)[0].(map[string]any)
		fn := tool["functionDeclarations"].([]any)[0].(map[string]any)
		actual, exists := fn["parametersJsonSchema"]
		require.True(t, exists)
		require.NotContains(t, fn, "parameters")
		encoded, err := kitutil.Marshal(actual)
		require.NoError(t, err)
		assert.JSONEq(t, schema, string(encoded))
		after, err := kitutil.Marshal(req)
		require.NoError(t, err)
		assert.JSONEq(t, string(before), string(after))
	}
}

type batch7DiagnosticMeta struct {
	*convmeta.Values
	Diagnostics []convmeta.ConversionDiagnostic
}

func (m *batch7DiagnosticMeta) RecordConversionDiagnostic(d convmeta.ConversionDiagnostic) {
	m.Diagnostics = append(m.Diagnostics, d)
}

func TestBatch7DeveloperCompatibilityDiagnostic(t *testing.T) {
	for _, tt := range []struct {
		messages, want string
		diagnostics    int
	}{
		{`[{"role":"system","content":"private-system"},{"role":"developer","content":"private-developer"},{"role":"user","content":"hi"}]`, "private-system\nprivate-developer", 1},
		{`[{"role":"developer","content":"private-developer"}]`, "private-developer", 1},
		{`[{"role":"system","content":"private-system"},{"role":"user","content":"hi"}]`, "private-system", 0},
		{`[{"role":"user","content":"hi"}]`, "", 0},
		{`[{"role":"developer","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}]`, "firstsecond", 1},
	} {
		var req dto.GeneralOpenAIRequest
		require.NoError(t, kitutil.Unmarshal([]byte(`{"messages":`+tt.messages+`}`), &req))
		meta := &batch7DiagnosticMeta{Values: &convmeta.Values{}}
		out, err := OpenAIChatRequestToGeminiGenerateContent(context.Background(), req, meta)
		require.NoError(t, err)
		if tt.want == "" {
			assert.Nil(t, out.SystemInstructions)
		} else {
			require.NotNil(t, out.SystemInstructions)
			require.Len(t, out.SystemInstructions.Parts, 1)
			assert.Equal(t, tt.want, out.SystemInstructions.Parts[0].Text)
		}
		require.Len(t, meta.Diagnostics, tt.diagnostics)
		for _, d := range meta.Diagnostics {
			assert.Equal(t, convmeta.DiagnosticDeveloperRoleMerged, d.Code)
			wire, err := kitutil.Marshal(d)
			require.NoError(t, err)
			assert.NotContains(t, string(wire), "private-")
			assert.Equal(t, "messages.role", d.Field)
		}
	}
}
