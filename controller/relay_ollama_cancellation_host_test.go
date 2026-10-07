package controller

import (
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
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ollamaHostTextFrame = `{"model":"private-ollama-alias","message":{"role":"assistant","content":"ollama visible generation"},"done":false}`
const ollamaHostDoneFrame = `{"model":"private-ollama-alias","message":{"role":"assistant","content":"last visible payload","thinking":"last reasoning payload","tool_calls":[{"function":{"name":"lookup","arguments":{"city":"Paris"}}}]},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2}`

func TestRelayOllamaCancellationClosesNDJSONWithoutWaitingForDone(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"text", "tool", "reasoning", "role"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				lifecycleHostAutoBanPolicy(t)
				release, closed := make(chan struct{}), make(chan struct{})
				var once sync.Once
				var calls atomic.Int64
				var guardExpired atomic.Bool
				frame, needle, observed := ollamaHostTextFrame, "ollama visible generation", "ollama visible generation"
				switch kind {
				case "tool":
					frame = `{"model":"private-ollama-alias","message":{"role":"assistant","tool_calls":[{"function":{"name":"lookup","arguments":{"city":"Paris"}}}]},"done":false}`
					needle = "lookup"
					observed = `lookup{"city":"Paris"}`
				case "reasoning":
					frame = `{"model":"private-ollama-alias","message":{"role":"assistant","thinking":"observed ollama reasoning"},"done":false}`
					needle = "observed ollama reasoning"
					observed = "observed ollama reasoning"
				case "role":
					frame = ""
					needle = `"role":"assistant"`
					observed = ""
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, "/api/chat", r.URL.Path)
					w.Header().Set("Content-Type", "application/x-ndjson")
					w.WriteHeader(200)
					if frame != "" {
						_, _ = io.WriteString(w, frame+"\n")
					}
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						close(closed)
					case <-release:
					}
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				ch.ModelMapping = common.GetPointer(`{"gpt-4o":"private-ollama-alias"}`)
				require.NoError(t, db.Save(&ch).Error)
				timer := time.AfterFunc(3*time.Second, func() { guardExpired.Store(true); once.Do(func() { close(release) }) })
				defer timer.Stop()
				_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) { w.cancelOnFlush = needle })
				assert.False(t, guardExpired.Load())
				assert.True(t, writer.cancelled.Load())
				assert.Equal(t, int64(1), calls.Load())
				assert.NotContains(t, writer.Body.String(), "[DONE]")
				assert.NotContains(t, writer.Body.String(), "private-ollama-alias")
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Error("actual Ollama HTTP request remained open after cancellation")
				}
				waitCancellationHostWork(t)
				reserved(preference)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				quota, status := 0, "refunded"
				if kind == "role" {
					for _, log := range logs {
						assert.Zero(t, log.Quota)
						assert.Zero(t, log.PromptTokens)
						assert.Zero(t, log.CompletionTokens)
					}
				} else {
					require.Len(t, logs, 1)
					expected := service.CountTextToken(observed, "private-ollama-alias")
					if kind == "tool" {
						expected += 7
					}
					assert.Equal(t, expected, logs[0].CompletionTokens, "existing observed-text formula plus existing per-function estimate")
					assert.Positive(t, logs[0].Quota)
					quota, status = logs[0].Quota, "settled"
				}
				assertCancellationBalances(t, db, user, token, sub, preference, quota, status)
				assertLifecycleNeutralAudit(t, db, ch)
				once.Do(func() { close(release) })
			})
		}
	}
}

func TestRelayOllamaDoneLastPayloadAndAuthoritativeZeroSettleOnce(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, zero := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/known", true: "/zero"}[zero], func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				frame := ollamaHostDoneFrame
				if zero {
					frame = strings.ReplaceAll(strings.ReplaceAll(frame, `"prompt_eval_count":10`, `"prompt_eval_count":0`), `"eval_count":2`, `"eval_count":0`)
				}
				release := make(chan struct{})
				var once sync.Once
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/x-ndjson")
					_, _ = io.WriteString(w, frame+"\n")
					w.(http.Flusher).Flush()
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				var deadlock atomic.Bool
				guard := time.AfterFunc(3*time.Second, func() { deadlock.Store(true); once.Do(func() { close(release) }) })
				defer guard.Stop()
				t.Cleanup(func() {
					assert.False(t, deadlock.Load(), "gated Ollama fixture must finish before its deadlock release guard")
				})

				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				ch.ModelMapping = common.GetPointer(`{"gpt-4o":"private-ollama-alias"}`)
				require.NoError(t, db.Save(&ch).Error)
				_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) { w.cancelOnFlush = `"finish_reason":"tool_calls"` })
				assert.True(t, writer.cancelled.Load())
				assert.Equal(t, int64(1), calls.Load())
				assert.Contains(t, writer.Body.String(), "last visible payload")
				assert.Contains(t, writer.Body.String(), "last reasoning payload")
				assert.Contains(t, writer.Body.String(), "lookup")
				assert.Contains(t, writer.Body.String(), "Paris")
				assert.NotContains(t, writer.Body.String(), "private-ollama-alias")
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
					assert.Equal(t, 18, logs[0].Quota)
					quota = 18
				}
				assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
				metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
				require.NoError(t, err)
				require.Len(t, metrics.Models, 1)
				assert.Equal(t, int64(1), metrics.Models[0].RequestCount)
				assert.Equal(t, 100.0, metrics.Models[0].SuccessRate)
				once.Do(func() { close(release) })
			})
		}
	}
}

// A real no-fault control protects Ollama's pre-existing usage,nil fee contract
// independently of its newly observed transport failure fact.
func ollamaHostFeeControl(t *testing.T, preference, frame string) int {
	t.Helper()
	quota := 0
	t.Run("fee_control", func(t *testing.T) {
		db, user, token, sub := cancellationHostFixture(t, "openai")
		reserved := observeCancellationReservation(t, db, user, token, sub)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, frame+"\n")
		}))
		t.Cleanup(server.Close)
		ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
		_, _ = runLifecycleHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`), nil)
		waitCancellationHostWork(t)
		reserved(preference)
		var logs []model.Log
		require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
		for _, log := range logs {
			quota += log.Quota
		}
		assertCancellationBalances(t, db, user, token, sub, preference, quota, "settled")
	})
	return quota
}

func TestRelayOllamaNonContextWriteFailureClosesConnectionAndKeepsEndpointFee(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, done := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/partial", true: "/done"}[done], func(t *testing.T) {
				frame, needle := ollamaHostTextFrame, "ollama visible generation"
				if done {
					frame, needle = ollamaHostDoneFrame, "last visible payload"
				}
				expectedQuota := ollamaHostFeeControl(t, preference, frame)
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				release, closed := make(chan struct{}), make(chan struct{})
				var once sync.Once
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
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
				var deadlock atomic.Bool
				guard := time.AfterFunc(3*time.Second, func() { deadlock.Store(true); once.Do(func() { close(release) }) })
				defer guard.Stop()
				t.Cleanup(func() {
					assert.False(t, deadlock.Load(), "gated Ollama fixture must finish before its deadlock release guard")
				})

				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOllama)
				_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/chat/completions", "application/json", []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"PRIVATE_B11_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) { w.failWriteNeedle = needle })
				assert.True(t, writer.failed.Load())
				assert.False(t, writer.cancelled.Load())
				assert.Equal(t, int64(1), calls.Load())
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Error("non-context write failure did not close actual Ollama upstream")
				}
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, expectedQuota, "settled")
				var audits []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&audits).Error)
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
