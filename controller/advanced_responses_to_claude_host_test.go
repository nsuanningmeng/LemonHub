package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const responsesToClaudeHostConverter = "openai_responses_to_claude_messages"

func TestAdvancedResponsesToClaudeHostRoundTrip(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 10
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			fetch := system_setting.GetFetchSetting()
			oldEnabled, oldPrivate := fetch.EnableSSRFProtection, fetch.AllowPrivateIp
			fetch.EnableSSRFProtection, fetch.AllowPrivateIp = false, true
			t.Cleanup(func() { fetch.EnableSSRFProtection, fetch.AllowPrivateIp = oldEnabled, oldPrivate })
			var mu sync.Mutex
			var seenPath, seenKey, seenVersion, seenBody string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(500)
					return
				}
				mu.Lock()
				seenPath, seenKey, seenVersion, seenBody = r.URL.Path, r.Header.Get("x-api-key"), r.Header.Get("anthropic-version"), string(body)
				mu.Unlock()
				if !stream {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"msg_fixture","type":"message","role":"assistant","model":"claude-upstream","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"tool_fixture","name":"weather","input":{"city":"Paris"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":4}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range []string{
					`{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"claude-upstream","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
					`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
					`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
					`{"type":"content_block_stop","index":0}`,
					`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_fixture","name":"weather","input":{}}}`,
					`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
					`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}`,
					`{"type":"content_block_stop","index":1}`,
					`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
					`{"type":"message_stop"}`,
				} {
					_, _ = io.WriteString(w, "data: "+event+"\n\n")
					w.(http.Flusher).Flush()
				}
			}))
			t.Cleanup(upstream.Close)
			config := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/messages", Converter: responsesToClaudeHostConverter, Auth: &dto.AdvancedCustomRouteAuth{Type: dto.AdvancedCustomAuthTypeHeader, Name: "x-api-key", Value: "{api_key}"}}}}
			settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &config})
			require.NoError(t, err)
			channel := model.Channel{Id: 7479, Type: constant.ChannelTypeAdvancedCustom, Name: "responses-claude", ModelMapping: common.GetPointer(`{"gpt-4o":"claude-upstream"}`), Key: "fixture-channel-key", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled, OtherSettings: string(settings)}
			require.NoError(t, channel.ValidateSettings())
			require.NoError(t, db.Create(&channel).Error)
			var saved model.Channel
			require.NoError(t, db.First(&saved, channel.Id).Error)
			require.Equal(t, responsesToClaudeHostConverter, saved.GetOtherSettings().AdvancedCustom.Routes[0].Converter)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channel.Id, Enabled: true}).Error)
			body := `{"model":"gpt-4o","max_output_tokens":32,"instructions":"system contract","input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo="}]},{"type":"function_call","call_id":"previous_call","name":"weather","arguments":"{\"city\":\"Rome\"}"},{"type":"function_call_output","call_id":"previous_call","output":[{"type":"input_text","text":"sunny"},{"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo="},{"type":"input_file","file_data":"data:application/pdf;base64,JVBERi0xLjQ=","filename":"forecast.pdf"}]}],"tools":[{"type":"function","name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]}`
			if stream {
				body = strings.TrimSuffix(body, "}") + `,"stream":true}`
			}
			response, _ := runProtocolConversionRelay(t, user, token, saved, "/v1/responses", body, types.RelayFormatOpenAIResponses)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			mu.Lock()
			path, key, version, wire := seenPath, seenKey, seenVersion, seenBody
			mu.Unlock()
			assert.Equal(t, "/v1/messages", path)
			assert.Equal(t, "fixture-channel-key", key)
			assert.Equal(t, "2023-06-01", version)
			var request map[string]any
			require.NoError(t, common.Unmarshal([]byte(wire), &request))
			assert.NotContains(t, request, "input")
			assert.NotContains(t, request, "max_output_tokens")
			assert.Equal(t, float64(32), request["max_tokens"])
			assert.Contains(t, wire, `"tool_result"`)
			assert.Contains(t, wire, `"tool_use"`)
			assert.Contains(t, wire, `"image"`)
			assert.Contains(t, wire, "iVBORw0KGgo=")
			assert.Contains(t, wire, `"document"`)
			assert.Contains(t, wire, "JVBERi0xLjQ=")
			assert.Contains(t, wire, "sunny")
			assert.Contains(t, wire, "system contract")
			assert.Equal(t, "claude-upstream", request["model"])
			tools, ok := request["tools"].([]any)
			require.True(t, ok)
			require.Len(t, tools, 1)
			function, ok := tools[0].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "weather", function["name"])
			schema, ok := function["input_schema"].(map[string]any)
			require.True(t, ok)
			properties, ok := schema["properties"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, map[string]any{"type": "string"}, properties["city"])
			if !stream {
				var output dto.OpenAIResponsesResponse
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &output))
				assert.Equal(t, "response", output.Object)
				assert.Equal(t, "gpt-4o", output.Model)
				require.NotNil(t, output.Usage)
				assert.Equal(t, 10, output.Usage.InputTokens)
				assert.Equal(t, 4, output.Usage.OutputTokens)
				assert.Contains(t, response.Body.String(), `"function_call"`)
				assert.Contains(t, response.Body.String(), "Paris")
			} else {
				var eventTypes []string
				var events []dto.ResponsesStreamResponse
				for _, line := range strings.Split(response.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					data := strings.TrimPrefix(line, "data: ")
					require.NotEqual(t, "[DONE]", data, "Responses lifecycle is not Chat DONE")
					var event dto.ResponsesStreamResponse
					require.NoError(t, common.Unmarshal([]byte(data), &event))
					eventTypes = append(eventTypes, event.Type)
					events = append(events, event)
				}
				require.NotEmpty(t, eventTypes, response.Body.String())
				assert.Equal(t, "response.created", eventTypes[0])
				assert.Equal(t, "response.completed", eventTypes[len(eventTypes)-1])
				assert.Equal(t, 1, strings.Count(response.Body.String(), `"type":"response.completed"`))
				assert.Contains(t, eventTypes, "response.output_text.delta")
				assert.Contains(t, eventTypes, "response.function_call_arguments.delta")
				for _, eventType := range []string{"response.output_item.added", "response.output_item.done", "response.content_part.added", "response.content_part.done", "response.output_text.done", "response.function_call_arguments.done"} {
					assert.Contains(t, eventTypes, eventType)
				}
				completed := events[len(events)-1].Response
				require.NotNil(t, completed)
				assert.Equal(t, "gpt-4o", completed.Model)
				require.NotNil(t, completed.Usage)
				assert.Equal(t, 10, completed.Usage.InputTokens)
				assert.Equal(t, 4, completed.Usage.OutputTokens)
				assert.Contains(t, response.Body.String(), "Paris")
				require.Len(t, completed.Output, 2)
				assert.Equal(t, "function_call", completed.Output[1].Type)
				assert.Equal(t, "weather", completed.Output[1].Name)
				var arguments string
				require.NoError(t, common.Unmarshal(completed.Output[1].Arguments, &arguments))
				assert.JSONEq(t, `{"city":"Paris"}`, arguments)
			}
			drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, service.WaitBillingRefunds(drain))
			var consume model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&consume).Error)
			assert.Equal(t, 10, consume.PromptTokens)
			assert.Equal(t, 4, consume.CompletionTokens)
			assert.Positive(t, consume.Quota)
		})
	}
}

