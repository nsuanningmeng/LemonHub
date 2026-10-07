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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRelayResponsesActualTransportAbortHasOneFailedTerminalAndEndpointFee(t *testing.T) {
	for _, route := range []string{"native", "chat", "advanced_claude"} {
		for _, fault := range []string{"eof", "reset", "timeout", "completed_reset", "incomplete_reset"} {
			for _, preference := range []string{"wallet_only", "subscription_only"} {
				t.Run(route+"/"+fault+"/"+preference, func(t *testing.T) {
					db, user, token, sub := cancellationHostFixture(t, "openai")
					reserved := observeCancellationReservation(t, db, user, token, sub)
					oldRetry := common.RetryTimes
					common.RetryTimes = 2
					t.Cleanup(func() { common.RetryTimes = oldRetry })
					fetch := system_setting.GetFetchSetting()
					oldEnabled, oldPrivate := fetch.EnableSSRFProtection, fetch.AllowPrivateIp
					fetch.EnableSSRFProtection, fetch.AllowPrivateIp = false, true
					t.Cleanup(func() { fetch.EnableSSRFProtection, fetch.AllowPrivateIp = oldEnabled, oldPrivate })
					release := make(chan struct{})
					var once sync.Once
					var calls atomic.Int64
					frames := []string{cancelHostCreated, `{"type":"response.output_text.delta","item_id":"item_abort","output_index":0,"content_index":0,"delta":"served partial text"}`}
					if route == "chat" {
						frames = []string{`{"id":"chat_abort","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"served partial text"},"finish_reason":null}]}`}
					}
					if route == "claude" || route == "advanced_claude" {
						frames = []string{`{"type":"message_start","message":{"id":"msg_abort","type":"message","role":"assistant","model":"gpt-4o","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"served partial text"}}`}
					}
					if fault == "completed_reset" || fault == "incomplete_reset" {
						switch route {
						case "native":
							frames = append(frames, `{"type":"response.completed","response":{"id":"resp_cancel","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`)
						case "chat":
							frames = append(frames, `{"id":"chat_abort","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
						default:
							frames = append(frames, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`, `{"type":"message_stop"}`)
						}
					}
					if fault == "incomplete_reset" {
						for i := range frames {
							frames[i] = strings.ReplaceAll(frames[i], "response.completed", "response.incomplete")
							frames[i] = strings.ReplaceAll(frames[i], `"status":"completed"`, `"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}`)
							frames[i] = strings.ReplaceAll(frames[i], `"finish_reason":"stop"`, `"finish_reason":"length"`)
							frames[i] = strings.ReplaceAll(frames[i], `"stop_reason":"end_turn"`, `"stop_reason":"max_tokens"`)
						}
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range frames {
							_, _ = io.WriteString(w, "data: "+frame+"\n\n")
						}
						w.(http.Flusher).Flush()
						if fault == "timeout" {
							select {
							case <-r.Context().Done():
							case <-release:
							}
							return
						}
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						if fault == "reset" || fault == "completed_reset" || fault == "incomplete_reset" {
							conn, _, err := w.(http.Hijacker).Hijack()
							if err == nil {
								_ = conn.Close()
							}
						}
					}))
					t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
					kind := constant.ChannelTypeOpenAI
					if route == "claude" {
						kind = constant.ChannelTypeAnthropic
					}
					if route == "chat" || route == "advanced_claude" {
						kind = constant.ChannelTypeAdvancedCustom
					}
					ch := lifecycleHostChannel(t, db, server.URL, kind)
					if kind == constant.ChannelTypeAdvancedCustom {
						converter, upstream := relayconvert.ConverterOpenAIResponsesToOpenAIChat, "/v1/chat/completions"
						if route == "advanced_claude" {
							converter, upstream = responsesToClaudeHostConverter, "/v1/messages"
						}
						cfg := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: upstream, Converter: converter}}}
						settings, err := common.Marshal(dto.ChannelOtherSettings{AdvancedCustom: &cfg})
						require.NoError(t, err)
						ch.OtherSettings = string(settings)
						require.NoError(t, ch.ValidateSettings())
						require.NoError(t, db.Save(&ch).Error)
					}
					var deadlock atomic.Bool
					guard := time.AfterFunc(5*time.Second, func() { deadlock.Store(true); once.Do(func() { close(release) }) })
					defer guard.Stop()
					_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/responses", "application/json", []byte(`{"model":"gpt-4o","stream":true,"max_output_tokens":32,"input":"PRIVATE_B12_PROMPT"}`), func(c *gin.Context, w *lifecycleHostWriter) {
						w.onFlush = func(p []byte) {
							if fault != "timeout" && strings.Contains(string(p), "served partial text") {
								once.Do(func() { close(release) })
							}
						}
					})
					assert.False(t, deadlock.Load())
					assert.Equal(t, int64(1), calls.Load(), "committed abort must never retry")
					wire := writer.Body.String()
					assert.Contains(t, wire, "served partial text")
					if fault == "completed_reset" || fault == "incomplete_reset" {
						assert.Zero(t, strings.Count(wire, "event: response.failed\n"), wire)
						terminalType := "response.completed"
						if fault == "incomplete_reset" {
							terminalType = "response.incomplete"
						}
						assert.Equal(t, 1, strings.Count(wire, "event: "+terminalType+"\n"), wire)
					} else {
						assert.Equal(t, 1, strings.Count(wire, "event: response.failed\n"), wire)
						assert.NotContains(t, wire, "event: response.completed\n")
					}
					if fault != "completed_reset" && fault != "incomplete_reset" {
						for _, line := range strings.Split(wire, "\n") {
							if !strings.HasPrefix(line, "data: ") {
								continue
							}
							var event map[string]interface{}
							if common.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || event["type"] != "response.failed" {
								continue
							}
							response, ok := event["response"].(map[string]interface{})
							require.True(t, ok)
							assert.Equal(t, "failed", response["status"])
							assert.NotEmpty(t, response["id"])
							assert.Equal(t, "gpt-4o", response["model"])
							apierr, ok := response["error"].(map[string]interface{})
							require.True(t, ok)
							assert.Equal(t, "server_error", apierr["code"])
							assert.NotEmpty(t, apierr["message"])
						}
					}
					assert.NotContains(t, wire, "[DONE]")
					assert.NotContains(t, wire, "PRIVATE_B12_PROMPT")
					assert.NotContains(t, wire, server.URL)
					waitCancellationHostWork(t)
					reserved(preference)
					var logs []model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
					if fault != "completed_reset" && fault != "incomplete_reset" && (route == "claude" || route == "advanced_claude") {
						assert.Empty(t, logs)
						assertCancellationBalances(t, db, user, token, sub, preference, 0, "refunded")
					} else {
						require.Len(t, logs, 1, "native/Chat conversion retains original usage,nil fee contract")
						assertCancellationBalances(t, db, user, token, sub, preference, logs[0].Quota, "settled")
					}
					once.Do(func() { close(release) })
				})
			}
		}
	}
}

