package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestB17RelayAliSyncChoiceDeliversEveryImageAndBillsProviderCount(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			oldPrice := ratio_setting.ModelPrice2JSONString()
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice)) })
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o":0.001}`))
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, "/api/v1/services/aigc/multimodal-generation/generation", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				var request struct {
					Model      string `json:"model"`
					Parameters struct {
						N int `json:"n"`
					} `json:"parameters"`
				}
				assert.NoError(t, common.Unmarshal(body, &request))
				assert.Equal(t, "qwen-image", request.Model)
				assert.Equal(t, 2, request.Parameters.N)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"output":{"choices":[{"message":{"role":"assistant","content":[{"text":"revised prompt"},{"image":"https://images.invalid/first.png"},{"image":"https://images.invalid/second.png"}]}}]},"usage":{"image_count":2}}`)
			}))
			t.Cleanup(server.Close)
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeAli)
			ch.ModelMapping = common.GetPointer(`{"gpt-4o":"qwen-image"}`)
			require.NoError(t, db.Save(&ch).Error)
			_, writer := b17RunImageHost(t, user, token, ch, preference, "/v1/images/generations", []byte(`{"model":"gpt-4o","prompt":"PRIVATE_B17_PROMPT","n":2,"response_format":"url"}`), func(c *gin.Context, w *lifecycleHostWriter) {
				c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
			})
			require.Equal(t, 200, writer.Code, writer.Body.String())
			assert.Equal(t, int32(1), calls.Load())
			var response dto.ImageResponse
			require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &response))
			assert.Len(t, response.Data, 2)
			if len(response.Data) == 2 {
				assert.Equal(t, "https://images.invalid/first.png", response.Data[0].Url)
				assert.Equal(t, "https://images.invalid/second.png", response.Data[1].Url)
				assert.Equal(t, "revised prompt", response.Data[0].RevisedPrompt)
				assert.Equal(t, "revised prompt", response.Data[1].RevisedPrompt)
			}
			waitCancellationHostWork(t)
			reserved(preference)
			assertCancellationBalances(t, db, user, token, sub, preference, 1000, "settled")
		})
	}
}

func b17RunImageHost(t *testing.T, user model.User, token model.Token, ch model.Channel, preference, path string, body []byte, configure func(*gin.Context, *lifecycleHostWriter)) (*gin.Context, *lifecycleHostWriter) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &lifecycleHostWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "b17-image-host-request")
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
	format := types.RelayFormat(types.RelayFormatOpenAIImage)
	Relay(c, format)
	return c, w
}

func TestB17RelayAliImageLegacyAndAsyncControls(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, tc := range []struct {
			name, output  string
			async         bool
			n             int
			urls, prompts []string
			quota         int
			failed        bool
		}{
			{name: "n1", output: `{"choices":[{"message":{"content":[{"image":"https://images.invalid/only.png"},{"text":"last shared text"}]}}]}`, n: 1, urls: []string{"https://images.invalid/only.png"}, prompts: []string{"last shared text"}, quota: 500},
			{name: "legacy_results", output: `{"results":[{"url":"https://images.invalid/first.png"},{"url":"https://images.invalid/second.png"}]}`, n: 2, urls: []string{"https://images.invalid/first.png", "https://images.invalid/second.png"}, prompts: []string{"", ""}, quota: 1000},
			{name: "only_text", output: `{"choices":[{"message":{"content":[{"text":"provider text only"}]}}]}`, n: 2, urls: []string{}, prompts: []string{}, quota: 1000},
			{name: "multiple_choices", output: `{"choices":[{"message":{"content":[{"image":"https://images.invalid/first.png"},{"text":"choice one"},{"image":"https://images.invalid/second.png"}]}},{"message":{"content":[{"text":"choice two"},{"image":"https://images.invalid/third.png"}]}}]}`, n: 3, urls: []string{"https://images.invalid/first.png", "https://images.invalid/second.png", "https://images.invalid/third.png"}, prompts: []string{"choice one", "choice one", "choice two"}, quota: 1500},
			{name: "async_success", async: true, output: `{"task_status":"SUCCEEDED","choices":[{"message":{"content":[{"text":"async shared"},{"image":"https://images.invalid/first.png"},{"image":"https://images.invalid/second.png"}]}}]}`, n: 2, urls: []string{"https://images.invalid/first.png", "https://images.invalid/second.png"}, prompts: []string{"async shared", "async shared"}, quota: 1000},
			{name: "async_failed", async: true, output: `{"task_status":"FAILED","message":"fixture task failed","code":"fixture_failed"}`, n: 2, quota: 0, failed: true},
		} {
			t.Run(preference+"/"+tc.name, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				common.RetryTimes = 0
				reserved := observeCancellationReservation(t, db, user, token, sub)
				oldPrice := ratio_setting.ModelPrice2JSONString()
				t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice)) })
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o":0.001}`))
				var submits, polls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						polls.Add(1)
						assert.Equal(t, "/api/v1/tasks/fixture-task", r.URL.Path)
					} else {
						submits.Add(1)
						if tc.async {
							_, _ = io.WriteString(w, `{"output":{"task_id":"fixture-task","task_status":"PENDING"}}`)
							return
						}
					}
					output := `{"output":` + tc.output + `,"usage":{"image_count":` + strconv.Itoa(tc.n) + `}}`
					_, _ = io.WriteString(w, output)
				}))
				t.Cleanup(server.Close)
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeAli)
				target := "qwen-image"
				if tc.async {
					target = "wanx2.1-t2i-turbo"
				}
				mapping, err := common.Marshal(map[string]string{"gpt-4o": target})
				require.NoError(t, err)
				ch.ModelMapping = common.GetPointer(string(mapping))
				require.NoError(t, db.Save(&ch).Error)
				body, err := common.Marshal(map[string]interface{}{"model": "gpt-4o", "prompt": "PRIVATE_B17_PROMPT", "n": tc.n, "response_format": "url"})
				require.NoError(t, err)
				_, writer := b17RunImageHost(t, user, token, ch, preference, "/v1/images/generations", body, func(c *gin.Context, w *lifecycleHostWriter) {
					c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
				})
				assert.Equal(t, int32(1), submits.Load())
				if tc.async {
					assert.Equal(t, int32(1), polls.Load())
				} else {
					assert.Zero(t, polls.Load())
				}
				status := "settled"
				if tc.failed {
					status = "refunded"
					assert.NotContains(t, writer.Body.String(), `"data":[`)
					assert.Contains(t, writer.Body.String(), "error")
				} else {
					require.Equal(t, 200, writer.Code, writer.Body.String())
					var response dto.ImageResponse
					require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &response))
					assert.Len(t, response.Data, len(tc.urls))
					for i, data := range response.Data {
						if i >= len(tc.urls) {
							break
						}
						assert.Equal(t, tc.urls[i], data.Url)
						assert.Equal(t, tc.prompts[i], data.RevisedPrompt)
					}
				}
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, tc.quota, status)
			})
		}
	}
}

