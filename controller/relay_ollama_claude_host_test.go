package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// b16RunHost goes through the actual Relay/controller and provider HTTP adapter;
// only authenticated request context is supplied by the isolated SQL fixture.
func b16RunHost(t *testing.T, user model.User, token model.Token, ch model.Channel, preference, path string, body []byte, configure func(*gin.Context, *lifecycleHostWriter)) (*gin.Context, *lifecycleHostWriter) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &lifecycleHostWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "b16-host-request")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: preference})
	c.Set("token_quota", token.RemainQuota)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, &ch, "gpt-4o"))
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	if configure != nil {
		configure(c, w)
	}
	format := types.RelayFormatOpenAI
	if path == "/v1/messages" {
		format = types.RelayFormatClaude
	}
	Relay(c, format)
	return c, w
}

func TestB16RelayOllamaClaudeCancellationAndRealDoneUsage(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"partial", "no_output", "done", "done_zero"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				frame := `{"message":{"content":"observed Claude generation"},"done":false}`
				needle := "observed Claude generation"
				if kind == "no_output" {
					needle = `"type":"message_start"`
				}
				if strings.HasPrefix(kind, "done") {
					frame = `{"message":{"content":"observed Claude generation"},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2}`
					needle = `"stop_reason":"end_turn"`
					if kind == "done_zero" {
						frame = strings.ReplaceAll(strings.ReplaceAll(frame, `"prompt_eval_count":10`, `"prompt_eval_count":0`), `"eval_count":2`, `"eval_count":0`)
					}
				}
				release, closed := make(chan struct{}), make(chan struct{})
				var once sync.Once
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/x-ndjson")
					_, _ = io.WriteString(w, frame+"\n")
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						close(closed)
					case <-release:
					}
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				var expired atomic.Bool
				guard := time.AfterFunc(3*time.Second, func() { expired.Store(true); once.Do(func() { close(release) }) })
				defer guard.Stop()
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				_, writer := b16RunHost(t, user, token, ch, preference, "/v1/messages", []byte(`{"model":"gpt-4o","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"PRIVATE_B16_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) { w.cancelOnFlush = needle })
				assert.False(t, expired.Load())
				assert.True(t, writer.cancelled.Load())
				assert.NotContains(t, writer.Body.String(), "[DONE]")
				assert.NotContains(t, writer.Body.String(), `"type":"message_stop"`)
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Error("cancel did not close actual Ollama request")
				}
				waitCancellationHostWork(t)
				reserved(preference)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				quota, status := 0, "settled"
				switch kind {
				case "partial":
					require.Len(t, logs, 1)
					assert.Equal(t, service.CountTextToken("observed Claude generation", "gpt-4o"), logs[0].CompletionTokens)
					assert.Positive(t, logs[0].Quota)
					quota = logs[0].Quota
				case "done":
					require.Len(t, logs, 1)
					assert.Equal(t, 10, logs[0].PromptTokens)
					assert.Equal(t, 2, logs[0].CompletionTokens)
					quota = 18
				case "no_output":
					status = "refunded"
				case "done_zero":
					for _, log := range logs {
						assert.Zero(t, log.PromptTokens)
						assert.Zero(t, log.CompletionTokens)
						assert.Zero(t, log.Quota)
					}
				}
				assertCancellationBalances(t, db, user, token, sub, preference, quota, status)
			})
		}
	}
}

func b16ClaudeEvents(t *testing.T, wire string) []map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	for _, line := range strings.Split(wire, "\n") {
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var event map[string]interface{}
		require.NoError(t, common.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		events = append(events, event)
	}
	return events
}

func TestB16RelayOllamaClaudeTextThinkingToolsAndUsage(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, stream := range []bool{false, true} {
			for _, zero := range []bool{false, true} {
				t.Run(preference+map[bool]string{false: "/json", true: "/stream"}[stream]+map[bool]string{false: "/known", true: "/zero"}[zero], func(t *testing.T) {
					db, user, token, sub := cancellationHostFixture(t, "openai")
					reserved := observeCancellationReservation(t, db, user, token, sub)
					frame := `{"model":"private-ollama-alias","message":{"role":"assistant","content":"visible answer","thinking":"visible thought","tool_calls":[{"function":{"name":"lookup","arguments":{"city":"Paris"}}},{"function":{"name":"weather","arguments":{"day":"Monday"}}}]},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2}`
					if zero {
						frame = strings.ReplaceAll(strings.ReplaceAll(frame, `"prompt_eval_count":10`, `"prompt_eval_count":0`), `"eval_count":2`, `"eval_count":0`)
					}
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						assert.Equal(t, "/api/chat", r.URL.Path)
						input, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						var req map[string]interface{}
						assert.NoError(t, common.Unmarshal(input, &req))
						assert.Equal(t, "private-ollama-alias", req["model"])
						assert.Equal(t, stream, req["stream"])
						w.Header().Set("Content-Type", "application/x-ndjson")
						_, _ = io.WriteString(w, frame+"\n")
					}))
					t.Cleanup(server.Close)
					ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
					ch.ModelMapping = common.GetPointer(`{"gpt-4o":"private-ollama-alias"}`)
					require.NoError(t, db.Save(&ch).Error)
					body, err := common.Marshal(map[string]interface{}{"model": "gpt-4o", "max_tokens": 128, "stream": stream, "messages": []interface{}{map[string]interface{}{"role": "user", "content": "PRIVATE_B16_PROMPT"}}})
					require.NoError(t, err)
					_, writer := b16RunHost(t, user, token, ch, preference, "/v1/messages", body, nil)
					assert.Equal(t, 200, writer.Code)
					assert.Equal(t, int32(1), calls.Load())
					assert.NotContains(t, writer.Body.String(), "private-ollama-alias")
					assert.NotContains(t, writer.Body.String(), "[DONE]")
					assert.Contains(t, writer.Body.String(), "visible answer")
					assert.Contains(t, writer.Body.String(), "visible thought")
					assert.Contains(t, writer.Body.String(), "lookup")
					assert.Contains(t, writer.Body.String(), "Paris")
					assert.Contains(t, writer.Body.String(), "weather")
					assert.Contains(t, writer.Body.String(), "Monday")
					assert.NotContains(t, writer.Body.String(), `"signature"`)
					input, output := float64(10), float64(2)
					if zero {
						input, output = 0, 0
					}
					if !stream {
						var response map[string]interface{}
						require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &response))
						assert.Equal(t, "message", response["type"])
						assert.Equal(t, "tool_use", response["stop_reason"])
						usage, ok := response["usage"].(map[string]interface{})
						require.True(t, ok)
						assert.Equal(t, input, usage["input_tokens"])
						assert.Equal(t, output, usage["output_tokens"])
					} else {
						starts, stops, terminals := 0, 0, 0
						var blockKinds []string
						openBlocks := map[float64]bool{}
						for _, event := range b16ClaudeEvents(t, writer.Body.String()) {
							switch event["type"] {
							case "content_block_start":
								index := event["index"].(float64)
								assert.Equal(t, float64(len(blockKinds)), index)
								assert.False(t, openBlocks[index])
								openBlocks[index] = true
								blockKinds = append(blockKinds, event["content_block"].(map[string]interface{})["type"].(string))
							case "content_block_delta":
								assert.True(t, openBlocks[event["index"].(float64)])
							case "content_block_stop":
								index := event["index"].(float64)
								assert.True(t, openBlocks[index])
								delete(openBlocks, index)
							case "message_start":
								starts++
							case "message_stop":
								stops++
							case "message_delta":
								terminals++
								assert.Equal(t, "tool_use", event["delta"].(map[string]interface{})["stop_reason"])
								usage := event["usage"].(map[string]interface{})
								assert.Equal(t, input, usage["input_tokens"])
								assert.Equal(t, output, usage["output_tokens"])
							}
						}
						assert.Equal(t, []string{"thinking", "text", "tool_use", "tool_use"}, blockKinds)
						assert.Empty(t, openBlocks)
						assert.Equal(t, 1, starts)
						assert.Equal(t, 1, terminals)
						assert.Equal(t, 1, stops)
					}
					waitCancellationHostWork(t)
					reserved(preference)
					quota := 18
					if zero {
						quota = 0
					}
					assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
				})
			}
		}
	}
}

func TestB16RelayOllamaClaudeTerminalAndFailureContracts(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, tc := range []struct {
			name, frame, stop string
			stream            bool
			code, quota       int
			status            string
		}{
			{"stop_json", `{"message":{"content":"answer"},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2}`, "end_turn", false, 200, 18, "settled"},
			{"length_json", `{"message":{"content":"answer"},"done":true,"done_reason":"length","prompt_eval_count":10,"eval_count":2}`, "max_tokens", false, 200, 18, "settled"},
			{"stop_stream", `{"message":{"content":"answer"},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2}`, "end_turn", true, 200, 18, "settled"},
			{"length_stream", `{"message":{"content":"answer"},"done":true,"done_reason":"length","prompt_eval_count":10,"eval_count":2}`, "max_tokens", true, 200, 18, "settled"},
			{"missing_done_json", `{"message":{"content":"answer"},"prompt_eval_count":10,"eval_count":2}`, "", false, 502, 0, "refunded"},
			{"error_json", `{"error":"PRIVATE_B16_PROVIDER_ERROR","done":true,"prompt_eval_count":10,"eval_count":2}`, "", false, 502, 0, "refunded"},
			{"partial_eof", `{"message":{"content":"answer"},"done":false}`, "", true, 200, 0, "settled"},
			{"provider_error_stream", `{"message":{"content":"answer"},"done":false}` + "\n" + `{"error":"PRIVATE_B16_PROVIDER_ERROR"}`, "", true, 200, 0, "settled"},
			{"decode_error_stream", `{"message":{"content":"answer"},"done":false}` + "\n" + `{invalid`, "", true, 200, 0, "refunded"},
		} {
			t.Run(preference+"/"+tc.name, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				common.RetryTimes = 0
				reserved := observeCancellationReservation(t, db, user, token, sub)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/x-ndjson")
					_, _ = io.WriteString(w, tc.frame+"\n")
				}))
				t.Cleanup(server.Close)
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				body, err := common.Marshal(map[string]interface{}{"model": "gpt-4o", "max_tokens": 128, "stream": tc.stream, "messages": []interface{}{map[string]interface{}{"role": "user", "content": "PRIVATE_B16_PROMPT"}}})
				require.NoError(t, err)
				c, writer := b16RunHost(t, user, token, ch, preference, "/v1/messages", body, nil)
				assert.Equal(t, tc.code, writer.Code)
				assert.NotContains(t, writer.Body.String(), "PRIVATE_B16_PROVIDER_ERROR")
				assert.NotContains(t, writer.Body.String(), "[DONE]")
				if tc.stop != "" {
					assert.Contains(t, writer.Body.String(), `"stop_reason":"`+tc.stop+`"`)
					if tc.stream {
						assert.Contains(t, writer.Body.String(), `"type":"message_stop"`)
					}
				} else {
					assert.NotContains(t, writer.Body.String(), `"type":"message_stop"`)
					assert.NotContains(t, writer.Body.String(), `"stop_reason":"end_turn"`)
					if tc.stream {
						assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponseFailed))
					}
				}
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, tc.quota, tc.status)
			})
		}
	}
}

// The new Claude strict-done rule must not change native OpenAI funding.
func TestB16RelayOllamaNativeNonstreamLegacyControl(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, frame := range []string{
			`{"message":{"content":"legacy answer"},"prompt_eval_count":10,"eval_count":2}`,
			`{"error":"PRIVATE_B16_PROVIDER_ERROR","done":true,"prompt_eval_count":10,"eval_count":2}`,
		} {
			t.Run(preference+map[bool]string{true: "/provider_error", false: "/missing_done"}[strings.Contains(frame, `"error"`)], func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/x-ndjson")
					_, _ = io.WriteString(w, frame+"\n")
				}))
				t.Cleanup(server.Close)
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				_, writer := b16RunHost(t, user, token, ch, preference, "/v1/chat/completions", []byte(`{"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"PRIVATE_B16_PROMPT"}]}`), nil)
				assert.Equal(t, 200, writer.Code)
				assert.Contains(t, writer.Body.String(), `"object":"chat.completion"`)
				assert.NotContains(t, writer.Body.String(), "PRIVATE_B16_PROVIDER_ERROR")
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, 18, "settled")
			})
		}
	}
}

