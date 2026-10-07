package controller

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Shared B11 host instruments change only actual downstream writer behavior.
// They do not set handler outcomes, usage, billing receipts, or protocol facts.
type lifecycleHostWriter struct {
	*httptest.ResponseRecorder
	cancel                       context.CancelFunc
	cancelOnWrite, cancelOnFlush string
	onFlush                      func([]byte)
	failWrite                    bool
	failWriteNeedle              string
	cancelled                    atomic.Bool
	failed                       atomic.Bool
	once                         sync.Once
}

func (w *lifecycleHostWriter) cancelRequest() {
	w.once.Do(func() { w.cancelled.Store(true); w.cancel() })
}
func (w *lifecycleHostWriter) Write(p []byte) (int, error) {
	if w.failWrite || (w.failWriteNeedle != "" && strings.Contains(string(p), w.failWriteNeedle)) {
		w.failed.Store(true)
		return 0, io.ErrClosedPipe
	}
	n, err := w.ResponseRecorder.Write(p)
	if w.cancelOnWrite != "" && strings.Contains(string(p), w.cancelOnWrite) {
		w.cancelRequest()
	}
	return n, err
}
func (w *lifecycleHostWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func (w *lifecycleHostWriter) Flush() {
	w.ResponseRecorder.Flush()
	if w.onFlush != nil {
		w.onFlush(w.Body.Bytes())
	}
	if w.cancelOnFlush != "" && strings.Contains(w.Body.String(), w.cancelOnFlush) {
		w.cancelRequest()
	}
}

func runLifecycleHost(t *testing.T, user model.User, token model.Token, ch model.Channel, preference, path, contentType string, body []byte, configure func(*gin.Context, *lifecycleHostWriter)) (*gin.Context, *lifecycleHostWriter) {
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
	if strings.HasPrefix(path, "/v1/audio/") {
		format = types.RelayFormatOpenAIAudio
	}
	Relay(c, format)
	return c, w
}

func lifecycleHostChannel(t *testing.T, db *gorm.DB, url string, kind int) model.Channel {
	t.Helper()
	ch := model.Channel{Id: 723100 + int(cancellationHostChannelID.Add(1)), Type: kind, Name: "lifecycle-host", Key: "PRIVATE_B11_KEY", BaseURL: common.GetPointer(url), Status: common.ChannelStatusEnabled, AutoBan: common.GetPointer(1)}
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: ch.Id, Enabled: true}).Error)
	return ch
}
func assertLifecycleNeutralAudit(t *testing.T, db *gorm.DB, ch model.Channel) {
	t.Helper()
	waitCancellationHostWork(t)
	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Other, "downstream_cancelled")
	assert.Equal(t, "b11-lifecycle-request", logs[0].RequestId)
	assert.NotContains(t, logs[0].Other, "PRIVATE_B11_KEY")
	assert.NotContains(t, logs[0].Other, "PRIVATE_B11_PROMPT")
	assert.NotContains(t, logs[0].Content, "PRIVATE_MULTIPART_SENTINEL")
	metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
	require.NoError(t, err)
	assert.Empty(t, metrics.Models, "cancel adds neither performance numerator nor denominator")
	var after model.Channel
	require.NoError(t, db.First(&after, ch.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, after.Status)
}
func lifecycleHostAutoBanPolicy(t *testing.T) {
	t.Helper()
	oldEnabled, oldRanges := common.AutomaticDisableChannelEnabled, operation_setting.AutomaticDisableStatusCodeRanges
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 499, End: 599}}
	t.Cleanup(func() {
		waitCancellationHostWork(t)
		common.AutomaticDisableChannelEnabled = oldEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldRanges
	})
}