func TestB17RelayAliBase64DownloadsEveryImageInProviderOrder(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		t.Run(preference, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			oldPrice := ratio_setting.ModelPrice2JSONString()
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice)) })
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o":0.001}`))
			oldMaxDownload := constant.MaxFileDownloadMB
			constant.MaxFileDownloadMB = 1
			t.Cleanup(func() { constant.MaxFileDownloadMB = oldMaxDownload })
			fetch := system_setting.GetFetchSetting()
			oldFetch := *fetch
			*fetch = system_setting.FetchSetting{EnableSSRFProtection: true, AllowPrivateIp: true}
			t.Cleanup(func() { *fetch = oldFetch })
			service.InitHttpClient()
			t.Cleanup(service.GetHttpClient().CloseIdleConnections)
			t.Cleanup(service.GetSSRFProtectedHTTPClient().CloseIdleConnections)
			var downloads atomic.Int32
			files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads.Add(1)
				w.Header().Set("Content-Type", "image/png")
				_, _ = io.WriteString(w, map[string]string{"/first": "FIRST_IMAGE_BYTES", "/second": "SECOND_IMAGE_BYTES"}[r.URL.Path])
			}))
			t.Cleanup(files.Close)
			providerBody, err := common.Marshal(map[string]interface{}{"output": map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"content": []interface{}{map[string]string{"image": files.URL + "/first"}, map[string]string{"text": "download shared"}, map[string]string{"image": files.URL + "/second"}}}}}}, "usage": map[string]int{"image_count": 2}})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(providerBody)
			}))
			t.Cleanup(server.Close)
			ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeAli)
			ch.ModelMapping = common.GetPointer(`{"gpt-4o":"qwen-image"}`)
			require.NoError(t, db.Save(&ch).Error)
			_, writer := b17RunImageHost(t, user, token, ch, preference, "/v1/images/generations", []byte(`{"model":"gpt-4o","prompt":"PRIVATE_B17_PROMPT","n":2,"response_format":"b64_json"}`), func(c *gin.Context, w *lifecycleHostWriter) {
				c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
			})
			require.Equal(t, 200, writer.Code, writer.Body.String())
			var response dto.ImageResponse
			require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &response))
			assert.Len(t, response.Data, 2)
			assert.Equal(t, int32(2), downloads.Load())
			if len(response.Data) == 2 {
				assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("FIRST_IMAGE_BYTES")), response.Data[0].B64Json)
				assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("SECOND_IMAGE_BYTES")), response.Data[1].B64Json)
				for _, data := range response.Data {
					assert.Equal(t, "download shared", data.RevisedPrompt)
				}
			}
			waitCancellationHostWork(t)
			reserved(preference)
			assertCancellationBalances(t, db, user, token, sub, preference, 1000, "settled")
		})
	}
}

