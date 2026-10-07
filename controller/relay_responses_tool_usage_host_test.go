package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func toolUsageHostChannel(t *testing.T, db *gorm.DB, url string) model.Channel {
	t.Helper()
	fetch := system_setting.GetFetchSetting()
	oldEnabled, oldPrivate := fetch.EnableSSRFProtection, fetch.AllowPrivateIp
	fetch.EnableSSRFProtection, fetch.AllowPrivateIp = false, true
	t.Cleanup(func() { fetch.EnableSSRFProtection, fetch.AllowPrivateIp = oldEnabled, oldPrivate })
	ch := lifecycleHostChannel(t, db, url, constant.ChannelTypeAdvancedCustom)
	cfg := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/chat/completions", Converter: relayconvert.ConverterOpenAIResponsesToOpenAIChat}}}
	settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &cfg})
	require.NoError(t, err)
	ch.OtherSettings = string(settings)
	require.NoError(t, ch.ValidateSettings())
	require.NoError(t, db.Save(&ch).Error)
	return ch
}
func toolUsageHostEvents(t *testing.T, wire string) []map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	for _, line := range strings.Split(wire, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]interface{}
		require.NoError(t, common.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		events = append(events, event)
	}
	return events
}
func assertToolUsageOpenItems(t *testing.T, events []map[string]interface{}) {
	t.Helper()
	open := map[string]bool{}
	indices := map[string]interface{}{}
	closed := map[string]bool{}
	parts := map[string]bool{}
	deltas := map[string]string{}
	for _, event := range events {
		kind, _ := event["type"].(string)
		if kind == "response.output_item.added" {
			item, ok := event["item"].(map[string]interface{})
			require.True(t, ok)
			id, _ := item["id"].(string)
			require.NotEmpty(t, id)
			assert.False(t, open[id])
			assert.False(t, closed[id])
			open[id] = true
			indices[id] = event["output_index"]
			if item["type"] == "function_call" {
				name, _ := item["name"].(string)
				assert.NotEmpty(t, strings.TrimSpace(name), "empty function cannot be published")
			}
		}
		if kind == "response.content_part.added" || kind == "response.reasoning_summary_part.added" {
			id, _ := event["item_id"].(string)
			assert.True(t, open[id])
			assert.False(t, parts[id])
			parts[id] = true
		}
		if kind == "response.content_part.done" || kind == "response.reasoning_summary_part.done" {
			id, _ := event["item_id"].(string)
			assert.True(t, open[id])
			assert.True(t, parts[id])
			delete(parts, id)
		}
		if kind == "response.output_text.delta" || kind == "response.reasoning_summary_text.delta" {
			id, _ := event["item_id"].(string)
			assert.True(t, parts[id], "delta must reference opened content/summary part")
		}
		if strings.HasSuffix(kind, ".delta") {
			id, _ := event["item_id"].(string)
			assert.True(t, open[id], "delta must refer to opened item: %v", event)
			assert.Equal(t, indices[id], event["output_index"])
			if delta, ok := event["delta"].(string); ok {
				deltas[id] += delta
			}
		}
		if kind == "response.output_item.done" {
			item, ok := event["item"].(map[string]interface{})
			require.True(t, ok)
			id, _ := item["id"].(string)
			assert.True(t, open[id])
			assert.Equal(t, indices[id], event["output_index"])
			if item["type"] == "function_call" {
				assert.Equal(t, deltas[id], item["arguments"], "final args match exactly observed delta join")
			} else if content, ok := item["content"].([]interface{}); ok {
				text := ""
				for _, part := range content {
					p, ok := part.(map[string]interface{})
					if ok {
						if v, ok := p["text"].(string); ok {
							text += v
						}
					}
				}
				assert.Equal(t, deltas[id], text, "final text/summary matches deltas")
			}
			assert.False(t, parts[id], "part done precedes item done")
			delete(open, id)
			closed[id] = true
		}
		if kind == "response.completed" {
			assert.Empty(t, open)
			assert.Empty(t, parts)
		}
	}
}

