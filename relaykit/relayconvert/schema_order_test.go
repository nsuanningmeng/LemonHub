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

func TestRequestConversionPreservesClientSchemaPropertyOrder(t *testing.T) {
	const schema = `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"string"}},"required":["reasoning","answer"]}`
	tests := []struct {
		name   string
		source string
		target types.RelayFormat
	}{
		{"chat to Claude", "chat", types.RelayFormatClaude},
		{"chat to Responses", "chat", types.RelayFormatOpenAIResponses},
		{"chat to Gemini JSON schema", "chat", types.RelayFormatGemini},
		{"Responses to chat", "responses", types.RelayFormatOpenAI},
		{"Responses to Claude", "responses", types.RelayFormatClaude},
		{"Responses to Gemini JSON schema", "responses", types.RelayFormatGemini},
		{"Claude to chat", "claude", types.RelayFormatOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request any
			switch tt.source {
			case "chat":
				r := &dto.GeneralOpenAIRequest{}
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"target","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"result","schema":`+schema+`}},"tools":[{"type":"function","function":{"name":"lookup","parameters":`+schema+`}}]}`, r))
				request = r
			case "responses":
				r := &dto.OpenAIResponsesRequest{}
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"target","max_output_tokens":100,"input":"hi","text":{"format":{"type":"json_schema","name":"result","schema":`+schema+`}},"tools":[{"type":"function","name":"lookup","parameters":`+schema+`}]}`, r))
				request = r
			case "claude":
				r := &dto.ClaudeRequest{}
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"target","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":`+schema+`}]}`, r))
				request = r
			}
			before, err := kitutil.Marshal(request)
			require.NoError(t, err)
			info := &convmeta.Values{Options: &convmeta.Options{Gemini: convmeta.GeminiOptions{SupportsJSONSchema: func(string) bool { return true }}}}
			converted, err := ConvertRequest(context.Background(), info, tt.target, request)
			require.NoError(t, err)
			wire, err := kitutil.Marshal(converted.Value)
			require.NoError(t, err)
			assert.Contains(t, string(wire), schema)
			assert.NotContains(t, string(wire), `"properties":{"answer"`)
			after, err := kitutil.Marshal(request)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}