func TestB16RelayOllamaClaudeNonContextWriteFailureKeepsHistoricalFunding(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, done := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/partial", true: "/done"}[done], func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				frame := `{"message":{"content":"writer fault payload"},"done":false}`
				quota := 0
				if done {
					frame = `{"message":{"content":"writer fault payload"},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2}`
					quota = 18
				}
				release, closed := make(chan struct{}), make(chan struct{})
				var once sync.Once
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/x-ndjson")
					_, _ = io.WriteString(w, frame+"\n")
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						close(closed)
					case <-release:
					}
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				var expired atomic.Bool
				guard := time.AfterFunc(3*time.Second, func() { expired.Store(true); once.Do(func() { close(release) }) })
				defer guard.Stop()
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				c, writer := b16RunHost(t, user, token, ch, preference, "/v1/messages", []byte(`{"model":"gpt-4o","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"PRIVATE_B16_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) { w.failWriteNeedle = "writer fault payload" })
				assert.False(t, expired.Load())
				assert.True(t, writer.failed.Load())
				assert.False(t, writer.cancelled.Load())
				assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponseFailed))
				assert.NotContains(t, writer.Body.String(), `"type":"message_stop"`)
				assert.NotContains(t, writer.Body.String(), "[DONE]")
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Error("write fault did not close actual Ollama request")
				}
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
			})
		}
	}
}