func TestRelayHTTPHeaderWaitCancellationClosesUpstreamAndRefundsReservations(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"JSON", "mid-body JSON", "multipart"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				verifyRefund := observeOpenAIErrorReservation(t, db, user, token, sub)
				lifecycleHostAutoBanPolicy(t)
				entered, upstreamClosed, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				var calls atomic.Int64
				var guardExpired atomic.Bool
				path, contentType, body := "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`)
				if kind == "multipart" {
					path = "/v1/audio/transcriptions"
					var buf bytes.Buffer
					form := multipart.NewWriter(&buf)
					require.NoError(t, form.WriteField("model", "gpt-4o"))
					require.NoError(t, form.WriteField("language", "en"))
					part, err := form.CreateFormFile("file", "fixture.wav")
					require.NoError(t, err)
					_, err = io.WriteString(part, "PRIVATE_MULTIPART_SENTINEL")
					require.NoError(t, err)
					require.NoError(t, form.Close())
					contentType, body = form.FormDataContentType(), buf.Bytes()
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, path, r.URL.Path)
					assert.Equal(t, "Bearer PRIVATE_B11_KEY", r.Header.Get("Authorization"))
					if kind == "multipart" {
						assert.NoError(t, r.ParseMultipartForm(1<<20))
						assert.Equal(t, "gpt-4o", r.FormValue("model"))
						assert.Equal(t, "en", r.FormValue("language"))
						f, _, err := r.FormFile("file")
						if assert.NoError(t, err) {
							data, e := io.ReadAll(f)
							assert.NoError(t, e)
							assert.Equal(t, "PRIVATE_MULTIPART_SENTINEL", string(data))
							_ = f.Close()
						}
					} else {
						_, _ = io.Copy(io.Discard, r.Body)
					}
					if kind == "mid-body JSON" {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"partial_json","choices":[`)
						w.(http.Flusher).Flush()
					}
					close(entered)
					select {
					case <-r.Context().Done():
						close(upstreamClosed)
					case <-release:
						w.WriteHeader(503)
					}
				}))
				t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); server.Close() })
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
				// This is a deadlock failure guard, not the implementation's global timeout.
				timer := time.AfterFunc(3*time.Second, func() { guardExpired.Store(true); releaseOnce.Do(func() { close(release) }) })
				defer timer.Stop()
				watcherDone, watcherStop := make(chan struct{}), make(chan struct{})
				_, writer := runLifecycleHost(t, user, token, ch, preference, path, contentType, body, func(c *gin.Context, w *lifecycleHostWriter) {
					go func() {
						defer close(watcherDone)
						select {
						case <-entered:
							w.cancelRequest()
						case <-watcherStop:
						}
					}()
				})
				close(watcherStop)
				<-watcherDone
				assert.False(t, guardExpired.Load(), "client context must release the real header wait without a global HTTP timeout")
				assert.True(t, writer.cancelled.Load())
				assert.Equal(t, int64(1), calls.Load())
				select {
				case <-upstreamClosed:
				case <-time.After(time.Second):
					t.Error("bound cancellation did not close actual upstream request")
				}
				verifyRefund(preference)
				assertLifecycleNeutralAudit(t, db, ch)
				releaseOnce.Do(func() { close(release) })
			})
		}
	}
}

func TestRelayKnownNonstreamUsageSurvivesWriteTimeCancellation(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"positive", "explicit_zero", "explicit_zero_per_call", "input_only_per_call", "absent", "null", "empty"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				expectedQuota, prompt, completion := 18, 10, 2
				usage := `,"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`
				switch kind {
				case "explicit_zero", "explicit_zero_per_call":
					usage = `,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`
					expectedQuota, prompt, completion = 0, 0, 0
				case "input_only_per_call":
					usage = `,"usage":{"prompt_tokens":10,"completion_tokens":0,"total_tokens":10}`
					expectedQuota, prompt, completion = 500, 10, 0
				case "absent", "null", "empty":
					usage = ""
					if kind == "null" {
						usage = `,"usage":null`
					}
					if kind == "empty" {
						usage = `,"usage":{}`
					}
					prompt = 0
					completion = service.CountTextToken("known nonstream answer", "gpt-4o")
					expectedQuota = completion * 4
				}
				if kind == "explicit_zero_per_call" || kind == "input_only_per_call" {
					oldPrice := ratio_setting.ModelPrice2JSONString()
					require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o":0.001}`))
					t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice)) })
					expectedQuota = nonstreamPerCallFeeControl(t, preference, prompt, completion)
					if kind == "input_only_per_call" {
						assert.Equal(t, 500, expectedQuota, "existing nonzero-input per-call fee stays charged")
					}
				}
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, "/v1/chat/completions", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"known_usage","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"known nonstream answer"},"finish_reason":"stop"}]`+usage+"}")
				}))
				t.Cleanup(server.Close)
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
				_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) { w.cancelOnWrite = "known nonstream answer" })
				assert.True(t, writer.cancelled.Load())
				assert.Equal(t, int64(1), calls.Load())
				assert.Equal(t, 200, writer.Code)
				waitCancellationHostWork(t)
				reserved(preference)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				if expectedQuota > 0 {
					require.Len(t, logs, 1)
				}
				for _, log := range logs {
					assert.Equal(t, prompt, log.PromptTokens)
					assert.Equal(t, completion, log.CompletionTokens)
					assert.Equal(t, expectedQuota, log.Quota)
				}
				assertCancellationBalances(t, db, user, token, sub, preference, expectedQuota, "settled")
			})
		}
	}
}

// Preserve the existing per-call pricing guard through a real uncancelled
// control. A context cancellation must not introduce a new fee exemption.
func nonstreamPerCallFeeControl(t *testing.T, preference string, prompt, completion int) int {
	t.Helper()
	quota := 0
	t.Run("original_pricing_without_cancel", func(t *testing.T) {
		db, user, token, sub := cancellationHostFixture(t, "openai")
		reserved := observeCancellationReservation(t, db, user, token, sub)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"known_usage","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"known nonstream answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":`+strconv.Itoa(prompt)+`,"completion_tokens":`+strconv.Itoa(completion)+`,"total_tokens":`+strconv.Itoa(prompt+completion)+`}}`)
		}))
		t.Cleanup(server.Close)
		ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
		_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`), nil)
		assert.False(t, writer.cancelled.Load())
		waitCancellationHostWork(t)
		reserved(preference)
		var logs []model.Log
		require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
		for _, log := range logs {
			quota += log.Quota
			assert.Equal(t, prompt, log.PromptTokens)
			assert.Equal(t, completion, log.CompletionTokens)
		}
		assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
	})
	return quota
}
