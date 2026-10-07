package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func speechHostRatios(t *testing.T) {
	oldInput, oldOutput := ratio_setting.AudioRatio2JSONString(), ratio_setting.AudioCompletionRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(`{"gpt-4o":1}`))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(`{"gpt-4o":1}`))
	t.Cleanup(func() {
		waitCancellationHostWork(t)
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldInput))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(oldOutput))
	})
}

func TestRelayBinarySpeechIncrementalBytesAndCancellationKeepActualFees(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"complete", "partial_cancel", "write_fault"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				speechHostRatios(t)
				reserved := observeCancellationReservation(t, db, user, token, sub)
				first := bytes.Repeat([]byte{0, 255, 'd', 'a', 't', 'a', ':'}, 3428)
				first = append(first, 0, 255, 0, 255)
				require.Len(t, first, 24000)
				second := bytes.Repeat([]byte{255, 0}, 12000)
				release, closed := make(chan struct{}), make(chan struct{})
				var once sync.Once
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, "/v1/audio/speech", r.URL.Path)
					w.Header().Set("Content-Type", "audio/pcm")
					_, _ = w.Write(first)
					w.(http.Flusher).Flush()
					select {
					case <-release:
						if kind == "complete" {
							_, _ = w.Write(second)
						}
					case <-r.Context().Done():
						close(closed)
					}
				}))
				t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
				var deadlock atomic.Bool
				guard := time.AfterFunc(3*time.Second, func() { deadlock.Store(true); once.Do(func() { close(release) }) })
				defer guard.Stop()
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
				_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/audio/speech", "application/json", []byte(`{"model":"gpt-4o","input":"PRIVATE_B11_PROMPT","voice":"alloy","response_format":"pcm","stream_format":"audio"}`), func(c *gin.Context, w *lifecycleHostWriter) {
					if kind == "write_fault" {
						w.failWrite = true
					}
					w.onFlush = func(body []byte) {
						if len(body) > 0 {
							if kind == "complete" {
								once.Do(func() { close(release) })
							} else {
								w.cancelRequest()
							}
						}
					}
				})
				assert.False(t, deadlock.Load(), "first binary chunk must flush before EOF")
				assert.Equal(t, int64(1), calls.Load())
				assert.Contains(t, writer.Header().Get("Content-Type"), "audio/pcm")
				assert.NotContains(t, writer.Header().Get("Content-Type"), "event-stream")
				if kind == "complete" {
					assert.Equal(t, append(append([]byte{}, first...), second...), writer.Body.Bytes())
				} else if kind == "partial_cancel" {
					assert.Equal(t, first, writer.Body.Bytes())
					assert.True(t, writer.cancelled.Load())
				} else {
					assert.Empty(t, writer.Body.Bytes())
					assert.True(t, writer.failed.Load())
					assert.False(t, writer.cancelled.Load())
				}
				if kind != "complete" {
					select {
					case <-closed:
					case <-time.After(time.Second):
						t.Error("binary upstream body did not close")
					}
				}
				waitCancellationHostWork(t)
				reserved(preference)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				assert.Equal(t, 17, logs[0].CompletionTokens)
				assert.Equal(t, 17, logs[0].Quota)
				assertCancellationBalances(t, db, user, token, sub, preference, 17, "settled")
				if kind == "partial_cancel" {
					assertLifecycleNeutralAudit(t, db, ch)
				}
				if kind == "write_fault" {
					var other map[string]interface{}
					require.NoError(t, json.Unmarshal([]byte(logs[0].Other), &other))
					stream, ok := other["stream_status"].(map[string]interface{})
					require.True(t, ok, "actual binary consume audit retains stream status")
					assert.Equal(t, "error", stream["status"])
					assert.Equal(t, "handler_stop", stream["end_reason"])
					assert.NotContains(t, logs[0].Other, "PRIVATE_B11_PROMPT")
				}
				once.Do(func() { close(release) })
			})
		}
	}
}

func TestRelaySpeechSSEKnownAudioUsageSettlesActualLedger(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			speechHostRatios(t)
			reserved := observeCancellationReservation(t, db, user, token, sub)
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"speech.audio.delta\",\"audio\":\"AP8=\"}\n\ndata: {\"type\":\"speech.audio.done\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2,\"total_tokens\":12,\"input_tokens_details\":{\"text_tokens\":10},\"output_tokens_details\":{\"audio_tokens\":2}}}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
			_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/audio/speech", "application/json", []byte(`{"model":"gpt-4o","input":"PRIVATE_B11_PROMPT","voice":"alloy","stream_format":"sse"}`), nil)
			assert.Equal(t, int64(1), calls.Load())
			assert.Contains(t, writer.Body.String(), "AP8=")
			assert.Contains(t, writer.Body.String(), "speech.audio.done")
			waitCancellationHostWork(t)
			reserved(preference)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 10, logs[0].PromptTokens)
			assert.Equal(t, 2, logs[0].CompletionTokens)
			assert.Equal(t, 12, logs[0].Quota)
			assertCancellationBalances(t, db, user, token, sub, preference, 12, "settled")
		})
	}
}

func TestRelayBinarySpeechNoBytesCancellationRefundsRealReservation(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			speechHostRatios(t)
			reserved := observeCancellationReservation(t, db, user, token, sub)
			entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "audio/pcm")
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				close(entered)
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
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
			_, writer := runLifecycleHost(t, user, token, ch, preference, "/v1/audio/speech", "application/json", []byte(`{"model":"gpt-4o","input":"PRIVATE_B11_PROMPT","voice":"alloy","response_format":"pcm","stream_format":"audio"}`), func(c *gin.Context, w *lifecycleHostWriter) {
				go func() {
					select {
					case <-entered:
						w.cancelRequest()
					case <-release:
					}
				}()
			})
			assert.False(t, deadlock.Load())
			assert.Equal(t, int64(1), calls.Load())
			assert.Empty(t, writer.Body.Bytes())
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Error("zero-byte binary body not closed")
			}
			waitCancellationHostWork(t)
			reserved(preference)
			assertCancellationBalances(t, db, user, token, sub, preference, 0, "refunded")
			assertLifecycleNeutralAudit(t, db, ch)
			once.Do(func() { close(release) })
		})
	}
}
