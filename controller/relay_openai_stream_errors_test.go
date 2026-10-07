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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const embeddedOverloadFixture = `{"error":{"code":"server_is_overloaded","message":"fixture upstream overloaded"}}`

func runOpenAIErrorHost(t *testing.T, user model.User, token model.Token, channel model.Channel, preference string, stream bool, writer http.ResponseWriter) *gin.Context {
	t.Helper()
	return runProtocolErrorHost(t, user, token, channel, preference, stream, writer, types.RelayFormatOpenAI)
}

func runProtocolErrorHost(t *testing.T, user model.User, token model.Token, channel model.Channel, preference string, stream bool, writer http.ResponseWriter, format types.RelayFormat) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(writer)
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_REQUEST_SENTINEL"}]}`
	if stream {
		body = strings.TrimSuffix(body, "}") + `,"stream":true}`
	}
	path := "/v1/chat/completions"
	if format == types.RelayFormatClaude {
		path = "/v1/messages"
		body = `{"model":"gpt-4o","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"PRIVATE_REQUEST_SENTINEL"}]}`
	}
	if format == types.RelayFormatGemini {
		path = "/v1beta/models/gpt-4o:streamGenerateContent"
		body = `{"contents":[{"role":"user","parts":[{"text":"PRIVATE_REQUEST_SENTINEL"}]}]}`
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
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
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-4o"))
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	Relay(c, format)
	return c
}

