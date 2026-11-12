package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func imageOutboundContext(t *testing.T, raw []byte, channelType int, baseURL string, override map[string]any, passthrough bool) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	c, info := passthroughTestContext(t, raw, false, baseURL, override)
	c.Request.URL.Path = "/v1/images/generations"
	info.RequestURLPath = c.Request.URL.Path
	info.RelayMode = relayconstant.RelayModeImagesGenerations
	info.RelayFormat = types.RelayFormatOpenAIImage
	request := &dto.ImageRequest{}
	require.NoError(t, common.Unmarshal(raw, request))
	info.Request = request
	common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: passthrough})
	return c, info
}

func TestImageOutboundCountValidationAfterOverrides(t *testing.T) {
	previousGlobal, previousDisk := model_setting.GetGlobalSettings().PassThroughRequestEnabled, common.GetDiskCacheConfig()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = previousGlobal
		common.SetDiskCacheConfig(previousDisk)
	})
	service.InitHttpClient()
	for _, passthrough := range []bool{false, true} {
		for _, disk := range []bool{false, true} {
			for _, tc := range []struct {
				name  string
				value any
				valid bool
			}{
				{"upper bound", 128, true}, {"legacy zero", 0, true}, {"null default", nil, true},
				{"over bound", 129, false}, {"negative", -1, false}, {"fraction", 1.5, false},
				{"string", "3", false}, {"boolean", true, false}, {"wrapped negative", uint64(18446744073686646784), false},
			} {
				t.Run(fmt.Sprintf("passthrough_%t/disk_%t/%s", passthrough, disk, tc.name), func(t *testing.T) {
					common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 64, Path: t.TempDir()})
					var hits atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						hits.Add(1)
						w.WriteHeader(http.StatusTeapot)
						_, _ = io.WriteString(w, `{"error":{"message":"fixture stop","type":"invalid_request_error"}}`)
					}))
					defer server.Close()
					raw := []byte(`{"model":"client-model","prompt":"sunset","n":1}`)
					c, info := imageOutboundContext(t, raw, constant.ChannelTypeOpenAI, server.URL, map[string]any{"n": tc.value}, passthrough)
					apiErr := ImageHelper(c, info)
					require.NotNil(t, apiErr)
					if tc.valid {
						assert.Equal(t, http.StatusTeapot, apiErr.StatusCode, apiErr.Error())
						assert.EqualValues(t, 1, hits.Load())
					} else {
						assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode, apiErr.Error())
						assert.True(t, types.IsSkipRetryError(apiErr))
						assert.Zero(t, hits.Load(), "invalid final quantity must never reach the provider")
					}
					assert.EqualValues(t, 1, *info.Request.(*dto.ImageRequest).N)
				})
			}
		}
	}
}

func TestImageOutboundProviderNativeQuantityAndReplay(t *testing.T) {
	previousGlobal, previousDisk := model_setting.GetGlobalSettings().PassThroughRequestEnabled, common.GetDiskCacheConfig()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 64, Path: t.TempDir()})
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = previousGlobal
		common.SetDiskCacheConfig(previousDisk)
	})
	service.InitHttpClient()
	for _, tc := range []struct {
		name, extra, path string
		channelType       int
		count             uint
		passthrough       bool
		override          map[string]any
	}{
		{"ali extra", `,"parameters":{"n":3}`, "parameters.n", constant.ChannelTypeAli, 3, false, nil},
		{"ali override", "", "parameters.n", constant.ChannelTypeAli, 4, false, map[string]any{"parameters": map[string]any{"n": 4}}},
		{"replicate input", `,"input":{"num_outputs":3}`, "input.num_outputs", constant.ChannelTypeReplicate, 3, false, nil},
		{"replicate extra fields", `,"extra_fields":{"num_outputs":129}`, "input.num_outputs", constant.ChannelTypeReplicate, 129, false, nil},
		{"replicate override", "", "input.num_outputs", constant.ChannelTypeReplicate, 129, false, map[string]any{"operations": []any{map[string]any{"mode": "set", "path": "input.num_outputs", "value": 129}}}},
		{"ali passthrough", `,"parameters":{"n":129}`, "parameters.n", constant.ChannelTypeAli, 129, true, nil},
		{"disk unchanged", `,"private":{"large":9007199254740993,"number":1.2300}`, "n", constant.ChannelTypeOpenAI, 1, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusTeapot)
				_, _ = io.WriteString(w, `{"error":{"message":"fixture stop","type":"invalid_request_error"}}`)
			}))
			defer server.Close()
			raw := []byte(`{"model":"client-model","prompt":"sunset","n":1` + tc.extra + `}`)
			c, info := imageOutboundContext(t, raw, tc.channelType, server.URL, tc.override, tc.passthrough)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			require.True(t, storage.IsDisk())
			counted := &passthroughBytesCounter{BodyStorage: storage}
			c.Set(common.KeyBodyStorage, counted)
			apiErr := ImageHelper(c, info)
			require.NotNil(t, apiErr)
			if tc.count > dto.MaxImageN {
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode, apiErr.Error())
				assert.Empty(t, got)
			} else {
				assert.Equal(t, http.StatusTeapot, apiErr.StatusCode, apiErr.Error())
				assert.EqualValues(t, tc.count, gjson.GetBytes(got, tc.path).Uint())
			}
			if tc.passthrough && tc.override == nil {
				assert.Zero(t, counted.bytesCalls, "quantity validation must use a separate replay reader")
				if tc.count <= dto.MaxImageN {
					assert.Equal(t, raw, got)
				}
			}
			original, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, raw, original)
		})
	}
}

