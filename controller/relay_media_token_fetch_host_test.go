package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestB17RelayKnownImageTokenEstimateFetchesOnlyWhenNeeded(t *testing.T) {
	for _, scenario := range []struct {
		name, model, preference          string
		media, nonstream, stream, denied bool
		estimate                         int
	}{
		{"qwen_wallet", "qwen-vl", "wallet_only", true, true, true, false, 528},
		{"qwen_subscription", "qwen-vl", "subscription_only", true, true, true, false, 528},
		{"openai_dimension_ssrf", "gpt-4o", "wallet_only", true, true, true, true, 0},
		{"openai_media_off", "gpt-4o", "wallet_only", false, true, true, false, 262},
		{"openai_nonstream_off", "gpt-4o", "wallet_only", true, false, false, false, 262},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			reserved := observeCancellationReservation(t, db, user, token, sub)
			var deniedQuotaUpdates atomic.Int64
			if scenario.denied {
				require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:b17_no_reservation", func(tx *gorm.DB) {
					if tx.Statement.Table == "users" || tx.Statement.Table == "tokens" || tx.Statement.Table == "user_subscriptions" {
						deniedQuotaUpdates.Add(1)
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:b17_no_reservation")) })
			}
			oldMedia, oldNonstream := constant.GetMediaToken, constant.GetMediaTokenNotStream
			constant.CountToken = true
			constant.GetMediaToken = scenario.media
			constant.GetMediaTokenNotStream = scenario.nonstream
			t.Cleanup(func() { constant.GetMediaToken = oldMedia; constant.GetMediaTokenNotStream = oldNonstream })
			oldCompletion := ratio_setting.CompletionRatio2JSONString()
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion)) })
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1,"qwen-vl":1}`))
			require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-4o":4,"qwen-vl":4}`))
			fetch := system_setting.GetFetchSetting()
			oldFetch := *fetch
			t.Cleanup(func() { *fetch = oldFetch })
			fetch.EnableSSRFProtection = true
			fetch.AllowPrivateIp = false
			fetch.DomainFilterMode = false
			fetch.IpFilterMode = false
			fetch.DomainList = nil
			fetch.IpList = nil
			fetch.AllowedPorts = []string{"1-65535"}
			fetch.ApplyIPFilterForDomain = true
			var mediaCalls, providerCalls atomic.Int64
			image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mediaCalls.Add(1); w.WriteHeader(500) }))
			t.Cleanup(image.Close)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				providerCalls.Add(1)
				input, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Contains(t, string(input), image.URL)
				if scenario.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"public\",\"model\":\""+scenario.model+"\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"public","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
				}
			}))
			t.Cleanup(provider.Close)
			ch := lifecycleHostChannel(t, db, provider.URL, constant.ChannelTypeOpenAI)
			raw := `{"model":"` + scenario.model + `","max_tokens":128,"stream":` + map[bool]string{true: "true", false: "false"}[scenario.stream] + `,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"` + image.URL + `/private-image.png","detail":"high"}}]}]}`
			t.Cleanup(func() { waitCancellationHostWork(t) })
			c, w := b16RunHost(t, user, token, ch, scenario.preference, "/v1/chat/completions", []byte(raw), func(c *gin.Context, _ *lifecycleHostWriter) {
				common.SetContextKey(c, constant.ContextKeyOriginalModel, scenario.model)
			})
			assert.Zero(t, mediaCalls.Load(), "no HTTP media request: fixed estimate skips loading, dimensions enforce SSRF before network")
			if scenario.denied {
				assert.Equal(t, 500, w.Code)
				assert.Zero(t, providerCalls.Load())
				assert.Contains(t, w.Body.String(), "count_token_failed")
				assert.Zero(t, deniedQuotaUpdates.Load(), "token estimate rejected before any SQL reservation mutation")
				var receiptCount, consumeCount int64
				require.NoError(t, db.Model(&model.SubscriptionPreConsumeRecord{}).Count(&receiptCount).Error)
				require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&consumeCount).Error)
				assert.Zero(t, receiptCount)
				assert.Zero(t, consumeCount)
				assertCancellationBalances(t, db, user, token, sub, scenario.preference, 0, "refunded")
			} else {
				require.Equal(t, 200, w.Code, w.Body.String())
				assert.Equal(t, int64(1), providerCalls.Load())
				assert.Contains(t, w.Body.String(), "answer")
				assert.Equal(t, scenario.estimate, common.GetContextKeyInt(c, constant.ContextKeyPromptTokens))
				assert.False(t, strings.Contains(w.Body.String(), image.URL), "provider output does not reflect request image URL")
				waitCancellationHostWork(t)
				reserved(scenario.preference)
				assertCancellationBalances(t, db, user, token, sub, scenario.preference, 18, "settled")
			}
		})
	}
}