func TestRelayNativeRealFailedTerminalIsNotDuplicatedByReset(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			release := make(chan struct{})
			var once sync.Once
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range []string{cancelHostCreated, `{"type":"response.failed","response":{"id":"resp_cancel","model":"gpt-4o","status":"failed","error":{"code":"source_failure","message":"source failure retained"},"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`} {
					_, _ = io.WriteString(w, "data: "+frame+"\n\n")
				}
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			guard := time.AfterFunc(3*time.Second, func() { once.Do(func() { close(release) }) })
			defer guard.Stop()
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
			_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/responses", "application/json", []byte(`{"model":"gpt-4o","stream":true,"input":"PRIVATE_B12_PROMPT"}`), func(c *gin.Context, w *lifecycleHostWriter) {
				w.onFlush = func(p []byte) {
					if strings.Contains(string(p), "source_failure") {
						once.Do(func() { close(release) })
					}
				}
			})
			wire := writer.Body.String()
			assert.Equal(t, int64(1), calls.Load())
			assert.Equal(t, 1, strings.Count(wire, `"type":"response.failed"`))
			assert.Contains(t, wire, "source failure retained")
			assert.NotContains(t, wire, "upstream stream interrupted")
			assert.NotContains(t, wire, "response.completed")
			waitCancellationHostWork(t)
			reserved(preference)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 18, logs[0].Quota)
			assertCancellationBalances(t, db, user, token, sub, preference, 18, "settled")
			once.Do(func() { close(release) })
		})
	}
}
