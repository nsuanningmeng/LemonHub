package controller

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var cancellationHostChannelID atomic.Int64

type cancellationHostWriter struct {
	*httptest.ResponseRecorder
	needle       string
	cancel       context.CancelFunc
	once         sync.Once
	beforeCancel func()
	cancelled    atomic.Bool
	failNeedle   string
	shortWrite   bool
	failed       atomic.Bool
}

func (w *cancellationHostWriter) Write(p []byte) (int, error) {
	if w.failNeedle != "" && strings.Contains(string(p), w.failNeedle) {
		w.failed.Store(true)
		if w.shortWrite {
			return len(p) - 1, nil
		}
		return 0, errors.New("fixture downstream pipe closed")
	}
	return w.ResponseRecorder.Write(p)
}

func (w *cancellationHostWriter) Flush() {
	w.ResponseRecorder.Flush()
	if w.needle != "" && strings.Contains(w.Body.String(), w.needle) {
		w.once.Do(func() {
			if w.beforeCancel != nil {
				w.beforeCancel()
			}
			w.cancelled.Store(true)
			w.cancel()
		})
	}
}

// The request remains a real Relay request. Cancellation is triggered only
// after the target upstream-derived frame has reached a downstream Flush.
func runCancellationHost(t *testing.T, user model.User, token model.Token, channel model.Channel, preference, pathKind, needle string) (*gin.Context, *cancellationHostWriter) {
	t.Helper()
	return runCancellationHostWithSignal(t, user, token, channel, preference, pathKind, needle, nil)
}

func runCancellationHostWithSignal(t *testing.T, user model.User, token model.Token, channel model.Channel, preference, pathKind, needle string, upstreamClosed <-chan struct{}) (*gin.Context, *cancellationHostWriter) {
	return runCancellationHostWithInstrument(t, user, token, channel, preference, pathKind, needle, upstreamClosed, nil)
}

func runCancellationHostWithInstrument(t *testing.T, user model.User, token model.Token, channel model.Channel, preference, pathKind, needle string, upstreamClosed <-chan struct{}, ready func(*cancellationHostWriter)) (*gin.Context, *cancellationHostWriter) {
	t.Helper()
	reqctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &cancellationHostWriter{ResponseRecorder: httptest.NewRecorder(), needle: needle, cancel: cancel}
	watchStop, watchDone := make(chan struct{}), make(chan struct{})
	if upstreamClosed != nil {
		go func() {
			defer close(watchDone)
			select {
			case <-upstreamClosed:
				writer.once.Do(func() { writer.cancelled.Store(true); cancel() })
			case <-watchStop:
			}
		}()
		defer close(watchStop)
	}
	if ready != nil {
		ready(writer)
	}
	c, _ := gin.CreateTestContext(writer)
	path, body, format := "/v1/responses", `{"model":"gpt-4o","stream":true,"input":"PRIVATE_CANCEL_PROMPT"}`, types.RelayFormat(types.RelayFormatOpenAIResponses)
	if pathKind == "chat" || pathKind == "openai" {
		path, body, format = "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"PRIVATE_CANCEL_PROMPT"}]}`, types.RelayFormatOpenAI
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(reqctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "cancel-host-request")
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
	if upstreamClosed != nil {
		select {
		case <-watchDone:
		case <-time.After(3 * time.Second):
			t.Error("native timeout did not close its upstream response body")
		}
	}
	return c, writer
}

func cancellationHostFixture(t *testing.T, pathKind string) (*gorm.DB, model.User, model.Token, model.UserSubscription) {
	t.Helper()
	db, user, token, sub := mediaConversionBillingFixture(t)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	perfSetting := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
	oldPerf := *perfSetting
	perfSetting.Enabled, perfSetting.ErrorCodeWhitelist = true, ""
	t.Cleanup(func() { *perfSetting = oldPerf })
	oldError, oldTimeout := constant.ErrorLogEnabled, constant.StreamingTimeout
	constant.ErrorLogEnabled, constant.StreamingTimeout = true, 2
	oldPing := operation_setting.GetGeneralSetting().PingIntervalEnabled
	operation_setting.GetGeneralSetting().PingIntervalEnabled = false
	t.Cleanup(func() { operation_setting.GetGeneralSetting().PingIntervalEnabled = oldPing })
	t.Cleanup(func() { constant.ErrorLogEnabled, constant.StreamingTimeout = oldError, oldTimeout })
	model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: pathKind == "chat", AllChannels: true, ModelPatterns: []string{"^gpt-4o$"}}
	return db, user, token, sub
}

