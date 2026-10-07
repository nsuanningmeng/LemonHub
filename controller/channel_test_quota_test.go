package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTestBillingExemptionRequiresEnabledTrustedEvidence(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	oldEnabled := settings.ClaudePreOutputRefusalFreeEnabled
	t.Cleanup(func() { settings.ClaudePreOutputRefusalFreeEnabled = oldEnabled })
	for _, tt := range []struct {
		name    string
		enabled bool
		trusted bool
		want    int
	}{
		{"default policy bills refusal", false, true, 100},
		{"enabled policy waives proven refusal", true, true, 0},
		{"admin rejection alone remains billable", true, false, 100},
	} {
		for _, mode := range []string{"tokens", "per request", "expression"} {
			t.Run(mode+"/"+tt.name, func(t *testing.T) {
				settings.ClaudePreOutputRefusalFreeEnabled = tt.enabled
				usage := &dto.Usage{PromptTokens: 100, TotalTokens: 100}
				usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{InputTokens: 100})
				usage.BillingUsage.ClaudePreOutputRefusal = tt.trusted
				price := types.PriceData{ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
				if mode == "per request" {
					price.UsePrice = true
					price.ModelPrice = 100 / common.QuotaPerUnit
				}
				if mode == "expression" {
					const expr = `tier("base", p * 2)`
					info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
						BillingMode: "tiered_expr", ExprString: expr,
						ExprHash: billingexpr.ExprHashString(expr), ExprVersion: 1,
						QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1,
					}
				}
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				common.SetContextKey(ctx, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")

				quota, result := settleTestQuota(info, price, usage)
				other := buildTestLogOther(ctx, info, price, usage, result)

				assert.Equal(t, tt.want, quota)
				if tt.want == 0 {
					assert.Equal(t, "claude_pre_output_refusal", other["billing_exempt_reason"])
					assert.Nil(t, result)
				} else {
					assert.NotContains(t, other, "billing_exempt_reason")
				}
				assert.Equal(t, 100, usage.PromptTokens, "waiving cost must preserve recorded usage")
				assert.Equal(t, 100, usage.TotalTokens)
			})
		}
	}
}

func TestSettleTestQuotaAppliesResolvedGroupRatioOnce(t *testing.T) {
	for _, tt := range []struct {
		name       string
		groupRatio float64
		want       int
	}{
		{"free group", 0, 0},
		{"discounted group", 0.5, 50},
		{"marked up group", 2, 200},
	} {
		for _, mode := range []string{"tokens", "per request", "expression"} {
			t.Run(mode+"/"+tt.name, func(t *testing.T) {
				price := types.PriceData{
					ModelRatio: 1, CompletionRatio: 1,
					GroupRatioInfo: types.GroupRatioInfo{
						GroupRatio: tt.groupRatio, GroupSpecialRatio: 7, HasSpecialRatio: true,
					},
				}
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
				switch mode {
				case "per request":
					price.UsePrice = true
					price.ModelPrice = 100 / common.QuotaPerUnit
				case "expression":
					const expr = `tier("base", p * 2)`
					info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
						BillingMode: "tiered_expr", ExprString: expr,
						ExprHash: billingexpr.ExprHashString(expr), ExprVersion: 1,
						QuotaPerUnit: common.QuotaPerUnit, GroupRatio: tt.groupRatio,
					}
				}

				quota, result := settleTestQuota(info, price, &dto.Usage{PromptTokens: 100})

				assert.Equal(t, tt.want, quota)
				if mode == "expression" {
					require.NotNil(t, result)
					assert.Equal(t, tt.want, result.ActualQuotaAfterGroup)
				} else {
					assert.Nil(t, result)
				}
				assert.Nil(t, info.QuotaClamp)
			})
		}
	}
}

