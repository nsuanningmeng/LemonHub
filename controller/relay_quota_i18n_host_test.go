package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
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

func TestB19RelayQuotaErrorsLocalizeWithoutChangingFundingOrLegacyMatching(t *testing.T) {
	for _, tc := range []struct{ name, kind, userLang, storedLang, contextLang, headerLang, wantLang string }{
		{name: "wallet_default_english", kind: "wallet_zero", wantLang: "en"},
		{name: "wallet_user_english_wins", kind: "wallet_zero", userLang: "en", contextLang: "zh-CN", headerLang: "zh-TW", wantLang: "en"},
		{name: "wallet_user_simplified_wins", kind: "wallet_zero", userLang: "zh-CN", contextLang: "en", headerLang: "zh-TW", wantLang: "zh-CN"},
		{name: "wallet_preconsume_context_traditional_wins", kind: "wallet_low", contextLang: "zh-TW", headerLang: "en", wantLang: "zh-TW"},
		{name: "subscription_no_active_header_english", kind: "subscription_no_active", headerLang: "en", wantLang: "en"},
		{name: "subscription_insufficient_header_traditional", kind: "subscription_low", headerLang: "zh-TW", wantLang: "zh-TW"},
		{name: "wallet_header_simplified", kind: "wallet_zero", headerLang: "zh-CN", wantLang: "zh-CN"},
		{name: "wallet_unsupported_languages_default_english", kind: "wallet_zero", userLang: "xx", contextLang: "xx", headerLang: "xx", wantLang: "en"},
		{name: "wallet_persisted_user_traditional_wins", kind: "wallet_zero", storedLang: "zh-TW", contextLang: "en", headerLang: "zh-CN", wantLang: "zh-TW"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, sub := cancellationHostFixture(t, "openai")
			require.NoError(t, i18n.Init())
			i18n.SetUserLangLoader(model.GetUserLanguage)
			t.Cleanup(func() { i18n.SetUserLangLoader(nil) })
			preference := "wallet_only"
			switch tc.kind {
			case "wallet_zero":
				user.Quota = 0
				require.NoError(t, db.Model(&user).Update("quota", 0).Error)
			case "wallet_low":
				user.Quota = 1
				require.NoError(t, db.Model(&user).Update("quota", 1).Error)
			case "subscription_no_active":
				preference = "subscription_only"
				sub.Status = "cancelled"
				require.NoError(t, db.Model(&sub).Update("status", "cancelled").Error)
			case "subscription_low":
				preference = "subscription_only"
				sub.AmountUsed = sub.AmountTotal - 1
				require.NoError(t, db.Model(&sub).Update("amount_used", sub.AmountUsed).Error)
			}
			if tc.storedLang != "" {
				user.SetSetting(dto.UserSetting{Language: tc.storedLang})
				require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
			}
			var minToken atomic.Int64
			minToken.Store(int64(token.RemainQuota))
			require.NoError(t, db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("test:b19_token_preconsume", func(tx *gorm.DB) {
				if tx.Error != nil || tx.Statement.Table != "tokens" {
					return
				}
				var v int64
				if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT remain_quota FROM tokens WHERE id = ?", token.Id).Scan(&v).Error; err != nil {
					tx.AddError(err)
					return
				}
				for {
					old := minToken.Load()
					if v >= old || minToken.CompareAndSwap(old, v) {
						break
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:b19_token_preconsume")) })
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"must not reach provider"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`))
			}))
			t.Cleanup(upstream.Close)
			ch := lifecycleHostChannel(t, db, upstream.URL, constant.ChannelTypeOpenAI)
			c, w := b16RunHost(t, user, token, ch, preference, "/v1/chat/completions", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_B19_PROMPT"}]}`), func(c *gin.Context, w *lifecycleHostWriter) {
				c.Set(common.RequestIdKey, "b19-quota-host")
				c.Set("id", user.Id)
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: preference, Language: tc.userLang})
				if tc.contextLang != "" {
					c.Set(string(constant.ContextKeyLanguage), tc.contextLang)
				}
				if tc.headerLang != "" {
					c.Request.Header.Set("Accept-Language", tc.headerLang)
				}
			})
			require.Equal(t, 403, w.Code, w.Body.String())
			var payload struct {
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &payload))
			assert.Equal(t, "insufficient_user_quota", payload.Error.Code)
			assert.Equal(t, int32(0), calls.Load())
			assert.NotContains(t, w.Body.String(), "PRIVATE_B19_PROMPT")
			assert.Equal(t, tc.wantLang, i18n.GetLangFromContext(c))
			legacy := "用户额度不足, 剩余额度: " + logger.FormatQuota(user.Quota)
			if tc.kind == "wallet_low" {
				legacy = "预扣费额度失败, 用户剩余额度: " + logger.FormatQuota(user.Quota) + ", 需要预扣费额度: " + logger.FormatQuota(common.PreConsumedQuota)
			}
			if strings.HasPrefix(tc.kind, "subscription_") {
				legacy = "订阅额度不足或未配置订阅: "
				if tc.kind == "subscription_no_active" {
					legacy += "no active subscription"
				} else {
					legacy += "subscription quota insufficient, need=500"
				}
			}
			assert.Contains(t, payload.Error.Message, legacy, "old downstream diagnostic must remain an exact substring")
			wantMessage := legacy
			if tc.wantLang != "zh-CN" {
				remaining := logger.FormatQuota(user.Quota)
				translated := "Insufficient user quota, remaining quota: " + remaining
				if tc.wantLang == "zh-TW" {
					translated = "使用者額度不足, 剩餘額度: " + remaining
				}
				if tc.kind == "wallet_low" {
					translated = "Quota pre-consumption failed, remaining user quota: " + remaining + ", required quota: " + logger.FormatQuota(common.PreConsumedQuota)
					if tc.wantLang == "zh-TW" {
						translated = "預扣費額度失敗, 使用者剩餘額度: " + remaining + ", 需要預扣費額度: " + logger.FormatQuota(common.PreConsumedQuota)
					}
				}
				if strings.HasPrefix(tc.kind, "subscription_") {
					reason := strings.TrimPrefix(legacy, "订阅额度不足或未配置订阅: ")
					translated = "Insufficient subscription quota or no active subscription: " + reason
					if tc.wantLang == "zh-TW" {
						translated = "訂閱額度不足或未設定訂閱: " + reason
					}
				}
				wantMessage = translated + " (" + legacy + ")"
			}
			assert.Equal(t, wantMessage+" (request id: b19-quota-host)", payload.Error.Message)

			waitCancellationHostWork(t)
			var finalUser model.User
			var finalToken model.Token
			var finalSub model.UserSubscription
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			require.NoError(t, db.First(&finalToken, token.Id).Error)
			require.NoError(t, db.First(&finalSub, sub.Id).Error)
			assert.Equal(t, user.Quota, finalUser.Quota)
			assert.Zero(t, finalUser.UsedQuota)
			assert.Equal(t, token.RemainQuota, finalToken.RemainQuota)
			assert.Zero(t, finalToken.UsedQuota)
			assert.Equal(t, sub.AmountUsed, finalSub.AmountUsed)
			var receipts int64
			require.NoError(t, db.Model(&model.SubscriptionPreConsumeRecord{}).Count(&receipts).Error)
			assert.Zero(t, receipts)
			var logs int64
			require.NoError(t, db.Model(&model.Log{}).Count(&logs).Error)
			assert.Zero(t, logs, "local quota rejection must not create error/consume logs")
			require.NoError(t, db.First(&ch, ch.Id).Error)
			assert.Equal(t, common.ChannelStatusEnabled, ch.Status)
			if preference == "subscription_only" {
				assert.Less(t, minToken.Load(), int64(token.RemainQuota), "real token preconsume must occur before failed funding then roll back exactly")
			} else {
				assert.Equal(t, int64(token.RemainQuota), minToken.Load())
			}
			oldEnabled, oldKeywords, oldRanges := common.AutomaticDisableChannelEnabled, operation_setting.AutomaticDisableKeywords, operation_setting.AutomaticDisableStatusCodeRanges
			common.AutomaticDisableChannelEnabled = true
			operation_setting.AutomaticDisableKeywords = []string{"用户额度不足", "预扣费额度失败", "订阅额度不足或未配置订阅"}
			operation_setting.AutomaticDisableStatusCodeRanges = nil
			t.Cleanup(func() {
				common.AutomaticDisableChannelEnabled = oldEnabled
				operation_setting.AutomaticDisableKeywords = oldKeywords
				operation_setting.AutomaticDisableStatusCodeRanges = oldRanges
			})
			downstream := types.NewErrorWithStatusCode(fmt.Errorf("%s", payload.Error.Message), types.ErrorCodeInsufficientUserQuota, 403)
			assert.True(t, service.ShouldDisableChannel(downstream), "old downstream provider classification still matches Chinese keyword")
		})
	}
}