func waitCancellationHostWork(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.WaitBillingRefunds(ctx))
	deadline := time.Now().Add(5 * time.Second)
	for gopool.WorkerCount() > 0 {
		require.True(t, time.Now().Before(deadline), "async billing/metric worker did not drain")
		runtime.Gosched()
	}
}

func cancellationHostChannel(t *testing.T, db *gorm.DB, url string) model.Channel {
	t.Helper()
	id := 706200 + int(cancellationHostChannelID.Add(1))
	ch := model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Name: "cancel-host", Key: "PRIVATE_CANCEL_KEY", BaseURL: common.GetPointer(url), Status: common.ChannelStatusEnabled, AutoBan: common.GetPointer(1)}
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: ch.Id, Enabled: true}).Error)
	return ch
}

// A real SQL transaction callback records reservation evidence before any
// settlement/refund. Final balances alone cannot prove that funds were reserved.
func observeCancellationReservation(t *testing.T, db *gorm.DB, user model.User, token model.Token, sub model.UserSubscription) func(string) {
	t.Helper()
	var mu sync.Mutex
	wallet, remaining, used := int64(user.Quota), int64(token.RemainQuota), sub.AmountUsed
	var observationErr error
	require.NoError(t, db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("test:cancel_host_preconsume", func(tx *gorm.DB) {
		if tx.Error != nil {
			return
		}
		var v int64
		var e error
		switch tx.Statement.Table {
		case "users":
			e = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT quota FROM users WHERE id = ?", user.Id).Scan(&v).Error
		case "tokens":
			e = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT remain_quota FROM tokens WHERE id = ?", token.Id).Scan(&v).Error
		case "user_subscriptions":
			e = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT amount_used FROM user_subscriptions WHERE id = ?", sub.Id).Scan(&v).Error
		default:
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if e != nil {
			observationErr = e
			return
		}
		switch tx.Statement.Table {
		case "users":
			wallet = min(wallet, v)
		case "tokens":
			remaining = min(remaining, v)
		case "user_subscriptions":
			used = max(used, v)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:cancel_host_preconsume")) })
	return func(preference string) {
		mu.Lock()
		defer mu.Unlock()
		require.NoError(t, observationErr)
		assert.Less(t, remaining, int64(token.RemainQuota))
		if preference == "wallet_only" {
			assert.Less(t, wallet, int64(user.Quota))
		} else {
			assert.Greater(t, used, sub.AmountUsed)
		}
	}
}

func assertCancellationBalances(t *testing.T, db *gorm.DB, user model.User, token model.Token, sub model.UserSubscription, preference string, quota int, receiptStatus string) {
	t.Helper()
	waitCancellationHostWork(t)
	var u model.User
	var tok model.Token
	var s model.UserSubscription
	require.NoError(t, db.First(&u, user.Id).Error)
	require.NoError(t, db.First(&tok, token.Id).Error)
	require.NoError(t, db.First(&s, sub.Id).Error)
	assert.Equal(t, token.RemainQuota-quota, tok.RemainQuota)
	assert.Equal(t, quota, tok.UsedQuota)
	assert.Equal(t, quota, u.UsedQuota)
	if preference == "wallet_only" {
		assert.Equal(t, user.Quota-quota, u.Quota)
		assert.Equal(t, sub.AmountUsed, s.AmountUsed)
	} else {
		assert.Equal(t, user.Quota, u.Quota)
		assert.Equal(t, sub.AmountUsed+int64(quota), s.AmountUsed)
		var receipts []model.SubscriptionPreConsumeRecord
		require.NoError(t, db.Find(&receipts).Error)
		require.Len(t, receipts, 1)
		assert.Positive(t, receipts[0].PreConsumed)
		assert.Equal(t, receiptStatus, receipts[0].Status)
	}
}

const cancelHostCreated = `{"type":"response.created","response":{"id":"resp_cancel","model":"gpt-4o","created_at":1710000000}}`
const cancelHostText = `{"type":"response.output_text.delta","output_index":0,"item_id":"msg_cancel","delta":"visible generated answer"}`

func TestRelayCancellationCompletedUsageSettlesBeforeUpstreamEOF(t *testing.T) {
	for _, path := range []string{"responses", "chat"} {
		for _, preference := range []string{"wallet_only", "subscription_only"} {
			for _, zero := range []bool{false, true} {
				t.Run(path+"/"+preference+map[bool]string{false: "/exact", true: "/zero"}[zero], func(t *testing.T) {
					db, user, token, sub := cancellationHostFixture(t, path)
					reserved := observeCancellationReservation(t, db, user, token, sub)
					release := make(chan struct{})
					var releaseOnce sync.Once
					t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
					var calls atomic.Int64

					terminal := `{"type":"response.completed","response":{"id":"resp_cancel","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`
					if zero {
						terminal = `{"type":"response.completed","response":{"id":"resp_cancel","status":"completed","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						assert.Equal(t, "/v1/responses", r.URL.Path, "actual Chat policy must reach Responses upstream")
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range []string{cancelHostCreated, cancelHostText, terminal} {
							_, _ = io.WriteString(w, "data: "+frame+"\n\n")
							w.(http.Flusher).Flush()
						}
						select {
						case <-release:
						case <-r.Context().Done():

						}
					}))
					t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); server.Close() })
					ch := cancellationHostChannel(t, db, server.URL)
					needle := "response.completed"
					if path == "chat" {
						needle = `"finish_reason":"stop"`
					}
					_, writer := runCancellationHost(t, user, token, ch, preference, path, needle)
					assert.True(t, writer.cancelled.Load(), "client must cancel only after receiving its terminal")
					assert.Equal(t, int64(1), calls.Load())
					assert.Equal(t, 200, writer.Code)
					waitCancellationHostWork(t)
					reserved(preference)
					var logs []model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
					quota := 0
					if zero {
						for _, log := range logs {
							assert.Zero(t, log.Quota)
							assert.Zero(t, log.PromptTokens)
							assert.Zero(t, log.CompletionTokens)
						}
					} else {
						require.Len(t, logs, 1)
						assert.Equal(t, 10, logs[0].PromptTokens)
						assert.Equal(t, 2, logs[0].CompletionTokens)
						assert.Equal(t, 18, logs[0].Quota, "known 10 input + 2 output at ratio 1/4 charges exactly once")
						quota = logs[0].Quota
						assert.Contains(t, logs[0].Other, `"status":"ok"`)
						assert.NotContains(t, logs[0].Other, "client_gone")
					}
					assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
					metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
					require.NoError(t, err)
					require.Len(t, metrics.Models, 1)
					assert.Equal(t, int64(1), metrics.Models[0].RequestCount)
					assert.Equal(t, 100.0, metrics.Models[0].SuccessRate, "completed stays normal even when client closes at terminal")
					releaseOnce.Do(func() { close(release) })

				})
			}
		}
	}
}

func TestRelayPureCancellationUsesObservedGenerationAndNeverInventsPromptCharge(t *testing.T) {
	for _, path := range []string{"responses", "chat"} {
		for _, preference := range []string{"wallet_only", "subscription_only"} {
			for _, kind := range []string{"text", "tool", "reasoning", "created"} {
				t.Run(path+"/"+preference+"/"+kind, func(t *testing.T) {
					db, user, token, sub := cancellationHostFixture(t, path)
					oldAutoBan, oldDisableRanges := common.AutomaticDisableChannelEnabled, operation_setting.AutomaticDisableStatusCodeRanges
					common.AutomaticDisableChannelEnabled = true
					operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 499, End: 599}}
					t.Cleanup(func() {
						waitCancellationHostWork(t)
						common.AutomaticDisableChannelEnabled = oldAutoBan
						operation_setting.AutomaticDisableStatusCodeRanges = oldDisableRanges
					})
					reserved := observeCancellationReservation(t, db, user, token, sub)
					release := make(chan struct{})
					var once sync.Once
					t.Cleanup(func() { once.Do(func() { close(release) }) })
					var calls atomic.Int64
					frames := []string{cancelHostCreated}
					needle := "response.created"
					if path == "chat" {
						needle = `"role":"assistant"`
					}
					switch kind {
					case "text":
						frames = append(frames, cancelHostText)
						needle = "visible generated answer"
					case "tool":
						frames = append(frames, `{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_cancel","call_id":"call_cancel","name":"lookup"}}`, `{"type":"response.function_call_arguments.delta","output_index":1,"item_id":"fc_cancel","delta":"{\"query\":\"observed tool generation\"}"}`)
						needle = "observed tool generation"
					case "reasoning":
						frames = append(frames, `{"type":"response.reasoning_summary_text.delta","output_index":2,"item_id":"reason_cancel","delta":"observed reasoning generation"}`)
						needle = "observed reasoning generation"
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						assert.Equal(t, "/v1/responses", r.URL.Path, "actual Chat policy must reach Responses upstream")
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range frames {
							_, _ = io.WriteString(w, "data: "+frame+"\n\n")
							w.(http.Flusher).Flush()
						}
						select {
						case <-release:
						case <-r.Context().Done():

						}
					}))
					t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
					ch := cancellationHostChannel(t, db, server.URL)
					_, writer := runCancellationHost(t, user, token, ch, preference, path, needle)
					assert.True(t, writer.cancelled.Load())
					assert.Equal(t, int64(1), calls.Load())
					waitCancellationHostWork(t)
					reserved(preference)
					var logs []model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
					quota, status := 0, "refunded"
					if kind == "created" {
						for _, log := range logs {
							assert.Zero(t, log.Quota)
							assert.Zero(t, log.PromptTokens)
							assert.Zero(t, log.CompletionTokens)
						}
					} else {
						require.Len(t, logs, 1)
						assert.Positive(t, logs[0].CompletionTokens)
						expectedText := map[string]string{"text": "visible generated answer", "tool": `{"query":"observed tool generation"}`, "reasoning": "observed reasoning generation"}[kind]
						assert.Equal(t, service.CountTextToken(expectedText, "gpt-4o"), logs[0].CompletionTokens, "only observed output is estimated once")
						assert.Positive(t, logs[0].Quota)
						quota, status = logs[0].Quota, "settled"
					}
					assertCancellationBalances(t, db, user, token, sub, preference, quota, status)
					var audits []model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&audits).Error)
					require.Len(t, audits, 1)
					assert.Contains(t, audits[0].Other, "downstream_cancelled")
					assert.NotContains(t, audits[0].Other, "PRIVATE_CANCEL_PROMPT")
					assert.NotContains(t, audits[0].Other, "PRIVATE_CANCEL_KEY")
					var after model.Channel
					require.NoError(t, db.First(&after, ch.Id).Error)
					assert.Equal(t, common.ChannelStatusEnabled, after.Status)
					once.Do(func() { close(release) })
				})
			}
		}
	}
}

func TestRelayPureCancellationDoesNotEnterPerformanceNumeratorOrDenominator(t *testing.T) {
	for _, whitelist := range []string{"", "500-599", "400"} {
		t.Run("whitelist="+whitelist, func(t *testing.T) {
			db, user, token, _ := cancellationHostFixture(t, "responses")
			common.RetryTimes = 0
			ptr := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
			old := *ptr
			ptr.Enabled = true
			ptr.ErrorCodeWhitelist = whitelist
			t.Cleanup(func() { *ptr = old })
			release := make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			var mode atomic.Int64
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, "/v1/responses", r.URL.Path, "actual Chat policy must reach Responses upstream")
				switch mode.Load() {
				case 0:
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"resp_ok","model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`+"\n\n")
				case 1:
					w.WriteHeader(503)
					_, _ = io.WriteString(w, embeddedOverloadFixture)
				case 2:
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+cancelHostCreated+"\n\ndata: "+cancelHostText+"\n\n")
					w.(http.Flusher).Flush()
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			ch := cancellationHostChannel(t, db, server.URL)
			// Both baseline outcomes travel through the real host and settle/refund paths.
			_, _ = runCancellationHost(t, user, token, ch, "wallet_only", "responses", "")
			mode.Store(1)
			_, _ = runCancellationHost(t, user, token, ch, "wallet_only", "responses", "")
			waitCancellationHostWork(t)
			before, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
			require.NoError(t, err)
			require.Len(t, before.Models, 1)
			require.Equal(t, int64(2), before.Models[0].RequestCount)
			expectedRate := 50.0
			if whitelist == "400" {
				expectedRate = 100
			}
			assert.Equal(t, expectedRate, before.Models[0].SuccessRate)
			mode.Store(2)
			_, writer := runCancellationHost(t, user, token, ch, "wallet_only", "responses", "visible generated answer")
			assert.True(t, writer.cancelled.Load())
			waitCancellationHostWork(t)
			after, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
			require.NoError(t, err)
			require.Len(t, after.Models, 1)
			assert.Equal(t, before.Models[0].RequestCount, after.Models[0].RequestCount)
			assert.Equal(t, before.Models[0].SuccessRate, after.Models[0].SuccessRate)
			assert.Equal(t, int64(3), calls.Load())
			var audit model.Log
			require.NoError(t, db.Where("type = ? AND other LIKE ?", model.LogTypeError, "%downstream_cancelled%").First(&audit).Error)
			assert.Equal(t, "cancel-host-request", audit.RequestId)
			once.Do(func() { close(release) })
		})
	}
}

func TestRelayUpstreamFailureThenClientCancellationRetainsFailureAndRefund(t *testing.T) {
	for _, path := range []string{"responses", "chat", "openai"} {
		for _, preference := range []string{"wallet_only", "subscription_only"} {
			for _, fault := range []string{"fatal", "timeout"} {

				t.Run(path+"/"+preference+"/"+fault, func(t *testing.T) {
					baselineQuota := 0
					if path == "responses" {
						baselineQuota = nativeUpstreamFailureBillingBaseline(t, preference, fault)
					}
					db, user, token, sub := cancellationHostFixture(t, path)
					verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
					reserved := observeCancellationReservation(t, db, user, token, sub)
					common.RetryTimes = 0
					ptr := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
					old := *ptr
					ptr.Enabled = true
					ptr.ErrorCodeWhitelist = ""
					t.Cleanup(func() { *ptr = old })
					release := make(chan struct{})
					var once sync.Once
					t.Cleanup(func() { once.Do(func() { close(release) }) })
					var calls atomic.Int64
					upstreamClosed := make(chan struct{})
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						expectedPath := "/v1/responses"
						if path == "openai" {
							expectedPath = "/v1/chat/completions"
						}
						assert.Equal(t, expectedPath, r.URL.Path)
						w.Header().Set("Content-Type", "text/event-stream")
						if path == "openai" {
							_, _ = io.WriteString(w, `data: {"id":"chat_fault","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"visible generated answer"}}]}`+"\n\n")
						} else {
							_, _ = io.WriteString(w, "data: "+cancelHostCreated+"\n\ndata: "+cancelHostText+"\n\n")
						}
						w.(http.Flusher).Flush()
						if fault == "fatal" {
							if path == "openai" {
								_, _ = io.WriteString(w, "data: "+embeddedOverloadFixture+"\n\n")
							} else {
								_, _ = io.WriteString(w, `data: {"type":"response.failed","response":{"id":"resp_cancel","status":"failed","error":{"code":"server_is_overloaded","message":"actual upstream failure before client cancellation"}}}`+"\n\n")
							}
							w.(http.Flusher).Flush()
						}
						select {
						case <-release:
						case <-r.Context().Done():
							close(upstreamClosed)
						}
					}))
					t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
					ch := cancellationHostChannel(t, db, server.URL)
					needle := `"error"`
					if path == "responses" && fault == "fatal" {
						needle = "response.failed"
					}
					var writer *cancellationHostWriter
					if path == "responses" && fault == "timeout" {
						_, writer = runCancellationHostWithSignal(t, user, token, ch, preference, path, "", upstreamClosed)
					} else {
						_, writer = runCancellationHost(t, user, token, ch, preference, path, needle)
					}
					assert.True(t, writer.cancelled.Load(), "cancel occurs only after the real fatal/timeout response is delivered")
					assert.Equal(t, int64(1), calls.Load())
					assert.NotContains(t, writer.Body.String(), "[DONE]")
					assert.NotContains(t, writer.Body.String(), "response.completed")
					if path == "responses" {
						waitCancellationHostWork(t)
						reserved(preference)
						var logs []model.Log
						require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
						require.Len(t, logs, 1)
						assert.Equal(t, baselineQuota, logs[0].Quota)
						if fault == "timeout" {
							assert.Contains(t, logs[0].Other, `"end_reason":"timeout"`, "timeout must be known before the body-close-triggered client cancellation")
						}
						assert.NotContains(t, logs[0].Other, "downstream_cancelled")
						assertCancellationBalances(t, db, user, token, sub, preference, baselineQuota, "settled")
					} else {
						verifyRefund(preference)
					}
					var audits []model.Log
					require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&audits).Error)
					if path != "responses" {
						require.Len(t, audits, 1)
					}
					for _, audit := range audits {
						assert.NotContains(t, audit.Other, "downstream_cancelled")
					}
					metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
					require.NoError(t, err)
					require.Len(t, metrics.Models, 1)
					assert.Equal(t, int64(1), metrics.Models[0].RequestCount)
					assert.Zero(t, metrics.Models[0].SuccessRate)
					once.Do(func() { close(release) })
				})
			}
		}
	}
}

// Native Responses failed events have an existing observed-usage settlement
// contract. A real no-cancellation control protects that contract independently
// of the new cancellation classification; converted Chat failures still refund.
func nativeUpstreamFailureBillingBaseline(t *testing.T, preference, fault string) int {
	t.Helper()
	quota := 0
	t.Run("native_without_cancel_billing_control", func(t *testing.T) {
		db, user, token, sub := cancellationHostFixture(t, "responses")
		reserved := observeCancellationReservation(t, db, user, token, sub)
		release := make(chan struct{})
		var once sync.Once
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+cancelHostCreated+"\n\ndata: "+cancelHostText+"\n\n")
			if fault == "fatal" {
				_, _ = io.WriteString(w, `data: {"type":"response.failed","response":{"id":"resp_cancel","status":"failed","error":{"code":"server_is_overloaded","message":"actual upstream failure before client cancellation"}}}`+"\n\n")
			} else {
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}
		}))
		t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
		ch := cancellationHostChannel(t, db, server.URL)
		_, writer := runCancellationHost(t, user, token, ch, preference, "responses", "")
		assert.False(t, writer.cancelled.Load())
		waitCancellationHostWork(t)
		reserved(preference)
		var logs []model.Log
		require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
		require.Len(t, logs, 1)
		quota = logs[0].Quota
		assert.Positive(t, quota)
		if fault == "timeout" {
			assert.Contains(t, logs[0].Other, `"end_reason":"timeout"`)
		}
		assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
	})
	return quota
}

