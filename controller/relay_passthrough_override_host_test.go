package controller

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const b16PassOverrides = `{"operations":[{"path":"temperature","mode":"set","value":0.125,"conditions":[{"path":"model","mode":"prefix","value":"gpt"}]},{"path":"remove_me","mode":"delete"},{"path":"custom.new_flag","mode":"set","value":false},{"path":"wrong_case","mode":"set","value":true,"conditions":[{"path":"model","mode":"prefix","value":"does-not-match"}]},{"mode":"set_header","path":"X-B16-Operation","value":"applied"}]}`

func TestB16RelayPassthroughOverridesPreserveRawProviderFields(t *testing.T) {
	for _, claude := range []bool{false, true} {
		for _, global := range []bool{false, true} {
			for _, override := range []bool{false, true} {
				t.Run(map[bool]string{false: "chat", true: "claude"}[claude]+map[bool]string{false: "/channel", true: "/global"}[global]+map[bool]string{false: "/exact", true: "/override"}[override], func(t *testing.T) {
					db, user, token, sub := cancellationHostFixture(t, "openai")
					reserved := observeCancellationReservation(t, db, user, token, sub)
					model_setting.GetGlobalSettings().PassThroughRequestEnabled = global
					path, kind := "/v1/chat/completions", constant.ChannelTypeOpenAI
					response := `{"id":"public-response","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`
					if claude {
						path, kind = "/v1/messages", constant.ChannelTypeAnthropic
						response = `{"id":"public-response","type":"message","role":"assistant","model":"gpt-4o","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":2}}`
					}
					body := []byte(" { \"model\" : \"gpt-4o\", \"max_tokens\":128, \"stream\":false, \"messages\":[{\"role\":\"user\",\"content\":\"PRIVATE_B16_PROMPT\"}], \"temperature\":0.8, \"remove_me\":\"gone\", \"custom\":{\"big\":5000000000000000001,\"decimal\":1.234567890123456789,\"literal\":\"line\\r\\n中文😀\",\"nested\":[null,false,{\"unknown\":\"exact\"}]} }\n")
					var mu sync.Mutex
					var received []byte
					var headers http.Header
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						input, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						assert.Equal(t, path, r.URL.Path)
						mu.Lock()
						received, headers = input, r.Header.Clone()
						mu.Unlock()
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, response)
					}))
					t.Cleanup(server.Close)
					ch := lifecycleHostChannel(t, db, server.URL, kind)
					settings, err := common.Marshal(dto.ChannelSettings{PassThroughBodyEnabled: !global})
					require.NoError(t, err)
					ch.Setting = common.GetPointer(string(settings))
					if override {
						ch.ParamOverride = common.GetPointer(b16PassOverrides)
					}
					require.NoError(t, db.Save(&ch).Error)
					t.Cleanup(func() { waitCancellationHostWork(t) })
					_, writer := b16RunHost(t, user, token, ch, "wallet_only", path, body, nil)
					assert.Equal(t, 200, writer.Code)
					mu.Lock()
					actual, actualHeaders := append([]byte(nil), received...), headers.Clone()
					mu.Unlock()
					require.NotEmpty(t, actual)
					if !override {
						assert.Equal(t, body, actual, "no-override must preserve exact spacing/order/unknown number bytes")
					} else {
						assert.Equal(t, 0.125, gjson.GetBytes(actual, "temperature").Float())
						assert.False(t, gjson.GetBytes(actual, "remove_me").Exists())
						assert.True(t, gjson.GetBytes(actual, "custom.new_flag").Exists())
						assert.False(t, gjson.GetBytes(actual, "custom.new_flag").Bool())
						assert.False(t, gjson.GetBytes(actual, "wrong_case").Exists())
						assert.Equal(t, "applied", actualHeaders.Get("X-B16-Operation"))
					}
					assert.Equal(t, "5000000000000000001", gjson.GetBytes(actual, "custom.big").Raw)
					assert.Equal(t, "1.234567890123456789", gjson.GetBytes(actual, "custom.decimal").Raw)
					assert.Equal(t, "line\r\n中文😀", gjson.GetBytes(actual, "custom.literal").String())
					assert.Equal(t, gjson.GetBytes(body, "custom.nested").Raw, gjson.GetBytes(actual, "custom.nested").Raw)
					assert.NotContains(t, writer.Body.String(), "PRIVATE_B16_PROMPT")
					waitCancellationHostWork(t)
					reserved("wallet_only")
					assertCancellationBalances(t, db, user, token, sub, "wallet_only", 18, "settled")
				})
			}
		}
	}
}

