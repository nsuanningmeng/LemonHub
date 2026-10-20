package relay

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDeepSeekBillingDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.Site{}, &model.SiteDomain{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	t.Cleanup(func() {
		require.NoError(t, db.Where("1 = 1").Delete(&model.Site{}).Error)
		require.NoError(t, model.ReloadSiteCache())
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

// Exercise the real request validator, price lookup, wallet/token reservation,
// DeepSeek helper, settlement and site-scoped consume log as one operation.
func TestDeepSeekToolsRealBillingAndRefunds(t *testing.T) {
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	previousRedis, previousCache, previousBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	previousLogs, previousExport, previousQuotaUnit := common.LogConsumeEnabled, common.DataExportEnabled, common.QuotaPerUnit
	previousPrices := ratio_setting.ModelPrice2JSONString()
	previousGroups, previousGroupGroups := ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	fetchSettings := system_setting.GetFetchSetting()
	previousFetchSettings := *fetchSettings
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
	common.LogConsumeEnabled, common.DataExportEnabled, common.QuotaPerUnit = true, false, 500000
	fetchSettings.EnableSSRFProtection = false
	service.InitHttpClient()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	t.Cleanup(func() {
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = previousRedis, previousCache, previousBatch
		common.LogConsumeEnabled, common.DataExportEnabled, common.QuotaPerUnit = previousLogs, previousExport, previousQuotaUnit
		*fetchSettings = previousFetchSettings
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousGroupGroups))
	})

	for _, tc := range []struct {
		name, path, response, prices   string
		status, siteID, markup, charge int
		wantError, wantResult          bool
	}{
		{name: "search default with legacy price map", path: "/v1/tools/search", response: `{"model":"untrusted-model","results":[{"title":"private-search-result"}]}`, prices: `{}`, status: 200, markup: 10000, charge: 375, wantResult: true},
		{name: "fetch default on subsite", path: "/v1/tools/fetch", response: `{"content":"private-page-content"}`, prices: `{}`, status: 200, siteID: 17, markup: 10000, charge: 375, wantResult: true},
		{name: "authenticated subsite markup", path: "/v1/tools/search", response: `{"results":[{"title":"result"}]}`, prices: `{}`, status: 200, siteID: 17, markup: 20000, charge: 750, wantResult: true},
		{name: "explicit free price", path: "/v1/tools/search", response: `{"results":[{"title":"result"}]}`, prices: `{"deepseek-search":0}`, status: 200, markup: 10000, charge: 0, wantResult: true},
		{name: "explicit custom price", path: "/v1/tools/fetch", response: `{"content":"page content"}`, prices: `{"deepseek-fetch":0.002}`, status: 200, markup: 10000, charge: 1000, wantResult: true},
		{name: "empty search refunded", path: "/v1/tools/search", response: `{"results":[]}`, prices: `{}`, status: 200, siteID: 17, markup: 10000},
		{name: "blank page refunded", path: "/v1/tools/fetch", response: `{"content":"  "}`, prices: `{}`, status: 200, siteID: 17, markup: 10000},
		{name: "wrong result shape refunded", path: "/v1/tools/search", response: `{"results":{"title":"not an array"}}`, prices: `{}`, status: 200, siteID: 17, markup: 10000},
		{name: "malformed response refunded", path: "/v1/tools/search", response: `{"results":[`, prices: `{}`, status: 200, siteID: 17, markup: 10000, wantError: true},
		{name: "upstream failure refunded", path: "/v1/tools/fetch", response: `{"error":{"message":"private-page-content","type":"not_found_error"}}`, prices: `{}`, status: 404, siteID: 17, markup: 10000, wantError: true},
		{name: "HTML failure refunded without body leak", path: "/v1/tools/fetch", response: `<html>private-page-content</html>`, prices: `{}`, status: 502, siteID: 17, markup: 10000, wantError: true},
		{name: "oversized response refunded", path: "/v1/tools/fetch", response: `{"content":"` + strings.Repeat("x", 16<<20) + `"}`, prices: `{}`, status: 200, siteID: 17, markup: 10000, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeLogs bytes.Buffer
			common.LogWriterMu.Lock()
			previousOutput, previousErrorOutput := gin.DefaultWriter, gin.DefaultErrorWriter
			gin.DefaultWriter, gin.DefaultErrorWriter = &runtimeLogs, &runtimeLogs
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultWriter, gin.DefaultErrorWriter = previousOutput, previousErrorOutput
				common.LogWriterMu.Unlock()
			})
			db := setupDeepSeekBillingDB(t)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(tc.prices))
			require.NoError(t, db.Create(&model.Site{Id: 17, Name: "current", ModelPriceRate: tc.markup, WalletBalance: 123456}).Error)
			require.NoError(t, db.Create(&model.Site{Id: 99, Name: "unrelated", ModelPriceRate: 40000, WalletBalance: 654321}).Error)
			require.NoError(t, model.ReloadSiteCache())
			user := model.User{Username: "same-name", SiteId: tc.siteID, AffCode: "owner", Status: common.UserStatusEnabled, Quota: 10000, Group: "default", Setting: `{"billing_preference":"wallet_only","quota_warning_enabled":false}`}
			other := model.User{Username: "same-name", SiteId: 99, AffCode: "other", Status: common.UserStatusEnabled, Quota: 20000, Group: "default"}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&other).Error)
			token := model.Token{UserId: user.Id, SiteId: user.SiteId, Key: "own-token", Status: common.TokenStatusEnabled, RemainQuota: 10000, ExpiredTime: -1}
			otherToken := model.Token{UserId: other.Id, SiteId: other.SiteId, Key: "other-token", Status: common.TokenStatusEnabled, RemainQuota: 20000, ExpiredTime: -1}
			require.NoError(t, db.Create(&token).Error)
			require.NoError(t, db.Create(&otherToken).Error)
			channel := model.Channel{Name: "deepseek", Type: constant.ChannelTypeDeepSeek, Key: "fixture-key", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			body := `{"query":"private-search-query","url":"https://example.com/private-page-path","model":"ignored-client-model"}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.path, r.URL.Path)
				gotBody, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, body, string(gotBody), "the original body is forwarded without changing its model field")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			t.Cleanup(upstream.Close)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
			c.Request.Host = "different-site.example"
			c.Request.Header.Set("Content-Type", "application/json")
			routingName := constant.DeepSeekSearchModel
			if tc.path == "/v1/tools/fetch" {
				routingName = constant.DeepSeekFetchModel
			}
			common.SetContextKey(c, constant.ContextKeyOriginalModel, routingName)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
			common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
			common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
			common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
			common.SetContextKey(c, constant.ContextKeySiteId, user.SiteId)
			common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeDeepSeek)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "fixture-key")
			common.SetContextKey(c, constant.ContextKeyUserSetting, user.GetSetting())
			c.Set("operator_site_id", user.SiteId)
			c.Set("username", user.Username)
			c.Set("token_quota", token.RemainQuota)
			c.Set(common.RequestIdKey, "tools-billing-"+strings.ReplaceAll(tc.name, " ", "-"))
			request, err := helper.GetAndValidateRequest(c, types.RelayFormatDeepSeekTools)
			require.NoError(t, err)
			info, err := relaycommon.GenRelayInfo(c, types.RelayFormatDeepSeekTools, request, nil)
			require.NoError(t, err)
			info.ForcePreConsume = true
			price, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
			require.NoError(t, err)
			require.True(t, price.UsePrice)
			if tc.prices == "{}" {
				assert.Equal(t, 375*tc.markup/10000, price.QuotaToPreConsume)
			} else {
				assert.Equal(t, tc.charge, price.QuotaToPreConsume)
			}
			require.Nil(t, service.PreConsumeBilling(c, price.QuotaToPreConsume, info))
			var reservedUser model.User
			require.NoError(t, db.First(&reservedUser, user.Id).Error)
			assert.Equal(t, 10000-price.QuotaToPreConsume, reservedUser.Quota)
			apiErr := DeepSeekToolsHelper(c, info)
			if tc.wantError {
				require.NotNil(t, apiErr)
				assert.NotContains(t, apiErr.Error(), "private-")
				if tc.status >= 300 {
					assert.Equal(t, tc.status, apiErr.StatusCode)
				}
				// Mirrors the controller's deferred cleanup on helper failure.
				info.Billing.Refund(c)
			} else {
				require.Nil(t, apiErr)
				assert.Equal(t, tc.status, recorder.Code)
				assert.Equal(t, tc.response, recorder.Body.String())
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, service.WaitBillingRefunds(ctx))
			var gotUser, gotOther model.User
			var gotToken, gotOtherToken model.Token
			require.NoError(t, db.First(&gotUser, user.Id).Error)
			require.NoError(t, db.First(&gotOther, other.Id).Error)
			require.NoError(t, db.First(&gotToken, token.Id).Error)
			require.NoError(t, db.First(&gotOtherToken, otherToken.Id).Error)
			assert.Equal(t, 10000-tc.charge, gotUser.Quota)
			assert.Equal(t, tc.charge, gotUser.UsedQuota)
			if tc.wantResult {
				assert.Equal(t, 1, gotUser.RequestCount)
			} else {
				assert.Zero(t, gotUser.RequestCount)
			}
			assert.Equal(t, 10000-tc.charge, gotToken.RemainQuota)
			assert.Equal(t, tc.charge, gotToken.UsedQuota)
			assert.Equal(t, 20000, gotOther.Quota)
			assert.Zero(t, gotOther.UsedQuota)
			assert.Equal(t, 20000, gotOtherToken.RemainQuota)
			assert.Zero(t, gotOtherToken.UsedQuota)
			var gotChannel model.Channel
			require.NoError(t, db.First(&gotChannel, channel.Id).Error)
			assert.EqualValues(t, tc.charge, gotChannel.UsedQuota)
			var currentSite, otherSite model.Site
			require.NoError(t, db.First(&currentSite, 17).Error)
			require.NoError(t, db.First(&otherSite, 99).Error)
			// Site wholesale wallets are charged by top-up, not twice during use.
			assert.EqualValues(t, 123456, currentSite.WalletBalance)
			assert.EqualValues(t, 654321, otherSite.WalletBalance)
			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			if tc.wantResult {
				require.Len(t, logs, 1)
				assert.Equal(t, user.SiteId, logs[0].SiteId)
				assert.Equal(t, user.Id, logs[0].UserId)
				assert.Equal(t, token.Id, logs[0].TokenId)
				assert.Equal(t, routingName, logs[0].ModelName)
				assert.Equal(t, tc.charge, logs[0].Quota)
				assert.Zero(t, logs[0].PromptTokens)
				assert.Zero(t, logs[0].CompletionTokens)
				assert.NotContains(t, logs[0].Content, "private-")
				assert.NotContains(t, logs[0].Other, "private-")
			} else {
				assert.Empty(t, logs)
			}
			assert.False(t, info.Billing.NeedsRefund())
			assert.NotContains(t, runtimeLogs.String(), "private-")
		})
	}
}

// The host-owned flag must not turn other tokenless relay responses into a
// fixed-price charge, even if it is accidentally carried over from a tool call.
func TestDeepSeekResultFlagDoesNotBillOtherFormats(t *testing.T) {
	previousLogs := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogs })
	for _, format := range []types.RelayFormat{
		types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini,
		types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIAlphaSearch,
	} {
		t.Run(string(format), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			settlement := &recordedSettlement{}
			info := &relaycommon.RelayInfo{
				RelayFormat: format, OriginModelName: "unrelated-model",
				BillableToolResult: true, Billing: settlement, StartTime: time.Now(),
				ChannelMeta: &relaycommon.ChannelMeta{},
			}
			info.PriceData.UsePrice = true
			info.PriceData.ModelPrice = 1
			info.PriceData.GroupRatioInfo.GroupRatio = 1
			service.PostTextConsumeQuota(c, info, &dto.Usage{}, nil)
			assert.Equal(t, []int{0}, settlement.settled)
		})
	}
}