func TestAdvancedResponsesToClaudeHostSettingsRejectMismatchedRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, path, converter string
		valid                 bool
	}{
		{"Responses conversion", "/v1/responses", responsesToClaudeHostConverter, true},
		{"wrong source", "/v1/messages", responsesToClaudeHostConverter, false},
		{"Chat source", "/v1/chat/completions", responsesToClaudeHostConverter, false},
		{"compact", "/v1/responses/compact", responsesToClaudeHostConverter, false},
		{"alpha management", "/v1/alpha/search", responsesToClaudeHostConverter, false},
		{"unknown converter", "/v1/responses", "unregistered_converter", false},
		{"existing native Responses", "/v1/responses", "none", true},
		{"existing native Claude", "/v1/messages", "none", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: tc.path, UpstreamPath: "/v1/messages", Converter: tc.converter}}}
			settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &config})
			require.NoError(t, err)
			channel := model.Channel{Type: constant.ChannelTypeAdvancedCustom, OtherSettings: string(settings)}
			if tc.valid {
				require.NoError(t, channel.ValidateSettings())
			} else {
				require.Error(t, channel.ValidateSettings())
			}
		})
	}
}

func TestAdvancedResponsesToClaudeHostErrorsRefundAndUseResponsesProtocol(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 10
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, tc := range []struct {
		name, body string
		upstream   bool
		sse        bool
	}{
		{"upstream HTTP error", `{"model":"gpt-4o","input":"hello","max_output_tokens":32}`, true, false},
		{"first SSE upstream error", `{"model":"gpt-4o","input":"hello","max_output_tokens":32,"stream":true}`, true, true},
		{"private file ID rejection", `{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_file","file_id":"PRIVATE_UNSUPPORTED_FILE_ID"}]}],"max_output_tokens":32}`, false, false},
		{"typed tool policy rejection", `{"model":"gpt-4o","input":"hello","max_output_tokens":32,"tools":[{"type":"function","name":"weather","parameters":{"type":"object","properties":{}}}],"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"weather"}]}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			common.RetryTimes = 0
			fetch := system_setting.GetFetchSetting()
			oldEnabled, oldPrivate := fetch.EnableSSRFProtection, fetch.AllowPrivateIp
			fetch.EnableSSRFProtection, fetch.AllowPrivateIp = false, true
			t.Cleanup(func() { fetch.EnableSSRFProtection, fetch.AllowPrivateIp = oldEnabled, oldPrivate })
			var mu sync.Mutex
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls++
				mu.Unlock()
				if tc.sse {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"fixture overload\"}}\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"fixture upstream rejection"}}`)
			}))
			t.Cleanup(upstream.Close)
			config := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/messages", Converter: responsesToClaudeHostConverter, Auth: &dto.AdvancedCustomRouteAuth{Type: dto.AdvancedCustomAuthTypeHeader, Name: "x-api-key", Value: "{api_key}"}}}}
			settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &config})
			require.NoError(t, err)
			channel := model.Channel{Id: 7479, Type: constant.ChannelTypeAdvancedCustom, Name: "responses-error", Key: "fixture-channel-key", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled, OtherSettings: string(settings)}
			require.NoError(t, channel.ValidateSettings())
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channel.Id, Enabled: true}).Error)
			response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/responses", tc.body, types.RelayFormatOpenAIResponses)
			expectedStatus := http.StatusBadRequest
			if tc.sse {
				expectedStatus = http.StatusBadGateway
			}
			require.Equal(t, expectedStatus, response.Code, response.Body.String())
			var body map[string]any
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			errorObject, ok := body["error"].(map[string]any)
			require.True(t, ok, response.Body.String())
			assert.NotEmpty(t, errorObject["message"])
			assert.NotContains(t, body, "type", "Responses errors use OpenAI error envelope, not Claude top-level type")
			assert.NotContains(t, response.Body.String(), "fixture-channel-key")
			assert.NotContains(t, response.Body.String(), "PRIVATE_UNSUPPORTED_FILE_ID")
			mu.Lock()
			observedCalls := calls
			mu.Unlock()
			if tc.upstream {
				assert.Equal(t, 1, observedCalls)
			} else {
				assert.Zero(t, observedCalls)
			}
			drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, service.WaitBillingRefunds(drain))
			var afterUser model.User
			var afterToken model.Token
			require.NoError(t, db.First(&afterUser, user.Id).Error)
			require.NoError(t, db.First(&afterToken, token.Id).Error)
			assert.Equal(t, user.Quota, afterUser.Quota)
			assert.Equal(t, token.RemainQuota, afterToken.RemainQuota)
			assert.Zero(t, afterUser.UsedQuota)
			var consumes int64
			require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&consumes).Error)
			assert.Zero(t, consumes)
		})
	}
}