func TestB16RelayPassthroughKeepsExplicitStreamAndAdministratorHeaderRules(t *testing.T) {
	for _, claude := range []bool{false, true} {
		t.Run(map[bool]string{false: "chat", true: "claude"}[claude], func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			path, kind := "/v1/chat/completions", constant.ChannelTypeOpenAI
			response := `{"id":"public","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`
			if claude {
				path, kind = "/v1/messages", constant.ChannelTypeAnthropic
				response = `{"id":"public","type":"message","role":"assistant","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":2}}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				input, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.True(t, gjson.GetBytes(input, "stream").Bool(), "existing explicit body override is allowed")
				assert.Equal(t, "Bearer ADMIN_B16", r.Header.Get("Authorization"), "administrator override follows existing priority")
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(server.Close)
			ch := lifecycleHostChannel(t, db, server.URL, kind)
			ch.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
			ch.ParamOverride = common.GetPointer(`{"operations":[{"path":"stream","mode":"set","value":true}]}`)
			ch.HeaderOverride = common.GetPointer(`{"Authorization":"Bearer ADMIN_B16"}`)
			require.NoError(t, db.Save(&ch).Error)
			t.Cleanup(func() { waitCancellationHostWork(t) })
			_, writer := b16RunHost(t, user, token, ch, "wallet_only", path, []byte(`{"model":"gpt-4o","max_tokens":128,"stream":false,"messages":[{"role":"user","content":"prompt"}]}`), nil)
			assert.Equal(t, 200, writer.Code)
			assertCancellationBalances(t, db, user, token, sub, "wallet_only", 18, "settled")
		})
	}
}

// Keep the large-body control on the real handler: an existing disk-backed
// replay source must remain reusable, and the override output owns its own file.
func TestB16RelayPassthroughLargeDiskBodyPreservesUnknownFields(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "override"}[override], func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			old := common.GetDiskCacheConfig()
			diskPath := t.TempDir()
			common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: true, ThresholdMB: 1, MaxSizeMB: 16, Path: diskPath})
			t.Cleanup(func() { common.SetDiskCacheConfig(old) })
			pad := strings.Repeat("x", 2*1024*1024)
			body := []byte(`{"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"prompt"}],"unknown_padding":"` + pad + `","remove_me":"gone"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				input, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				if !override {
					assert.Equal(t, body, input)
				} else {
					assert.False(t, gjson.GetBytes(input, "remove_me").Exists())
				}
				assert.Equal(t, pad, gjson.GetBytes(input, "unknown_padding").String())
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"public","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			}))
			t.Cleanup(server.Close)
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
			ch.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
			if override {
				ch.ParamOverride = common.GetPointer(`{"operations":[{"path":"remove_me","mode":"delete"}]}`)
			}
			require.NoError(t, db.Save(&ch).Error)
			t.Cleanup(func() { waitCancellationHostWork(t) })
			c, writer := b16RunHost(t, user, token, ch, "wallet_only", "/v1/chat/completions", body, nil)
			assert.Equal(t, 200, writer.Code)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			assert.True(t, storage.IsDisk())
			original, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, body, original)
			common.CleanupBodyStorage(c)
			var remaining []string
			require.NoError(t, filepath.WalkDir(diskPath, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					remaining = append(remaining, path)
				}
				return nil
			}))
			assert.Empty(t, remaining, "both outbound override spool and original storage cleaned")
			assertCancellationBalances(t, db, user, token, sub, "wallet_only", 18, "settled")
		})
	}
}