// The callback observes actual SQL reservation debits before any refund, rather
// than proving only that final balances happen to equal their initial values.
func observeOpenAIErrorReservation(t *testing.T, db *gorm.DB, user model.User, token model.Token, sub model.UserSubscription) func(string) {
	t.Helper()
	var mu sync.Mutex
	minWallet, minToken, maxSub := int64(user.Quota), int64(token.RemainQuota), sub.AmountUsed
	var observationErr error
	require.NoError(t, db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("test:openai_error_reservation", func(tx *gorm.DB) {
		if tx.Error != nil {
			return
		}
		var value int64
		var err error
		switch tx.Statement.Table {
		case "users":
			err = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT quota FROM users WHERE id = ?", user.Id).Scan(&value).Error
		case "tokens":
			err = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT remain_quota FROM tokens WHERE id = ?", token.Id).Scan(&value).Error
		case "user_subscriptions":
			err = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT amount_used FROM user_subscriptions WHERE id = ?", sub.Id).Scan(&value).Error
		default:
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			observationErr = err
			return
		}
		switch tx.Statement.Table {
		case "users":
			if value < minWallet {
				minWallet = value
			}
		case "tokens":
			if value < minToken {
				minToken = value
			}
		case "user_subscriptions":
			if value > maxSub {
				maxSub = value
			}
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:openai_error_reservation")) })
	return func(preference string) {
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, service.WaitBillingRefunds(drain))
		mu.Lock()
		walletReserved, tokenReserved, subscriptionReserved, err := minWallet, minToken, maxSub, observationErr
		mu.Unlock()
		require.NoError(t, err)
		assert.Less(t, tokenReserved, int64(token.RemainQuota))
		if preference == "wallet_only" {
			assert.Less(t, walletReserved, int64(user.Quota))
		} else {
			assert.Greater(t, subscriptionReserved, sub.AmountUsed)
		}
		var afterUser model.User
		var afterToken model.Token
		var afterSub model.UserSubscription
		require.NoError(t, db.First(&afterUser, user.Id).Error)
		require.NoError(t, db.First(&afterToken, token.Id).Error)
		require.NoError(t, db.First(&afterSub, sub.Id).Error)
		assert.Equal(t, user.Quota, afterUser.Quota)
		assert.Equal(t, token.RemainQuota, afterToken.RemainQuota)
		assert.Equal(t, sub.AmountUsed, afterSub.AmountUsed)
		assert.Zero(t, afterUser.UsedQuota)
		assert.Zero(t, afterToken.UsedQuota)
		var paid int64
		require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&paid).Error)
		assert.Zero(t, paid)
		if preference == "subscription_only" {
			var receipts []model.SubscriptionPreConsumeRecord
			require.NoError(t, db.Find(&receipts).Error)
			require.Len(t, receipts, 1)
			assert.Equal(t, "refunded", receipts[0].Status)
			assert.Positive(t, receipts[0].PreConsumed)
		}
	}
}

func TestRelayOpenAIEmbeddedErrorsRefundRealReservations(t *testing.T) {
	oldErrorLog := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = oldErrorLog })
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, stream := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/JSON", true: "/first SSE"}[stream], func(t *testing.T) {
				db, user, token, sub := mediaConversionBillingFixture(t)
				common.RetryTimes = 0
				oldTimeout := constant.StreamingTimeout
				constant.StreamingTimeout = 10
				t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
				verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+embeddedOverloadFixture+"\n\ndata: [DONE]\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, embeddedOverloadFixture)
					}
				}))
				t.Cleanup(upstream.Close)
				channel := model.Channel{Id: 7399, Type: constant.ChannelTypeOpenAI, Key: "PRIVATE_KEY_SENTINEL", Name: "embedded-error", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
				require.NoError(t, db.Create(&channel).Error)
				response := httptest.NewRecorder()
				runOpenAIErrorHost(t, user, token, channel, preference, stream, response)
				require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
				assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
				var body struct {
					Error struct{ Code, Message string } `json:"error"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				assert.Equal(t, "server_is_overloaded", body.Error.Code)
				assert.Contains(t, body.Error.Message, "fixture upstream overloaded")
				assert.NotContains(t, response.Body.String(), "PRIVATE_REQUEST_SENTINEL")
				assert.NotContains(t, response.Body.String(), "PRIVATE_KEY_SENTINEL")
				assert.NotContains(t, response.Body.String(), "[DONE]")
				assert.Equal(t, int64(1), calls.Load())
				verifyRefund(preference)
				var log model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeError).First(&log).Error)
				assert.Contains(t, log.Other, "503")
			})
		}
	}
}

func TestRelayOpenAIInitialErrorsRetryToOneSuccessfulCharge(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"JSON embedded", "SSE embedded", "SSE to JSON", "HTTP 503"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := mediaConversionBillingFixture(t)
				oldTimeout := constant.StreamingTimeout
				constant.StreamingTimeout = 10
				t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
				var firstCalls, secondCalls atomic.Int64
				first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					firstCalls.Add(1)
					switch kind {
					case "SSE embedded", "SSE to JSON":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+embeddedOverloadFixture+"\n\n")
					case "HTTP 503":
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = io.WriteString(w, embeddedOverloadFixture)
					default:
						_, _ = io.WriteString(w, embeddedOverloadFixture)
					}
				}))
				t.Cleanup(first.Close)
				second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					secondCalls.Add(1)
					if kind == "SSE embedded" {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"id\":\"success\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"backup success\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"success","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"backup success"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
					}
				}))
				t.Cleanup(second.Close)
				initial := model.Channel{Id: 7399, Type: constant.ChannelTypeOpenAI, Key: "fixture", Name: "initial", BaseURL: common.GetPointer(first.URL), Status: common.ChannelStatusEnabled}
				backup := model.Channel{Id: 7400, Type: constant.ChannelTypeOpenAI, Key: "fixture", Name: "backup", BaseURL: common.GetPointer(second.URL), Status: common.ChannelStatusEnabled}
				require.NoError(t, db.Create(&initial).Error)
				require.NoError(t, db.Create(&backup).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: backup.Id, Enabled: true}).Error)
				response := httptest.NewRecorder()
				c := runOpenAIErrorHost(t, user, token, initial, preference, kind == "SSE embedded", response)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), "backup success")
				if kind == "SSE embedded" {
					assert.Contains(t, response.Header().Get("Content-Type"), "text/event-stream")
				} else {
					assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
					assert.NotContains(t, response.Body.String(), "data:")
				}
				assert.NotContains(t, response.Body.String(), "server_is_overloaded")
				assert.Equal(t, int64(1), firstCalls.Load())
				assert.Equal(t, int64(1), secondCalls.Load())
				assert.Equal(t, []string{"7399", "7400"}, c.GetStringSlice("use_channel"))
				var consumes []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&consumes).Error)
				require.Len(t, consumes, 1)
				assert.Equal(t, 10, consumes[0].PromptTokens)
				assert.Equal(t, 2, consumes[0].CompletionTokens)
				var afterUser model.User
				var afterToken model.Token
				require.NoError(t, db.First(&afterUser, user.Id).Error)
				require.NoError(t, db.First(&afterToken, token.Id).Error)
				if preference == "wallet_only" {
					assert.Equal(t, user.Quota-consumes[0].Quota, afterUser.Quota)
				} else {
					assert.Equal(t, user.Quota, afterUser.Quota)
					var afterSub model.UserSubscription
					require.NoError(t, db.First(&afterSub, sub.Id).Error)
					assert.Equal(t, sub.AmountUsed+int64(consumes[0].Quota), afterSub.AmountUsed)
					var receipts []model.SubscriptionPreConsumeRecord
					require.NoError(t, db.Find(&receipts).Error)
					require.Len(t, receipts, 1)
					assert.Equal(t, "settled", receipts[0].Status)
					assert.Positive(t, receipts[0].PreConsumed)
				}

				assert.Equal(t, token.RemainQuota-consumes[0].Quota, afterToken.RemainQuota)
				assert.Equal(t, consumes[0].Quota, afterUser.UsedQuota)
				assert.Equal(t, consumes[0].Quota, afterToken.UsedQuota)
			})
		}
	}
}

// A Flush gate makes the upstream's next frame depend on the client's actual
// first frame. Lag-by-one cannot pass by receiving an immediately queued frame.
type openAICommitRecorder struct {
	*httptest.ResponseRecorder
	flushed        chan struct{}
	once           sync.Once
	awaitPing      bool
	lastFlushedLen int
}

func (w *openAICommitRecorder) Flush() {
	w.ResponseRecorder.Flush()
	current := w.Body.Bytes()[w.lastFlushedLen:]
	w.lastFlushedLen = w.Body.Len()
	if (!w.awaitPing && len(current) > 0) || (w.awaitPing && bytes.Contains(current, []byte(": PING\n\n"))) {
		w.once.Do(func() { close(w.flushed) })
	}
}

func TestRelayOpenAICommittedStreamErrorHasOneProtocolTerminal(t *testing.T) {
	for _, scenario := range []struct {
		commit, preference string
		format             types.RelayFormat
	}{{"role", "wallet_only", types.RelayFormatOpenAI}, {"ping", "wallet_only", types.RelayFormatOpenAI}, {"role", "subscription_only", types.RelayFormatOpenAI}, {"role", "wallet_only", types.RelayFormatClaude}, {"role", "wallet_only", types.RelayFormatGemini}} {
		commit, preference := scenario.commit, scenario.preference
		t.Run(string(scenario.format)+"/"+commit+"/"+preference, func(t *testing.T) {
			db, user, token, sub := mediaConversionBillingFixture(t)
			verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 10
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			settings := operation_setting.GetGeneralSetting()
			oldEnabled, oldInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
			settings.PingIntervalEnabled, settings.PingIntervalSeconds = commit == "ping", 1
			t.Cleanup(func() { settings.PingIntervalEnabled, settings.PingIntervalSeconds = oldEnabled, oldInterval })
			response := &openAICommitRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}), awaitPing: commit == "ping"}
			var firstCalls, secondCalls atomic.Int64
			var gateFailed atomic.Bool
			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				firstCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				if commit == "role" && scenario.format == types.RelayFormatGemini {
					_, _ = io.WriteString(w, `data: {"id":"first","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"first visible"}}]}`+"\n\n")
					w.(http.Flusher).Flush()
				} else if commit == "role" || commit == "ping" {
					_, _ = io.WriteString(w, "data: {\"id\":\"first\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
					w.(http.Flusher).Flush()
				}
				timeout := time.NewTimer(3 * time.Second)
				defer timeout.Stop()
				select {
				case <-response.flushed:
				case <-timeout.C:
					gateFailed.Store(true)
				case <-r.Context().Done():
					return
				}
				_, _ = io.WriteString(w, "data: "+embeddedOverloadFixture+"\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(first.Close)
			second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { secondCalls.Add(1); w.WriteHeader(500) }))
			t.Cleanup(second.Close)
			initial := model.Channel{Id: 7399, Type: constant.ChannelTypeOpenAI, Key: "fixture", Name: "committed", BaseURL: common.GetPointer(first.URL), Status: common.ChannelStatusEnabled}
			backup := model.Channel{Id: 7400, Type: constant.ChannelTypeOpenAI, Key: "fixture", Name: "must-not-retry", BaseURL: common.GetPointer(second.URL), Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&initial).Error)
			require.NoError(t, db.Create(&backup).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: backup.Id, Enabled: true}).Error)
			runProtocolErrorHost(t, user, token, initial, preference, true, response, scenario.format)
			assert.False(t, gateFailed.Load(), "first role or readiness-gated actual timer ping must flush before second data frame")
			if commit == "ping" {
				rolePos := strings.Index(response.Body.String(), `"role":"assistant"`)
				pingPos := strings.Index(response.Body.String(), ": PING\n\n")
				assert.GreaterOrEqual(t, rolePos, 0)
				assert.Greater(t, pingPos, rolePos, "actual business frame precedes timer keepalive")
			}
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, int64(1), firstCalls.Load())
			assert.Zero(t, secondCalls.Load())
			var errorsSeen int
			for _, line := range strings.Split(response.Body.String(), "\n") {
				if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
					continue
				}
				require.True(t, strings.HasPrefix(line, "data: "), "no bare HTTP JSON after commit: %s", line)
				payload := strings.TrimPrefix(line, "data: ")
				require.NotEqual(t, "[DONE]", payload)
				var event map[string]any
				require.NoError(t, common.Unmarshal([]byte(payload), &event))
				if _, ok := event["error"]; ok {
					errorsSeen++
				}
			}
			assert.Equal(t, 1, errorsSeen, response.Body.String())
			assert.NotContains(t, response.Body.String(), "PRIVATE_REQUEST_SENTINEL")
			verifyRefund(preference)
		})
	}
}