// This instrument preserves a real HTTP transport and response. By returning
// exactly one complete SSE frame per Read, the next Read proves that the prior
// frame has been accepted into the scanner's data queue, rather than merely
// arriving in bufio's network buffer.
type cancellationFrameBody struct {
	io.ReadCloser
	reader    *bufio.Reader
	pending   []byte
	frames    int
	accepted  chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
	once      sync.Once
}

func (b *cancellationFrameBody) Read(p []byte) (int, error) {
	if len(b.pending) == 0 {
		if b.frames == 2 {
			b.once.Do(func() { close(b.accepted) })
		}
		for {
			line, err := b.reader.ReadString('\n')
			b.pending = append(b.pending, []byte(line)...)
			if err != nil {
				if len(b.pending) == 0 {
					return 0, err
				}
				break
			}
			if line == "\n" || line == "\r\n" {
				break
			}
		}
		b.frames++
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

func (b *cancellationFrameBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return b.ReadCloser.Close()
}

type cancellationHostRoundTripper func(*http.Request) (*http.Response, error)

func (f cancellationHostRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRelayAcceptedOpenAIErrorCannotBeNeutralizedByEarlierDownstreamCancel(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
			accepted, release := make(chan struct{}), make(chan struct{})
			bodyClosed := make(chan struct{})
			var once sync.Once
			var calls atomic.Int64
			var gateFailed atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"id":"accepted_fault","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n\ndata: "+embeddedOverloadFixture+"\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			ch := cancellationHostChannel(t, db, server.URL)
			client := service.GetHttpClient()
			if client == nil {
				service.InitHttpClient()
				client = service.GetHttpClient()
			}
			old := client.Transport
			base := old
			if base == nil {
				base = http.DefaultTransport
			}
			client.Transport = cancellationHostRoundTripper(func(r *http.Request) (*http.Response, error) {
				resp, err := base.RoundTrip(r)
				if err == nil && strings.HasPrefix(r.URL.String(), server.URL) {
					resp.Body = &cancellationFrameBody{ReadCloser: resp.Body, reader: bufio.NewReader(resp.Body), accepted: accepted, closed: bodyClosed}
				}
				return resp, err
			})
			t.Cleanup(func() { client.Transport = old })
			_, writer := runCancellationHostWithInstrument(t, user, token, ch, preference, "openai", `"role":"assistant"`, nil, func(w *cancellationHostWriter) {
				w.beforeCancel = func() {
					select {
					case <-accepted:
						w.cancelled.Store(true)
						w.cancel()
						select {
						case <-bodyClosed:
						case <-time.After(3 * time.Second):
							gateFailed.Store(true)
						}
					case <-time.After(3 * time.Second):
						gateFailed.Store(true)
					}
				}
			})
			assert.False(t, gateFailed.Load(), "cancel only after upstream error is accepted into the real scanner queue")
			assert.True(t, writer.cancelled.Load())
			assert.Equal(t, int64(1), calls.Load())
			assert.NotContains(t, writer.Body.String(), "[DONE]")
			verifyRefund(preference)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Contains(t, logs[0].Other, `"status_code":503`)
			assert.NotContains(t, logs[0].Other, "downstream_cancelled")
			waitCancellationHostWork(t)
			metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
			require.NoError(t, err)
			require.Len(t, metrics.Models, 1)
			assert.Equal(t, int64(1), metrics.Models[0].RequestCount)
			assert.Zero(t, metrics.Models[0].SuccessRate)
			once.Do(func() { close(release) })
		})
	}
}