func TestSettleTestQuotaKeepsExistingTokenRoundingAndFreeModelRules(t *testing.T) {
	for _, tt := range []struct {
		name  string
		price types.PriceData
		usage dto.Usage
		want  int
	}{
		{
			name:  "completion rounds before group multiplier",
			price: types.PriceData{ModelRatio: 1, CompletionRatio: 0.5, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}},
			usage: dto.Usage{CompletionTokens: 1}, want: 2,
		},
		{
			name:  "positive fractional charge keeps minimum quota",
			price: types.PriceData{ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0.001}},
			usage: dto.Usage{PromptTokens: 1}, want: 1,
		},
		{
			name:  "free group does not acquire minimum quota",
			price: types.PriceData{ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0}},
			usage: dto.Usage{}, want: 0,
		},
		{
			name:  "zero model ratio remains free in paid group",
			price: types.PriceData{ModelRatio: 0, CompletionRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}},
			usage: dto.Usage{PromptTokens: 100}, want: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			quota, result := settleTestQuota(&relaycommon.RelayInfo{}, tt.price, &tt.usage)
			assert.Equal(t, tt.want, quota)
			assert.Nil(t, result)
		})
	}
}

func TestSettleTestQuotaAuditsGroupMultiplierSaturation(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	for _, usePrice := range []bool{false, true} {
		t.Run(fmt.Sprintf("per-request=%t", usePrice), func(t *testing.T) {
			price := types.PriceData{
				UsePrice: usePrice, ModelPrice: 3000, ModelRatio: 1,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
			}
			usage := &dto.Usage{PromptTokens: 1_500_000_000}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

			quota, result := settleTestQuota(info, price, usage)
			other := buildTestLogOther(ctx, info, price, usage, result)

			assert.Equal(t, common.MaxQuota, quota)
			require.NotNil(t, info.QuotaClamp)
			assert.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
			admin, ok := other["admin_info"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, info.QuotaClamp.AuditMap(), admin["quota_saturation"])
			assert.Equal(t, float64(2), other["group_ratio"])

			firstClamp := info.QuotaClamp
			_, _ = settleTestQuota(info, types.PriceData{
				UsePrice: true, ModelPrice: 1e300, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
			}, usage)
			assert.Same(t, firstClamp, info.QuotaClamp, "later saturation must preserve the first audit marker")
		})
	}
}

func TestChannelProbeLogsGroupAdjustedQuotaWithoutSpendingBalances(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Token{}))
	oldRatios := ratio_setting.ModelRatio2JSONString()
	oldCompletionRatios := ratio_setting.CompletionRatio2JSONString()
	oldLogEnabled, oldExportEnabled := common.LogConsumeEnabled, common.DataExportEnabled
	fetchSetting := system_setting.GetFetchSetting()
	oldFetchSetting := *fetchSetting
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletionRatios))
		common.LogConsumeEnabled, common.DataExportEnabled = oldLogEnabled, oldExportEnabled
		*fetchSetting = oldFetchSetting
	})
	common.LogConsumeEnabled, common.DataExportEnabled = true, false
	*fetchSetting = system_setting.FetchSetting{EnableSSRFProtection: true, AllowPrivateIp: true, AllowedPorts: []string{"1-65535"}}
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"probe-group-quota":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"probe-group-quota":1}`))
	user := model.User{Username: "probe-quota-admin", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 10000, UsedQuota: 17}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "probe-quota-token", RemainQuota: 8000, UsedQuota: 11}
	require.NoError(t, db.Create(&token).Error)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"probe","object":"chat.completion","model":"probe-group-quota","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`))
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Name: "group quota probe", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "test-channel-key", BaseURL: common.GetPointer(upstream.URL), Models: "probe-group-quota", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)

	result := testChannel(context.Background(), &channel, user.Id, "probe-group-quota", string(constant.EndpointTypeOpenAI), false)

	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	var log model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume).First(&log).Error)
	assert.Equal(t, 220, log.Quota)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, float64(2), other["group_ratio"])
	var afterUser model.User
	require.NoError(t, db.First(&afterUser, user.Id).Error)
	assert.Equal(t, user.Quota, afterUser.Quota)
	assert.Equal(t, user.UsedQuota, afterUser.UsedQuota)
	var afterToken model.Token
	require.NoError(t, db.First(&afterToken, token.Id).Error)
	assert.Equal(t, token.RemainQuota, afterToken.RemainQuota)
	assert.Equal(t, token.UsedQuota, afterToken.UsedQuota)
}