func TestRelayOpenAIQuotedErrorAndToolArgumentsAreNormalPayload(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "JSON", true: "SSE"}[stream], func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 10
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			message := map[string]any{"role": "assistant", "content": `text {"error":{"code":"server_is_overloaded"}}`, "tool_calls": []any{map[string]any{"index": 0, "id": "call_fixture", "type": "function", "function": map[string]any{"name": "inspect", "arguments": `{"error":"ordinary tool input"}`}}}}
			choice := map[string]any{"index": 0, "finish_reason": "tool_calls"}
			if stream {
				choice["delta"] = message
			} else {
				choice["message"] = message
			}
			body, err := common.Marshal(map[string]any{"id": "success", "model": "gpt-4o", "choices": []any{choice}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
			require.NoError(t, err)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+string(body)+"\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(body)
				}
			}))
			t.Cleanup(upstream.Close)
			channel := model.Channel{Id: 7399, Type: constant.ChannelTypeOpenAI, Key: "fixture", Name: "ordinary-payload", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			response := httptest.NewRecorder()
			runOpenAIErrorHost(t, user, token, channel, "wallet_only", stream, response)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, int64(1), calls.Load())
			assert.Contains(t, response.Body.String(), "ordinary tool input")
			if stream {
				assert.Equal(t, 1, strings.Count(response.Body.String(), "data: [DONE]"))
			}
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 10, logs[0].PromptTokens)
			assert.Equal(t, 2, logs[0].CompletionTokens)
		})
	}
}