func TestAdvancedResponsesToClaudeHostPreservesNativeResponsesRoute(t *testing.T) {
	db, user, token, _ := mediaConversionBillingFixture(t)
	fetch := system_setting.GetFetchSetting()
	oldEnabled, oldPrivate := fetch.EnableSSRFProtection, fetch.AllowPrivateIp
	fetch.EnableSSRFProtection, fetch.AllowPrivateIp = false, true
	t.Cleanup(func() { fetch.EnableSSRFProtection, fetch.AllowPrivateIp = oldEnabled, oldPrivate })
	var mu sync.Mutex
	var path, wire, key string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, wire, key = r.URL.Path, string(b), r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_native","object":"response","model":"gpt-4o","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"native hello"}]}],"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14}}`)
	}))
	t.Cleanup(upstream.Close)
	config := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/responses", Converter: "none"}}}
	settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &config})
	require.NoError(t, err)
	channel := model.Channel{Id: 7479, Type: constant.ChannelTypeAdvancedCustom, Name: "native", Key: "fixture-native-key", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled, OtherSettings: string(settings)}
	require.NoError(t, channel.ValidateSettings())
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channel.Id, Enabled: true}).Error)
	response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/responses", `{"model":"gpt-4o","input":"native request","max_output_tokens":32}`, types.RelayFormatOpenAIResponses)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	mu.Lock()
	seenPath, seenWire, seenKey := path, wire, key
	mu.Unlock()
	assert.Equal(t, "/v1/responses", seenPath)
	assert.Equal(t, "Bearer fixture-native-key", seenKey)
	var request map[string]any
	require.NoError(t, common.Unmarshal([]byte(seenWire), &request))
	assert.Equal(t, "native request", request["input"])
	assert.NotContains(t, request, "messages")
	assert.Contains(t, response.Body.String(), "native hello")
	assert.Contains(t, response.Body.String(), `"id":"resp_native"`)
}