func TestImageOutboundRejectsAmbiguousQuantityKeys(t *testing.T) {
	service.InitHttpClient()
	for _, tc := range []struct {
		name, fields string
		channel      int
	}{
		{"root case", `,"n":129,"N":1`, constant.ChannelTypeOpenAI},
		{"root duplicate", `,"n":129,"n":1`, constant.ChannelTypeOpenAI},
		{"ali parent case", `,"parameters":{"n":129},"Parameters":{"n":1}`, constant.ChannelTypeAli},
		{"ali parent duplicate", `,"parameters":{"n":129},"parameters":{"n":1}`, constant.ChannelTypeAli},
		{"ali leaf case", `,"parameters":{"n":129,"N":1}`, constant.ChannelTypeAli},
		{"ali leaf duplicate", `,"parameters":{"n":129,"n":1}`, constant.ChannelTypeAli},
		{"replicate parent case", `,"input":{"num_outputs":129},"Input":{"num_outputs":1}`, constant.ChannelTypeReplicate},
		{"replicate leaf case", `,"input":{"num_outputs":129,"NUM_OUTPUTS":1}`, constant.ChannelTypeReplicate},
		{"replicate leaf duplicate", `,"input":{"num_outputs":129,"num_outputs":1}`, constant.ChannelTypeReplicate},
		{"gemini leaf case", `,"parameters":{"sampleCount":129,"SampleCount":1}`, constant.ChannelTypeGemini},
		{"siliconflow duplicate", `,"batch_size":129,"batch_size":1`, constant.ChannelTypeSiliconFlow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
			defer server.Close()
			raw := []byte(`{"model":"client-model","prompt":"sunset"` + tc.fields + `}`)
			c, info := imageOutboundContext(t, raw, tc.channel, server.URL, nil, true)
			apiErr := ImageHelper(c, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Zero(t, hits.Load())
		})
	}
}

func TestImageOutboundImagenRequiresExplicitCount(t *testing.T) {
	for _, apiType := range []int{constant.APITypeGemini, constant.APITypeVertexAi} {
		for _, body := range []string{`{}`, `{"parameters":null}`, `{"parameters":{}}`, `{"parameters":{"sampleCount":null}}`, `{"parameters":{"sampleCount":0}}`} {
			_, err := outboundImageCount(strings.NewReader(body), apiType)
			require.Error(t, err, "Imagen's server-side default must not be guessed as one")
		}
		count, err := outboundImageCount(strings.NewReader(`{"parameters":{"sampleCount":3}}`), apiType)
		require.NoError(t, err)
		assert.EqualValues(t, 3, count)
	}
}

