package controller

import (
	"bytes"
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func runReadinessHost(t *testing.T, user model.User, token model.Token, ch model.Channel, preference, path, contentType string, body []byte, configure func(*gin.Context, *lifecycleHostWriter)) (*gin.Context, *lifecycleHostWriter) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &lifecycleHostWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", contentType)
	c.Set(common.RequestIdKey, "b11-lifecycle-request")
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
	format := types.RelayFormat(types.RelayFormatOpenAI)
	if path == "/v1/responses" {
		format = types.RelayFormatOpenAIResponses
	}
	if strings.HasPrefix(path, "/v1/audio/") {
		format = types.RelayFormatOpenAIAudio
	}
	Relay(c, format)
	return c, w
}

func TestRelayValidatedLifecycleFlushesBeforeLateTextAndPreservesCommentOrder(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		t.Run(path, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			release := make(chan struct{})
			var once sync.Once
			var calls atomic.Int64
			first := `{"id":"chat_ready","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`
			text := `{"id":"chat_ready","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"later visible text"},"finish_reason":null}]}`
			terminal := `{"id":"chat_ready","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`
			if path == "/v1/responses" {
				first = cancelHostCreated
				text = `{"type":"response.output_text.delta","item_id":"item_ready","output_index":0,"content_index":0,"delta":"later visible text"}`
				terminal = `{"type":"response.completed","response":{"id":"resp_cancel","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`
			}
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
				_, _ = io.WriteString(w, ": PRIVATE_UPSTREAM_COMMENT\n\ndata: "+text+"\n\ndata: "+terminal+"\n\n")
				if path != "/v1/responses" {
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
				}
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			var guardExpired atomic.Bool
			guard := time.AfterFunc(3*time.Second, func() { guardExpired.Store(true); once.Do(func() { close(release) }) })
			defer guard.Stop()
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
			body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"PRIVATE_B12_PROMPT"}],"stream_options":{"include_usage":true}}`
			if path == "/v1/responses" {
				body = `{"model":"gpt-4o","stream":true,"input":"PRIVATE_B12_PROMPT"}`
			}
			var firstFlush atomic.Bool
			_, writer := runReadinessHost(t, user, token, ch, "wallet_only", path, "application/json", []byte(body), func(c *gin.Context, w *lifecycleHostWriter) {
				w.onFlush = func(p []byte) {
					if strings.Contains(string(p), "chat_ready") || strings.Contains(string(p), "response.created") {
						if !firstFlush.Load() {
							assert.NotContains(t, string(p), "later visible text")
							assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
							assert.Equal(t, 200, w.Code)
						}
						firstFlush.Store(true)
						once.Do(func() { close(release) })
					}
				}
			})
			assert.False(t, guardExpired.Load(), "validated lifecycle must flush without waiting for text")
			assert.True(t, firstFlush.Load())
			assert.Equal(t, int64(1), calls.Load())
			wire := writer.Body.String()
			assert.NotContains(t, wire, "PRIVATE_UPSTREAM_COMMENT")
			comment := strings.Index(wire, ": ")
			later := strings.Index(wire, "later visible text")
			assert.Greater(t, comment, 0, "data enables canonical comment keepalive")
			assert.Greater(t, later, comment, "comment remains before later data")
			waitCancellationHostWork(t)
			reserved("wallet_only")
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 10, logs[0].PromptTokens)
			assert.Equal(t, 2, logs[0].CompletionTokens)
			assertCancellationBalances(t, db, user, token, sub, "wallet_only", 18, "settled")
		})
	}
}

func TestRelayPreDataCommentsCannotCommitFirstErrorOrPreventRetry(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, retry := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/final", true: "/retry"}[retry], func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				general := operation_setting.GetGeneralSetting()
				oldPing, oldInterval := general.PingIntervalEnabled, general.PingIntervalSeconds
				general.PingIntervalEnabled, general.PingIntervalSeconds = true, 1
				t.Cleanup(func() { general.PingIntervalEnabled, general.PingIntervalSeconds = oldPing, oldInterval })
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					if n == 1 {
						_, _ = io.WriteString(w, ": PRIVATE_PRE_DATA_COMMENT\n\n")
						w.(http.Flusher).Flush()
						_, _ = io.WriteString(w, "data: {\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"fixture overload\"}}\n\n")
						return
					}
					_, _ = io.WriteString(w, "data: "+`{"id":"backup","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"backup success"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`+"\n\ndata: [DONE]\n\n")
				}))
				defer server.Close()
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
				oldRetry := common.RetryTimes
				common.RetryTimes = 0
				if retry {
					common.RetryTimes = 1
				}
				t.Cleanup(func() { common.RetryTimes = oldRetry })
				_, writer := runReadinessHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"PRIVATE_B12_PROMPT"}]}`), nil)
				assert.NotContains(t, writer.Body.String(), "PRIVATE_PRE_DATA_COMMENT")
				assert.NotContains(t, writer.Body.String(), ": keep-alive")
				quota, status := 0, "refunded"
				if retry {
					assert.Equal(t, 2, int(calls.Load()))
					assert.Equal(t, 200, writer.Code)
					assert.Contains(t, writer.Body.String(), "backup success")
					assert.Contains(t, writer.Header().Get("Content-Type"), "text/event-stream")
					quota, status = 18, "settled"
				} else {
					assert.Equal(t, int64(1), calls.Load())
					assert.Equal(t, 503, writer.Code)
					assert.Contains(t, writer.Header().Get("Content-Type"), "application/json")
					var body map[string]interface{}
					require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &body))
					require.NotNil(t, body["error"])
				}
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, quota, status)
			})
		}
	}
}