func TestRelayResponsesPostFinishGenerationFailsWithoutClosedItemWriteOrRetry(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"reasoning", "text", "tool"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				oldRetry := common.RetryTimes
				common.RetryTimes = 2
				t.Cleanup(func() { common.RetryTimes = oldRetry })
				delta, late := `"reasoning_content":"round one reasoning"`, `"reasoning_content":"ILLEGAL_SECOND_REASONING"`
				if kind == "text" {
					delta, late = `"content":"round one text"`, `"content":"ILLEGAL_SECOND_TEXT"`
				}
				if kind == "tool" {
					delta, late = `"tool_calls":[{"index":0,"id":"call_closed","type":"function","function":{"name":"lookup","arguments":"{}"}}]`, `"tool_calls":[{"index":0,"function":{"arguments":"ILLEGAL_SECOND_ARGS"}}]`
				}
				release := make(chan struct{})
				var once sync.Once
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, "/v1/chat/completions", r.URL.Path)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+`{"id":"bad_rounds","model":"gpt-4o","choices":[{"index":0,"delta":{`+delta+`},"finish_reason":"tool_calls"}]}`+"\n\n")
					w.(http.Flusher).Flush()
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					_, _ = io.WriteString(w, "data: "+`{"id":"bad_rounds","model":"gpt-4o","choices":[{"index":0,"delta":{`+late+`},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				var expired atomic.Bool
				guard := time.AfterFunc(3*time.Second, func() { expired.Store(true); once.Do(func() { close(release) }) })
				defer guard.Stop()
				ch := toolUsageHostChannel(t, db, server.URL)
				_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/responses", "application/json", []byte(`{"model":"gpt-4o","stream":true,"input":"PRIVATE_B13_PROMPT"}`), func(c *gin.Context, w *lifecycleHostWriter) {
					w.onFlush = func(p []byte) {
						if strings.Contains(string(p), "response.output_item.done") {
							once.Do(func() { close(release) })
						}
					}
				})
				assert.False(t, expired.Load())
				assert.Equal(t, int64(1), calls.Load())
				wire := writer.Body.String()
				assert.NotContains(t, wire, "ILLEGAL_SECOND")
				assert.NotContains(t, wire, "PRIVATE_B13_PROMPT")
				assert.NotContains(t, wire, server.URL)
				assert.Equal(t, 1, strings.Count(wire, "event: response.failed\n"))
				assert.NotContains(t, wire, "event: response.completed\n")
				assert.NotContains(t, wire, "[DONE]")
				assertToolUsageOpenItems(t, toolUsageHostEvents(t, wire))
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, 0, "refunded")
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				assert.Empty(t, logs, "typed converter protocol error keeps original refund contract")
				once.Do(func() { close(release) })
			})
		}
	}
}

func TestRelayResponsesLateToolNameBuffersArgumentsAndCanReplay(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			release := make(chan struct{})
			var once sync.Once
			var calls atomic.Int64
			first := `{"id":"late_tool","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"prefix text","tool_calls":[{"index":0,"id":"call_replay","type":"function","function":{"arguments":"{\"city\":"}},{"index":1,"id":"call_nameless","type":"function","function":{"name":"　 \t","arguments":"{}"}}]}}]}`
			later := `{"id":"late_tool","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"\"Paris\"}"}}]}}]}`
			finish := `{"id":"late_tool","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`
			usage := `{"id":"late_tool","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"completion_tokens_details":{"reasoning_tokens":0,"text_tokens":2},"reasoning_tokens":398}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: "+first+"\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				for _, frame := range []string{later, finish, usage} {
					_, _ = io.WriteString(w, "data: "+frame+"\n\n")
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			var expired atomic.Bool
			guard := time.AfterFunc(3*time.Second, func() { expired.Store(true); once.Do(func() { close(release) }) })
			defer guard.Stop()
			ch := toolUsageHostChannel(t, db, server.URL)
			_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/responses", "application/json", []byte(`{"model":"gpt-4o","stream":true,"input":"PRIVATE_B13_PROMPT"}`), func(c *gin.Context, w *lifecycleHostWriter) {
				w.onFlush = func(p []byte) {
					if strings.Contains(string(p), "prefix text") {
						once.Do(func() { close(release) })
					}
				}
			})
			assert.False(t, expired.Load())
			assert.Equal(t, int64(1), calls.Load())
			wire := writer.Body.String()
			events := toolUsageHostEvents(t, wire)
			assertToolUsageOpenItems(t, events)
			var completed map[string]interface{}
			args := ""
			validAdded := 0
			for _, event := range events {
				if event["type"] == "response.function_call_arguments.delta" {
					assert.Equal(t, "call_replay", event["item_id"])
					delta, _ := event["delta"].(string)
					args += delta
				}
				if event["type"] == "response.output_item.added" {
					item := event["item"].(map[string]interface{})
					if item["type"] == "function_call" {
						validAdded++
						assert.Equal(t, "lookup", item["name"])
						assert.Equal(t, float64(1), event["output_index"])
					}
				}
				if event["type"] == "response.completed" {
					require.Nil(t, completed)
					completed = event["response"].(map[string]interface{})
				}
			}
			assert.Equal(t, 1, validAdded)
			assert.Equal(t, `{"city":"Paris"}`, args)
			assert.NotContains(t, wire, "call_nameless")
			assert.NotContains(t, wire, "[DONE]")
			require.NotNil(t, completed)
			responseUsage := completed["usage"].(map[string]interface{})
			details, ok := responseUsage["output_tokens_details"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, float64(0), details["reasoning_tokens"], "standard explicit0 defeats top398")
			assert.Equal(t, float64(2), details["text_tokens"])
			output := completed["output"].([]interface{})
			require.Len(t, output, 2)
			tool := output[1].(map[string]interface{})
			assert.Equal(t, "lookup", tool["name"])
			assert.Equal(t, `{"city":"Paris"}`, tool["arguments"])
			waitCancellationHostWork(t)
			reserved(preference)
			assertCancellationBalances(t, db, user, token, sub, preference, 18, "settled")
			t.Run("actual_replay", func(t *testing.T) { replayResponsesToolHost(t, preference, tool) })
			once.Do(func() { close(release) })
		})
	}
}

func replayResponsesToolHost(t *testing.T, preference string, tool map[string]interface{}) {
	db, user, token, sub := cancellationHostFixture(t, "openai")
	reserved := observeCancellationReservation(t, db, user, token, sub)
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		seen = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"replayed","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"tool replay success"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
	}))
	defer server.Close()
	ch := toolUsageHostChannel(t, db, server.URL)
	input := []interface{}{map[string]interface{}{"type": "function_call", "call_id": tool["call_id"], "name": tool["name"], "arguments": tool["arguments"]}, map[string]interface{}{"type": "function_call_output", "call_id": tool["call_id"], "output": "sunny"}}
	body, err := common.Marshal(map[string]interface{}{"model": "gpt-4o", "input": input})
	require.NoError(t, err)
	_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/responses", "application/json", body, nil)
	assert.Equal(t, 200, writer.Code)
	assert.Contains(t, writer.Body.String(), "tool replay success")
	assert.Contains(t, seen, `"name":"lookup"`)
	assert.Contains(t, seen, `"tool_call_id":"call_replay"`)
	assert.Contains(t, seen, "Paris")
	waitCancellationHostWork(t)
	reserved(preference)
	assertCancellationBalances(t, db, user, token, sub, preference, 18, "settled")
}

func TestRelayResponsesReasoningUsagePresenceAndNonstreamInvalidTools(t *testing.T) {
	cases := []struct {
		name, usage                          string
		reasoning, prompt, completion, quota int
	}{
		{"top_fallback", `{"prompt_tokens":10,"completion_tokens":500,"total_tokens":510,"reasoning_tokens":398}`, 398, 10, 500, 2010},
		{"standard_zero_priority", `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"reasoning_tokens":398,"completion_tokens_details":{"reasoning_tokens":0,"text_tokens":2},"prompt_tokens_details":{"cached_tokens":0}}`, 0, 10, 2, 18},
		{"standard_positive", `{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"reasoning_tokens":398,"completion_tokens_details":{"reasoning_tokens":3,"text_tokens":2}}`, 3, 10, 5, 30},
		{"details_absent", `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`, 0, 10, 2, 18},
		{"authoritative_all_zero", `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"completion_tokens_details":{"reasoning_tokens":0}}`, 0, 0, 0, 0},
		{"fallback_null", `null`, 0, 0, 3, 12},
		{"fallback_empty", `{}`, 0, 0, 3, 12},
	}
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range cases {
				if stream && strings.HasPrefix(tc.name, "fallback_") {
					continue
				}
				t.Run(preference+map[bool]string{false: "/nonstream/", true: "/stream/"}[stream]+tc.name, func(t *testing.T) {
					db, user, token, sub := cancellationHostFixture(t, "openai")
					reserved := observeCancellationReservation(t, db, user, token, sub)
					var calls atomic.Int64
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if !stream {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, `{"id":"usage_source","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"known answer","tool_calls":[{"id":"empty_call","type":"function","function":{"name":"","arguments":"{}"}},{"id":"space_call","type":"function","function":{"name":"　 \t","arguments":"{}"}},{"id":"valid_call","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":"tool_calls"}],"usage":`+tc.usage+`}`)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"id":"usage_source","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"known answer","reasoning_content":"visible reasoning"},"finish_reason":"stop"}]}`+"\n\ndata: "+`{"id":"usage_source","model":"gpt-4o","choices":[],"usage":`+tc.usage+`}`+"\n\ndata: [DONE]\n\n")
					}))
					defer server.Close()
					ch := toolUsageHostChannel(t, db, server.URL)
					body := `{"model":"gpt-4o","input":"PRIVATE_B13_PROMPT"}`
					if stream {
						body = `{"model":"gpt-4o","stream":true,"input":"PRIVATE_B13_PROMPT"}`
					}
					_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/responses", "application/json", []byte(body), nil)
					assert.Equal(t, int64(1), calls.Load())
					assert.Equal(t, 200, writer.Code)
					var response map[string]interface{}
					if stream {
						events := toolUsageHostEvents(t, writer.Body.String())
						assertToolUsageOpenItems(t, events)
						for _, event := range events {
							if event["type"] == "response.completed" {
								require.Nil(t, response)
								response = event["response"].(map[string]interface{})
							}
						}
						assert.NotContains(t, writer.Body.String(), "[DONE]")
					} else {
						require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &response))
						assert.NotContains(t, writer.Body.String(), "empty_call")
						assert.NotContains(t, writer.Body.String(), "space_call")
						assert.Contains(t, writer.Body.String(), "valid_call")
					}
					require.NotNil(t, response)
					usage := response["usage"].(map[string]interface{})
					details, ok := usage["output_tokens_details"].(map[string]interface{})
					require.True(t, ok, "reasoning detail object required, including absent/explicit0 upstream fields")
					reasoning, ok := details["reasoning_tokens"]
					require.True(t, ok)
					assert.Equal(t, float64(tc.reasoning), reasoning)
					assert.Equal(t, float64(tc.prompt), usage["input_tokens"])
					assert.Equal(t, float64(tc.completion), usage["output_tokens"])
					if tc.name == "standard_zero_priority" || tc.name == "standard_positive" {
						assert.Equal(t, float64(2), details["text_tokens"])
					}
					waitCancellationHostWork(t)
					reserved(preference)
					var logs []model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					assert.Equal(t, tc.prompt, logs[0].PromptTokens)
					assert.Equal(t, tc.completion, logs[0].CompletionTokens)
					assert.Equal(t, tc.quota, logs[0].Quota, "reasoning detail never adds new quota beyond recognized source counts")
					assertCancellationBalances(t, db, user, token, sub, preference, tc.quota, "settled")
				})
			}
		}
	}
}