func TestImageOutboundConvertedProviderCountsReserveBeforeSend(t *testing.T) {
	for _, tc := range []struct {
		name, extra, field string
		channel            int
	}{
		{"gemini", "", "parameters.sampleCount", constant.ChannelTypeGemini},
		{"vertex", "", "parameters.sampleCount", constant.ChannelTypeVertexAi},
		{"siliconflow", `,"batch_size":3`, "batch_size", constant.ChannelTypeSiliconFlow},
		{"ali", `,"parameters":{"n":3}`, "parameters.n", constant.ChannelTypeAli},
		{"replicate", `,"input":{"num_outputs":3}`, "input.num_outputs", constant.ChannelTypeReplicate},
	} {
		for _, override := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/override_%t", tc.name, override), func(t *testing.T) {
				raw := []byte(`{"model":"imagen-test","prompt":"sunset","n":3` + tc.extra + `}`)
				var overrides map[string]any
				if override {
					overrides = map[string]any{"operations": []any{map[string]any{"mode": "set", "path": tc.field, "value": 129}}}
				}
				c, info := imageOutboundContext(t, raw, tc.channel, "http://unused.invalid", overrides, false)
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "imagen-test")
				info.OriginModelName = "imagen-test"
				info.PriceData = hosttypes.PriceData{UsePrice: true, ModelPrice: 50 / common.QuotaPerUnit, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
				billing := &deniedTaskReservation{}
				info.Billing = billing
				apiErr := ImageHelper(c, info)
				require.NotNil(t, apiErr)
				if override {
					assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode, apiErr.Error())
					assert.Zero(t, billing.target)
				} else {
					assert.Equal(t, http.StatusForbidden, apiErr.StatusCode, apiErr.Error())
					assert.Equal(t, 150, billing.target, "the converted provider quantity remains three")
				}
			})
		}
	}
}

func TestImageOutboundMultipartRejectsAmbiguousOrInvalidCounts(t *testing.T) {
	service.InitHttpClient()
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			fields [][2]string
			valid  bool
		}{
			{"single", [][2]string{{"n", "3"}}, true},
			{"duplicate", [][2]string{{"n", "1"}, {"n", "129"}}, false},
			{"case alias", [][2]string{{"n", "1"}, {"N", "129"}}, false},
			{"too large", [][2]string{{"n", "129"}}, false},
		} {
			t.Run(fmt.Sprintf("%s/passthrough_%t", tc.name, passthrough), func(t *testing.T) {
				var payload bytes.Buffer
				writer := multipart.NewWriter(&payload)
				require.NoError(t, writer.WriteField("model", "client-model"))
				for _, field := range tc.fields {
					require.NoError(t, writer.WriteField(field[0], field[1]))
				}
				file, err := writer.CreateFormFile("image", "input.png")
				require.NoError(t, err)
				_, err = file.Write([]byte{0, 1, 2, 3})
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				var got []byte
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got, _ = io.ReadAll(r.Body)
					w.WriteHeader(http.StatusTeapot)
				}))
				defer server.Close()
				c, info := passthroughTestContext(t, payload.Bytes(), false, server.URL, nil)
				c.Request.Header.Set("Content-Type", writer.FormDataContentType())
				c.Request.URL.Path = "/v1/images/edits"
				info.RequestURLPath, info.RelayMode, info.RelayFormat = c.Request.URL.Path, relayconstant.RelayModeImagesEdits, types.RelayFormatOpenAIImage
				info.Request = &dto.ImageRequest{Model: "client-model", N: common.GetPointer(uint(1))}
				common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
				common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: passthrough})
				apiErr := ImageHelper(c, info)
				require.NotNil(t, apiErr)
				if tc.valid {
					assert.Equal(t, http.StatusTeapot, apiErr.StatusCode, apiErr.Error())
					assert.NotEmpty(t, got)
					if passthrough {
						assert.Equal(t, payload.Bytes(), got)
					}
				} else {
					assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode, apiErr.Error())
					assert.Empty(t, got)
				}
			})
		}
	}
}

func TestImageOutboundMultipartValidationCleansTemporaryUploads(t *testing.T) {
	previousLimit, previousDisk := constant.MaxFileDownloadMB, common.GetDiskCacheConfig()
	constant.MaxFileDownloadMB = 1
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: false})
	t.Cleanup(func() {
		constant.MaxFileDownloadMB = previousLimit
		common.SetDiskCacheConfig(previousDisk)
	})
	service.InitHttpClient()
	for _, count := range []string{"1", "129"} {
		t.Run(count, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			var payload bytes.Buffer
			writer := multipart.NewWriter(&payload)
			require.NoError(t, writer.WriteField("n", count))
			file, err := writer.CreateFormFile("image", "large.png")
			require.NoError(t, err)
			_, err = file.Write(bytes.Repeat([]byte("x"), (1<<20)+17))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
			defer server.Close()
			c, info := passthroughTestContext(t, payload.Bytes(), false, server.URL, nil)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			info.RelayMode, info.RelayFormat = relayconstant.RelayModeImagesEdits, types.RelayFormatOpenAIImage
			info.Request = &dto.ImageRequest{Model: "client-model"}
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
			apiErr := ImageHelper(c, info)
			require.NotNil(t, apiErr)
			if count == "1" {
				assert.Equal(t, http.StatusTeapot, apiErr.StatusCode)
			} else {
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			}
			entries, err := os.ReadDir(tmp)
			require.NoError(t, err)
			assert.Empty(t, entries, "both successful validation and early rejection must remove temporary uploads")
		})
	}
}