func TestRelayConvertedCompletedNonContextWriteFailureKeepsRefundPolicy(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, short := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/broken_pipe", true: "/short_write"}[short], func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "chat")
				verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
				release := make(chan struct{})
				var once sync.Once
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+cancelHostCreated+"\n\ndata: "+cancelHostText+"\n\n"+`data: {"type":"response.completed","response":{"id":"resp_cancel","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`+"\n\n")
					w.(http.Flusher).Flush()
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				ch := cancellationHostChannel(t, db, server.URL)
				_, writer := runCancellationHostWithInstrument(t, user, token, ch, preference, "chat", "", nil, func(w *cancellationHostWriter) { w.failNeedle = `"finish_reason":"stop"`; w.shortWrite = short })
				assert.True(t, writer.failed.Load())
				assert.False(t, writer.cancelled.Load(), "broken/short write is not client context cancellation")
				assert.Equal(t, int64(1), calls.Load())
				assert.NotContains(t, writer.Body.String(), "[DONE]")
				verifyRefund(preference)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&logs).Error)
				require.Len(t, logs, 1)
				assert.NotContains(t, logs[0].Other, "downstream_cancelled")
				once.Do(func() { close(release) })
			})
		}
	}
}

func TestRelayNativeCompletedImageItemCancellationSettlesOneExistingToolCharge(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "responses")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			prices := config.GlobalConfig.Get("tool_price_setting").(*operation_setting.ToolPriceSetting)
			oldPrices := prices.Prices
			operation_setting.LoadToolPricesFromJSONString(`{"image_generation":1}`)
			t.Cleanup(func() { prices.Prices = oldPrices; operation_setting.RebuildToolPriceIndex() })
			imageResult := strings.Repeat("aW1hZ2U=", 200)
			item := map[string]any{"type": "response.output_item.done", "output_index": 0, "item": map[string]any{"id": "img_cancel", "type": "image_generation_call", "status": "completed", "result": imageResult}}
			first, err := common.Marshal(item)
			require.NoError(t, err)
			item["fixture_repeat"] = true
			repeat, err := common.Marshal(item)
			require.NoError(t, err)
			release := make(chan struct{})
			var once sync.Once
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range []string{cancelHostCreated, string(first), string(repeat)} {
					_, _ = io.WriteString(w, "data: "+frame+"\n\n")
					w.(http.Flusher).Flush()
				}
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
			ch := cancellationHostChannel(t, db, server.URL)
			_, writer := runCancellationHost(t, user, token, ch, preference, "responses", `"fixture_repeat":true`)
			assert.True(t, writer.cancelled.Load())
			assert.Equal(t, int64(1), calls.Load())
			waitCancellationHostWork(t)
			reserved(preference)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Zero(t, logs[0].CompletionTokens, "base64 image output is never tokenized as text")
			var other map[string]any
			require.NoError(t, common.Unmarshal([]byte(logs[0].Other), &other))
			items, ok := other["tool_surcharges"].([]any)
			require.True(t, ok)
			require.Len(t, items, 1)
			tool := items[0].(map[string]any)
			assert.Equal(t, "image_generation", tool["name"])
			assert.Equal(t, float64(1), tool["count"])
			assert.Equal(t, float64(1), tool["price"])
			assert.Equal(t, 500+logs[0].PromptTokens, logs[0].Quota, "one configured existing image surcharge plus existing observed-input estimate")
			assert.NotContains(t, logs[0].Other, imageResult)
			assertCancellationBalances(t, db, user, token, sub, preference, logs[0].Quota, "settled")
			metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
			require.NoError(t, err)
			assert.Empty(t, metrics.Models, "partial image work is still a neutral cancellation for reliability")
			once.Do(func() { close(release) })
		})
	}
}