func TestB16RelayPassthroughRetryUsesFreshBodyAndChannelHeaders(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			original := []byte(` {"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"prompt"}],"custom":{"big":5000000000000000001}} `)
			var recordsMu sync.Mutex
			var firstBody, secondBody []byte
			var firstHeaders, secondHeaders http.Header
			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordsMu.Lock()
				defer recordsMu.Unlock()
				firstBody, _ = io.ReadAll(r.Body)
				firstHeaders = r.Header.Clone()
				w.WriteHeader(503)
				_, _ = io.WriteString(w, `{"error":{"message":"fixture unavailable"}}`)
			}))
			t.Cleanup(first.Close)
			second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordsMu.Lock()
				defer recordsMu.Unlock()
				secondBody, _ = io.ReadAll(r.Body)
				secondHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"public","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			}))
			t.Cleanup(second.Close)
			ch := lifecycleHostChannel(t, db, first.URL, constant.ChannelTypeOpenAI)
			ch.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
			ch.ParamOverride = common.GetPointer(`{"operations":[{"mode":"set","path":"custom.first_only","value":true},{"mode":"set_header","path":"X-B16-Private","value":"FIRST_PRIVATE"},{"mode":"set_header","path":"Authorization","value":"Bearer FIRST_CREDENTIAL"}]}`)
			require.NoError(t, db.Save(&ch).Error)
			require.NoError(t, db.Where("channel_id = ?", ch.Id).Delete(&model.Ability{}).Error)
			backup := model.Channel{Id: ch.Id + 1, Type: constant.ChannelTypeOpenAI, Key: "SECOND_CREDENTIAL", Name: "backup", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(second.URL), Setting: common.GetPointer(`{"pass_through_body_enabled":true}`)}
			require.NoError(t, db.Create(&backup).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: backup.Id, Enabled: true}).Error)
			t.Cleanup(func() { waitCancellationHostWork(t) })
			c, w := b16RunHost(t, user, token, ch, preference, "/v1/chat/completions", original, nil)
			require.Equal(t, 200, w.Code, w.Body.String())
			recordsMu.Lock()
			defer recordsMu.Unlock()
			require.True(t, gjson.GetBytes(firstBody, "custom.first_only").Bool())
			assert.Equal(t, "FIRST_PRIVATE", firstHeaders.Get("X-B16-Private"))
			assert.Equal(t, "Bearer FIRST_CREDENTIAL", firstHeaders.Get("Authorization"))
			assert.Equal(t, original, secondBody, "retry must start from immutable original storage")
			assert.Empty(t, secondHeaders.Get("X-B16-Private"), "failed channel runtime header must not cross provider boundary")
			assert.Equal(t, "Bearer SECOND_CREDENTIAL", secondHeaders.Get("Authorization"))
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			saved, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, original, saved)
			waitCancellationHostWork(t)
			reserved(preference)
			assertCancellationBalances(t, db, user, token, sub, preference, 18, "settled")
		})
	}
}