func TestImageOutboundFixedPriceRealReservationSettlementAndRefund(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousRedis, previousCache, previousBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	previousLogs, previousExport := common.LogConsumeEnabled, common.DataExportEnabled
	previousGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
	common.LogConsumeEnabled, common.DataExportEnabled = true, false
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = previousRedis, previousCache, previousBatch
		common.LogConsumeEnabled, common.DataExportEnabled = previousLogs, previousExport
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = previousGlobal
		constant.StreamingTimeout = previousTimeout
	})
	for _, tc := range []struct {
		name                                     string
		initialN, overrideN, responseN, balance  int
		status, wantStatus, wantCharge, wantHold int
		passthrough, deleteN                     bool
		retryN                                   int
		disconnect, cancelBefore                 bool
	}{
		{name: "converted raise", initialN: 1, overrideN: 3, responseN: 3, balance: 1000, status: 200, wantCharge: 150, wantHold: 150},
		{name: "passthrough raise", initialN: 1, overrideN: 3, responseN: 3, balance: 1000, status: 200, wantCharge: 150, wantHold: 150, passthrough: true},
		{name: "response fewer images", initialN: 1, overrideN: 3, responseN: 2, balance: 1000, status: 200, wantCharge: 100, wantHold: 150},
		{name: "removed count uses provider default", initialN: 3, responseN: 1, balance: 1000, status: 200, wantCharge: 50, wantHold: 150, passthrough: true, deleteN: true},
		{name: "upstream failure refund", initialN: 1, overrideN: 3, balance: 1000, status: 502, wantStatus: 502, wantHold: 150},
		{name: "insufficient before send", initialN: 1, overrideN: 3, balance: 100, status: 200, wantStatus: 403, passthrough: true},
		{name: "invalid override refund", initialN: 1, overrideN: 129, balance: 1000, status: 200, wantStatus: 400},
		{name: "retry resets quantity and refunds excess hold", initialN: 1, overrideN: 3, responseN: 2, balance: 1000, status: 200, wantCharge: 100, wantHold: 150, retryN: 2},
		{name: "disconnect retains final wire quantity", initialN: 1, overrideN: 3, responseN: 1, balance: 1000, status: 200, wantCharge: 150, wantHold: 150, passthrough: true, disconnect: true},
		{name: "cancelled request refunds all holds", initialN: 1, overrideN: 3, balance: 1000, status: 200, wantStatus: 500, cancelBefore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDeepSeekBillingDB(t)
			user := model.User{Username: "image-owner", AffCode: "image", Status: common.UserStatusEnabled, Quota: tc.balance, Group: "default", Setting: `{"billing_preference":"wallet_only","quota_warning_enabled":false}`}
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "image-token", Status: common.TokenStatusEnabled, RemainQuota: tc.balance, ExpiredTime: -1}
			require.NoError(t, db.Create(&token).Error)
			channel := model.Channel{Name: "image-channel", Type: constant.ChannelTypeOpenAI, Key: "fixture-key"}
			require.NoError(t, db.Create(&channel).Error)
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := hits.Add(1)
				var heldUser model.User
				assert.NoError(t, db.First(&heldUser, user.Id).Error)
				assert.Equal(t, tc.balance-tc.wantHold, heldUser.Quota, "reserve the final quantity before sending")
				wire, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				if tc.retryN != 0 {
					if attempt == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					assert.EqualValues(t, tc.retryN, gjson.GetBytes(wire, "n").Int())
				}
				if tc.disconnect {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"first-image\"}\n\n")
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status != 200 {
					_, _ = io.WriteString(w, `{"error":{"message":"fixture failure","type":"upstream_error"}}`)
					return
				}
				images := make([]string, tc.responseN)
				for i := range images {
					images[i] = `{"url":"https://example.test/image"}`
				}
				_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(images, ","))
			}))
			defer server.Close()
			raw := []byte(fmt.Sprintf(`{"model":"client-model","prompt":"sunset","n":%d}`, tc.initialN))
			override := map[string]any{"n": tc.overrideN}
			if tc.deleteN {
				override = map[string]any{"operations": []any{map[string]any{"mode": "delete", "path": "n"}}}
			}
			c, info := imageOutboundContext(t, raw, constant.ChannelTypeOpenAI, server.URL, override, tc.passthrough)
			common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
			info.UserId, info.TokenId, info.TokenKey = user.Id, token.Id, token.Key
			info.UsingGroup, info.UserGroup = "default", "default"
			info.StartTime, info.ForcePreConsume, info.UserSetting = time.Now(), true, user.GetSetting()
			info.PriceData = hosttypes.PriceData{UsePrice: true, ModelPrice: 50 / common.QuotaPerUnit, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
			info.PriceData.AddOtherRatio("n", float64(tc.initialN))
			require.Nil(t, service.PreConsumeBilling(c, 50*tc.initialN, info))
			if tc.disconnect || tc.cancelBefore {
				ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				if tc.cancelBefore {
					cancel()
				} else {
					c.Writer = &imageCancelAfterWrite{ResponseWriter: c.Writer, cancel: cancel}
				}
			}
			apiErr := ImageHelper(c, info)
			if tc.retryN != 0 {
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
				common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"n": tc.retryN})
				apiErr = ImageHelper(c, info)
			}
			if tc.wantStatus != 0 {
				require.NotNil(t, apiErr)
				assert.Equal(t, tc.wantStatus, apiErr.StatusCode, apiErr.Error())
				// The controller owns this cleanup after a failed helper attempt.
				info.Billing.Refund(c)
			} else {
				require.Nil(t, apiErr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, service.WaitBillingRefunds(ctx))
			var gotUser model.User
			var gotToken model.Token
			require.NoError(t, db.First(&gotUser, user.Id).Error)
			require.NoError(t, db.First(&gotToken, token.Id).Error)
			assert.Equal(t, tc.balance-tc.wantCharge, gotUser.Quota)
			assert.Equal(t, tc.balance-tc.wantCharge, gotToken.RemainQuota)
			assert.Equal(t, tc.wantCharge, gotUser.UsedQuota)
			assert.Equal(t, tc.wantCharge, gotToken.UsedQuota)
			if tc.wantHold == 0 {
				assert.Zero(t, hits.Load())
			} else if tc.retryN != 0 {
				assert.EqualValues(t, 2, hits.Load())
			} else {
				assert.EqualValues(t, 1, hits.Load())
			}
			assert.EqualValues(t, tc.initialN, *info.Request.(*dto.ImageRequest).N)
		})
	}
}