func TestB17RelayAliChoicesDownloadFailureRefundsWithoutRetryAndKeepsLegacyResults(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, kind := range []string{"choices_http_failure", "choices_private_blocked", "async_choices_http_failure", "legacy_results_partial"} {
			t.Run(preference+"/"+kind, func(t *testing.T) {
				db, user, token, sub := cancellationHostFixture(t, "openai")
				reserved := observeCancellationReservation(t, db, user, token, sub)
				oldPrice := ratio_setting.ModelPrice2JSONString()
				t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice)) })
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o":0.001}`))
				// Retry is enabled: the new static error must prevent duplicate generation.
				oldMaxDownload := constant.MaxFileDownloadMB
				constant.MaxFileDownloadMB = 1
				t.Cleanup(func() { constant.MaxFileDownloadMB = oldMaxDownload })
				fetch := system_setting.GetFetchSetting()
				oldFetch := *fetch
				*fetch = system_setting.FetchSetting{EnableSSRFProtection: true, AllowPrivateIp: kind != "choices_private_blocked"}
				t.Cleanup(func() { *fetch = oldFetch })
				service.InitHttpClient()
				t.Cleanup(service.GetHttpClient().CloseIdleConnections)
				t.Cleanup(service.GetSSRFProtectedHTTPClient().CloseIdleConnections)
				var downloads, submits, polls atomic.Int32
				files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					downloads.Add(1)
					if r.URL.Path == "/PRIVATE_B17_FAILED_IMAGE" {
						w.WriteHeader(404)
						return
					}
					w.Header().Set("Content-Type", "image/png")
					_, _ = io.WriteString(w, "FIRST_IMAGE_BYTES")
				}))
				t.Cleanup(files.Close)
				output := map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"content": []interface{}{map[string]string{"image": files.URL + "/first"}, map[string]string{"image": files.URL + "/PRIVATE_B17_FAILED_IMAGE"}}}}}}
				if kind == "legacy_results_partial" {
					output = map[string]interface{}{"results": []interface{}{map[string]string{"url": files.URL + "/first"}, map[string]string{"url": files.URL + "/PRIVATE_B17_FAILED_IMAGE"}}}
				}
				if kind == "async_choices_http_failure" {
					output["task_status"] = "SUCCEEDED"
				}
				providerBody, err := common.Marshal(map[string]interface{}{"output": output, "usage": map[string]int{"image_count": 2}})
				require.NoError(t, err)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						polls.Add(1)
						assert.Equal(t, "/api/v1/tasks/fixture-task", r.URL.Path)
					} else {
						submits.Add(1)
						if kind == "async_choices_http_failure" {
							_, _ = io.WriteString(w, `{"output":{"task_id":"fixture-task","task_status":"PENDING"}}`)
							return
						}
					}
					_, _ = w.Write(providerBody)
				}))
				t.Cleanup(server.Close)
				ch := lifecycleHostChannel(t, db, server.URL, constant.ChannelTypeAli)
				ch.ModelMapping = common.GetPointer(`{"gpt-4o":"qwen-image"}`)
				if kind == "async_choices_http_failure" {
					ch.ModelMapping = common.GetPointer(`{"gpt-4o":"wanx2.1-t2i-turbo"}`)
				}
				require.NoError(t, db.Save(&ch).Error)
				_, writer := b17RunImageHost(t, user, token, ch, preference, "/v1/images/generations", []byte(`{"model":"gpt-4o","prompt":"PRIVATE_B17_PROMPT","n":2,"response_format":"b64_json"}`), func(c *gin.Context, w *lifecycleHostWriter) {
					c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
				})
				assert.Equal(t, int32(1), submits.Load())
				if kind == "async_choices_http_failure" {
					assert.Equal(t, int32(1), polls.Load())
				} else {
					assert.Zero(t, polls.Load())
				}
				quota, status := 0, "refunded"
				if kind == "legacy_results_partial" {
					require.Equal(t, 200, writer.Code)
					var response dto.ImageResponse
					require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &response))
					assert.Len(t, response.Data, 1)
					quota, status = 1000, "settled"
				} else {
					assert.Equal(t, 502, writer.Code)
					assert.NotContains(t, writer.Body.String(), `"data":[`)
					assert.NotContains(t, writer.Body.String(), "PRIVATE_B17_FAILED_IMAGE")
					assert.NotContains(t, writer.Body.String(), files.URL)
				}
				if kind == "choices_private_blocked" {
					assert.Zero(t, downloads.Load())
				} else {
					assert.Equal(t, int32(2), downloads.Load())
				}
				waitCancellationHostWork(t)
				reserved(preference)
				assertCancellationBalances(t, db, user, token, sub, preference, quota, status)
			})
		}
	}
}