func TestB16RelayPassthroughInvalidOverrideRetainsChannelFallbackPolicy(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"public","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			}))
			t.Cleanup(server.Close)
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
			ch.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
			ch.ParamOverride = common.GetPointer(`{"operations":[{"mode":"regex_replace","path":"model","value":{"pattern":"[PRIVATE_INVALID_CONFIG","replacement":"x"}}]}`)
			require.NoError(t, db.Save(&ch).Error)
			require.NoError(t, db.Where("channel_id = ?", ch.Id).Delete(&model.Ability{}).Error)
			backup := model.Channel{Id: ch.Id + 1, Type: constant.ChannelTypeOpenAI, Key: "backup", Name: "configuration-fallback", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(server.URL), Setting: common.GetPointer(`{"pass_through_body_enabled":true}`)}
			require.NoError(t, db.Create(&backup).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: backup.Id, Enabled: true}).Error)
			t.Cleanup(func() { waitCancellationHostWork(t) })
			_, w := b16RunHost(t, user, token, ch, preference, "/v1/chat/completions", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_PROMPT"}],"stream":false}`), nil)
			assert.Equal(t, 200, w.Code)
			assert.Equal(t, int64(1), calls.Load())
			var errorLogs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&errorLogs).Error)
			require.Len(t, errorLogs, 1)
			assert.Contains(t, errorLogs[0].Content, "invalid passthrough parameter override")
			assert.NotContains(t, errorLogs[0].Content, "PRIVATE_INVALID_CONFIG")
			assert.NotContains(t, w.Body.String(), "PRIVATE_INVALID_CONFIG")
			assert.NotContains(t, w.Body.String(), "PRIVATE_PROMPT")
			waitCancellationHostWork(t)
			reserved(preference)
			assertCancellationBalances(t, db, user, token, sub, preference, 18, "settled")
		})
	}
}
func TestB16RelayPassthroughRequestCapRejectsBeforeDispatch(t *testing.T) {
	db, user, token, sub := cancellationHostFixture(t, "openai")
	oldMax := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = oldMax })
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	t.Cleanup(server.Close)
	ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
	ch.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
	require.NoError(t, db.Save(&ch).Error)
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"prompt"}],"unknown":"` + strings.Repeat("x", 2<<20) + `"}`)
	t.Cleanup(func() { waitCancellationHostWork(t) })
	_, w := b16RunHost(t, user, token, ch, "wallet_only", "/v1/chat/completions", body, nil)
	assert.Equal(t, 413, w.Code)
	assert.Zero(t, calls.Load())
	assertCancellationBalances(t, db, user, token, sub, "wallet_only", 0, "refunded")
}

func TestB16RelayPassthroughStaticErrorsPreserveRetryAndRefundContracts(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, adminReturn := range []bool{false, true} {
			t.Run(preference+map[bool]string{false: "/all_invalid", true: "/admin_skip"}[adminReturn], func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
				t.Cleanup(server.Close)
				invalid := `{"operations":[{"mode":"regex_replace","path":"model","value":{"pattern":"[PRIVATE_INVALID_CONFIG","replacement":"x"}}]}`
				rules := invalid
				if adminReturn {
					rules = `{"operations":[{"mode":"return_error","value":{"message":"Configured static diagnostic","status_code":400,"code":"operator_rule","skip_retry":true}}]}`
				}
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeOpenAI)
				ch.Setting = common.GetPointer(`{"pass_through_body_enabled":true}`)
				ch.ParamOverride = common.GetPointer(rules)
				require.NoError(t, db.Save(&ch).Error)
				require.NoError(t, db.Where("channel_id = ?", ch.Id).Delete(&model.Ability{}).Error)
				backup := model.Channel{Id: ch.Id + 1, Type: constant.ChannelTypeOpenAI, Key: "backup", Name: "available-backup", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(server.URL), Setting: common.GetPointer(`{"pass_through_body_enabled":true}`)}
				if !adminReturn {
					backup.ParamOverride = common.GetPointer(invalid)
				}
				require.NoError(t, db.Create(&backup).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: backup.Id, Enabled: true}).Error)
				t.Cleanup(func() { waitCancellationHostWork(t) })
				_, w := b16RunHost(t, user, token, ch, preference, "/v1/chat/completions", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_PROMPT"}],"stream":false}`), nil)
				assert.Zero(t, calls.Load())
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeError).Find(&logs).Error)
				if adminReturn {
					assert.Equal(t, 400, w.Code)
					require.Len(t, logs, 1)
					assert.Contains(t, w.Body.String(), "operator_rule")
				} else {
					assert.Equal(t, 500, w.Code)
					assert.GreaterOrEqual(t, len(logs), 2, "existing channel-error fallback remains higher priority than SkipRetry")
					assert.Contains(t, w.Body.String(), "channel:param_override_invalid")
				}
				assert.NotContains(t, w.Body.String(), "PRIVATE_INVALID_CONFIG")
				assert.NotContains(t, w.Body.String(), "PRIVATE_PROMPT")
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, 0, "refunded")
			})
		}
	}
}
