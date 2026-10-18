package relayconvert

import (
	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiSchemasRespectAPIVersionWithoutLosingConstraints(t *testing.T) {
	for _, source := range []string{"chat", "responses"} {
		for _, supports := range []bool{false, true} {
			for _, tc := range []struct {
				name, schema string
				complex      bool
			}{
				{"simple", `{"type":"object","properties":{"answer":{"type":"string","enum":["yes","no"]}},"required":["answer"]}`, false},
				{"constraints", `{"type":"object","$defs":{"value":{"type":["string","integer"]}},"properties":{"answer":{"$ref":"#/$defs/value"}},"additionalProperties":false}`, true},
				{"boolean enum", `{"type":"object","properties":{"answer":{"type":"boolean","enum":[true,false]}}}`, true},
			} {
				t.Run(source+"/"+tc.name+map[bool]string{true: "/JSON", false: "/v1"}[supports], func(t *testing.T) {
					var request any
					if source == "chat" {
						req := &dto.GeneralOpenAIRequest{}
						require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-model","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"result","schema":`+tc.schema+`}},"tools":[{"type":"function","function":{"name":"check","parameters":`+tc.schema+`}}]}`, req))
						request = req
					} else {
						req := &dto.OpenAIResponsesRequest{}
						require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"gemini-model","input":"hi","text":{"format":{"type":"json_schema","name":"result","schema":`+tc.schema+`}},"tools":[{"type":"function","name":"check","parameters":`+tc.schema+`}]}`, req))
						request = req
					}
					before, err := kitutil.Marshal(request)
					require.NoError(t, err)
					info := &convmeta.Values{Options: &convmeta.Options{Gemini: convmeta.GeminiOptions{SupportsJSONSchema: func(string) bool { return supports }}}}
					converted, err := ConvertRequest(context.Background(), info, types.RelayFormatGemini, request)
					if !supports && tc.complex {
						var api *types.NewAPIError
						require.ErrorAs(t, err, &api)
						assert.Equal(t, 400, api.StatusCode)
						assert.True(t, types.IsSkipRetryError(api))
						assert.NotContains(t, err.Error(), "answer")
					} else {
						require.NoError(t, err)
						out := converted.Value.(*dto.GeminiChatRequest)
						if supports {
							assert.JSONEq(t, tc.schema, string(out.GenerationConfig.ResponseJsonSchema))
							assert.Nil(t, out.GenerationConfig.ResponseSchema)
						} else {
							assert.Empty(t, out.GenerationConfig.ResponseJsonSchema)
							require.NotNil(t, out.GenerationConfig.ResponseSchema)
							assert.Equal(t, "OBJECT", out.GenerationConfig.ResponseSchema.(map[string]any)["type"])
						}
						functions, err := kitutil.Any2Type[[]dto.GeminiFunctionDeclaration](out.GetTools()[0].FunctionDeclarations)
						require.NoError(t, err)
						require.Len(t, functions, 1)
						if supports {
							encoded, err := kitutil.Marshal(functions[0].ParametersJSONSchema)
							require.NoError(t, err)
							assert.JSONEq(t, tc.schema, string(encoded))
							assert.Nil(t, functions[0].Parameters)
						} else {
							assert.Nil(t, functions[0].ParametersJSONSchema)
							assert.NotNil(t, functions[0].Parameters)
						}
					}
					after, err := kitutil.Marshal(request)
					require.NoError(t, err)
					assert.Equal(t, before, after)
				})
			}
		}
	}
}

func TestResponsesHistoryDoesNotBecomeAssistantPrefill(t *testing.T) {
	for _, target := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini} {
		for _, tc := range []struct {
			name, input string
			omitted     bool
		}{
			{"completed history", `[{"role":"user","content":"question"},{"role":"assistant","content":"private-answer"}]`, true},
			{"tool result then history", `[{"type":"function_call","call_id":"a","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"a","output":"result"},{"role":"assistant","content":"private-answer"}]`, true},
			{"unresolved call", `[{"role":"user","content":"question"},{"type":"function_call","call_id":"a","name":"lookup","arguments":"{}"}]`, false},
			{"assistant-only history", `[{"role":"assistant","content":"private-answer"}]`, false},
		} {
			t.Run(string(target)+"/"+tc.name, func(t *testing.T) {
				var req dto.OpenAIResponsesRequest
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"target","max_output_tokens":100,"input":`+tc.input+`}`, &req))
				info := &batch7RouteDiagnosticMeta{Values: &convmeta.Values{}}
				before, err := kitutil.Marshal(req)
				require.NoError(t, err)
				out, err := ConvertRequest(context.Background(), info, target, &req)
				require.NoError(t, err)
				wire, err := kitutil.Marshal(out.Value)
				require.NoError(t, err)
				if tc.omitted {
					assert.NotContains(t, string(wire), "private-answer")
					require.Len(t, info.Diagnostics, 1)
					assert.Equal(t, convmeta.DiagnosticTrailingAssistantOmitted, info.Diagnostics[0].Code)
					assert.NotContains(t, info.Diagnostics[0].Message, "private-answer")
				} else {
					assert.Empty(t, info.Diagnostics)
					if tc.name == "assistant-only history" {
						assert.Contains(t, string(wire), "private-answer")
					} else {
						assert.Contains(t, string(wire), "lookup")
					}
				}
				after, err := kitutil.Marshal(req)
				require.NoError(t, err)
				assert.Equal(t, before, after)
			})
		}
	}
}

func TestClaudeAndGeminiAssistantTextSurvivesToolCalls(t *testing.T) {
	for _, source := range []string{"claude", "gemini"} {
		t.Run(source, func(t *testing.T) {
			var req any
			if source == "claude" {
				r := &dto.ClaudeRequest{}
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"model":"m","max_tokens":100,"messages":[{"role":"assistant","content":[{"type":"text","text":"Checking now"},{"type":"tool_use","id":"a","name":"lookup","input":{}}]}]}`, r))
				req = r
			} else {
				r := &dto.GeminiChatRequest{}
				require.NoError(t, kitutil.UnmarshalJsonStr(`{"contents":[{"role":"model","parts":[{"text":"Checking now"},{"functionCall":{"name":"lookup","args":{}}}]}]}`, r))
				req = r
			}
			out, err := ConvertRequest(context.Background(), nil, types.RelayFormatOpenAI, req)
			require.NoError(t, err)
			messages := out.Value.(*dto.GeneralOpenAIRequest).Messages
			require.Len(t, messages, 1)
			require.Len(t, messages[0].ParseToolCalls(), 1)
			wire, err := kitutil.Marshal(messages[0])
			require.NoError(t, err)
			assert.Contains(t, string(wire), "Checking now")
			assert.Equal(t, "lookup", messages[0].ParseToolCalls()[0].Function.Name)
		})
	}
}