type imageCancelAfterWrite struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

func (w *imageCancelAfterWrite) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if bytes.Contains(data, []byte("first-image")) {
		w.cancel()
	}
	return n, err
}

func (w *imageCancelAfterWrite) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func TestImageOutboundTokenAndExpressionPricingStayFrozen(t *testing.T) {
	service.InitHttpClient()
	for _, expression := range []bool{false, true} {
		t.Run(fmt.Sprintf("expression_%t", expression), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
			defer server.Close()
			raw := []byte(`{"model":"client-model","prompt":"sunset","n":1}`)
			c, info := imageOutboundContext(t, raw, constant.ChannelTypeOpenAI, server.URL, map[string]any{"n": 3}, false)
			input := &billingexpr.RequestInput{Body: bytes.Clone(raw)}
			info.BillingRequestInput = input
			info.PriceData.QuotaToPreConsume = 77
			billing := &deniedTaskReservation{}
			info.Billing = billing
			if expression {
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{ExprString: `param("n") * 100`, BillingMode: "tiered_expr"}
			}
			apiErr := ImageHelper(c, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusTeapot, apiErr.StatusCode)
			assert.Zero(t, billing.target, "quantity must not multiply token/expression pricing")
			assert.Equal(t, 77, info.PriceData.QuotaToPreConsume)
			assert.Same(t, input, info.BillingRequestInput)
			assert.Equal(t, raw, input.Body)
		})
	}
}