type cancellationOnCloseBody struct {
	io.ReadCloser
	cancel func()
	once   sync.Once
}

func (b *cancellationOnCloseBody) Close() error { b.once.Do(b.cancel); return b.ReadCloser.Close() }

// The scanner stops on an actual upstream DONE before cancellation occurs.
// Its cleanup closes the actual HTTP body and synchronously cancels downstream;
// a queued legacy callback (sr.Stop) or finalizer then returns a wrapped
// actual request-context error. No scanner cancellation fact was manually injected, so the controller
// must recognize the trusted cause even though the scanner's stop won first.
func TestRelayLegacyStreamStopBeforeWrappedRequestCancelIsNeutral(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			common.RetryTimes = 0
			verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"id":"legacy_stop","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			ch := cancellationHostChannel(t, db, server.URL)
			client := service.GetHttpClient()
			if client == nil {
				service.InitHttpClient()
				client = service.GetHttpClient()
			}
			old := client.Transport
			base := old
			if base == nil {
				base = http.DefaultTransport
			}
			var cancelClient func()
			client.Transport = cancellationHostRoundTripper(func(r *http.Request) (*http.Response, error) {
				resp, err := base.RoundTrip(r)
				if err == nil && strings.HasPrefix(r.URL.String(), server.URL) {
					resp.Body = &cancellationOnCloseBody{ReadCloser: resp.Body, cancel: cancelClient}
				}
				return resp, err
			})
			t.Cleanup(func() { client.Transport = old })
			_, writer := runCancellationHostWithInstrument(t, user, token, ch, preference, "openai", "", nil, func(w *cancellationHostWriter) { cancelClient = func() { w.cancelled.Store(true); w.cancel() } })
			assert.True(t, writer.cancelled.Load())
			assert.Equal(t, int64(1), calls.Load())
			assert.NotContains(t, writer.Body.String(), "[DONE]", "queued callback/finalizer observes actual cancellation and cannot append success DONE")
			verifyRefund(preference)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Contains(t, logs[0].Other, "downstream_cancelled")
			assert.Equal(t, "cancel-host-request", logs[0].RequestId)
			waitCancellationHostWork(t)
			metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
			require.NoError(t, err)
			assert.Empty(t, metrics.Models, "trusted wrapped request cancellation adds neither failure nor success sample")
		})
	}
}
