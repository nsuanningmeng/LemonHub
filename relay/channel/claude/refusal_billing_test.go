package claude

import (
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClaudePreOutputRefusalReturnsPrecharge(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, format := range []relaytypes.RelayFormat{relaytypes.RelayFormatClaude, relaytypes.RelayFormatOpenAI} {
			for _, stream := range []bool{false, true} {
				for _, pricing := range []string{"ratio", "fixed", "expression"} {
					name := pricing + "/" + string(format)
					if enabled {
						name += "/enabled"
					} else {
						name += "/default-disabled"
					}
					if stream {
						name += "/stream"
					}
					t.Run(name, func(t *testing.T) {
						c, info, db := refusalSettlementFixture(t)
						model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = enabled
						info.RelayFormat, info.IsStream = format, stream
						expectedQuota := 2060
						if pricing == "fixed" {
							info.PriceData.UsePrice, info.PriceData.ModelPrice = true, 0.01
							expectedQuota = 5000
						} else if pricing == "expression" {
							info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
								BillingMode: "tiered_expr", ExprString: `tier("base", 1234)`,
								QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1,
							}
							expectedQuota = 617
						}
						if enabled {
							expectedQuota = 0
						}
						require.Nil(t, service.PreConsumeBilling(c, 3000, info))
						var user model.User
						require.NoError(t, db.First(&user, info.UserId).Error)
						assert.Equal(t, 7000, user.Quota, "fixture really precharges before handling the response")
						parsed := &ClaudeResponseInfo{Usage: &dto.Usage{}, ResponseText: strings.Builder{}}
						if stream {
							for _, event := range []string{refusalStart, refusalDelta, refusalStop} {
								require.Nil(t, HandleStreamResponseData(c, info, parsed, event))
							}
							HandleStreamFinalResponse(c, info, parsed)
						} else {
							response := `{"type":"message","content":[],"stop_reason":"refusal","usage":{"input_tokens":412,"output_tokens":0}}`
							require.Nil(t, HandleClaudeResponseData(c, info, parsed, nil, []byte(response)))
						}
						before, err := common.Marshal(parsed.Usage)
						require.NoError(t, err)
						service.PostTextConsumeQuota(c, info, parsed.Usage, nil)
						after, err := common.Marshal(parsed.Usage)
						require.NoError(t, err)
						assert.JSONEq(t, string(before), string(after), "settlement must not erase reference tokens")
						require.NoError(t, db.First(&user, info.UserId).Error)
						var token model.Token
						require.NoError(t, db.First(&token, info.TokenId).Error)
						assert.Equal(t, 10000-expectedQuota, user.Quota)
						assert.Equal(t, 10000-expectedQuota, token.RemainQuota)
						assert.Equal(t, expectedQuota, token.UsedQuota)
						assert.Equal(t, expectedQuota, user.UsedQuota)
						var consumeLog model.Log
						require.NoError(t, db.First(&consumeLog).Error)
						assert.Equal(t, expectedQuota, consumeLog.Quota)
						assert.Equal(t, 412, consumeLog.PromptTokens, "raw usage remains available for audit")
						require.NotNil(t, consumeLog.InputTokensTotal)
						assert.Equal(t, int64(412), *consumeLog.InputTokensTotal, "free requests retain their reference statistics")
						assert.NotContains(t, consumeLog.Content, "上游没有返回计费信息")
						var other map[string]interface{}
						require.NoError(t, common.UnmarshalJsonStr(consumeLog.Other, &other))
						if enabled {
							assert.Equal(t, "claude_pre_output_refusal", other["billing_exempt_reason"])
						} else {
							assert.NotContains(t, other, "billing_exempt_reason")
						}
						require.NoError(t, service.SettleBilling(c, info, expectedQuota), "settlement retry remains idempotent")
						require.NoError(t, db.First(&user, info.UserId).Error)
						require.NoError(t, db.First(&token, info.TokenId).Error)
						assert.Equal(t, 10000-expectedQuota, user.Quota)
						assert.Equal(t, 10000-expectedQuota, token.RemainQuota)
					})
				}
			}
		}
	}
}

func refusalSettlementFixture(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldBatch, oldLog, oldExport := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	oldSettings, oldQuotaPerUnit := *model_setting.GetGlobalSettings(), common.QuotaPerUnit
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled = oldRedis, oldBatch, oldLog, oldExport
		*model_setting.GetGlobalSettings(), common.QuotaPerUnit = oldSettings, oldQuotaPerUnit
		common.SetDatabaseTypes(oldMainType, oldLogType)
	})
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled = false, false, true, false
	common.QuotaPerUnit = 500000
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}))
	user := model.User{Id: 7551, Username: "refusal-user", Quota: 10000, Status: common.UserStatusEnabled}
	token := model.Token{Id: 7551, UserId: user.Id, Key: "refusal-token", RemainQuota: 10000, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 7551, Name: "claude-refusal", Key: "test", Status: common.ChannelStatusEnabled}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, UserQuota: 10000,
		UserSetting:     dto.UserSetting{BillingPreference: "wallet_only"},
		OriginModelName: "claude-test", RelayFormat: relaytypes.RelayFormatClaude,
		FinalRequestRelayFormat: relaytypes.RelayFormatClaude, StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7551, ChannelType: constant.ChannelTypeAnthropic},
		PriceData:   hosttypes.PriceData{ModelRatio: 5, CompletionRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}
	// Cleanup is LIFO: finish PostTextConsumeQuota's asynchronous samples before
	// closing the fixture database or restoring any globals. gopool workers exit
	// when their queue is empty, and WorkerCount's atomic read observes task exit.
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for gopool.WorkerCount() != 0 {
			require.True(t, time.Now().Before(deadline), "settlement background tasks did not finish")
			runtime.Gosched()
		}
	})
	return c, info, db
}

func TestClaudeZeroTokenRefusalPreservesReferenceUsageWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "default-disabled retains legacy fallback"
		if enabled {
			name = "enabled preserves explicit zero"
		}
		t.Run(name, func(t *testing.T) {
			c, info, db := refusalSettlementFixture(t)
			model_setting.GetGlobalSettings().ClaudePreOutputRefusalFreeEnabled = enabled
			info.SetEstimatePromptTokens(777)
			require.Nil(t, service.PreConsumeBilling(c, 3000, info))
			parsed := &ClaudeResponseInfo{Usage: &dto.Usage{}}
			for _, event := range []string{strings.Replace(refusalStart, "412", "0", 1), refusalDelta, refusalStop} {
				require.Nil(t, HandleStreamResponseData(c, info, parsed, event))
			}
			HandleStreamFinalResponse(c, info, parsed)
			expectedTokens, expectedQuota := 777, 3885
			if enabled {
				expectedTokens, expectedQuota = 0, 0
			}
			assert.Equal(t, expectedTokens, parsed.Usage.PromptTokens)
			require.NotNil(t, parsed.Usage.BillingUsage)
			assert.Equal(t, expectedTokens, parsed.Usage.BillingUsage.ClaudeUsage.InputTokens)
			service.PostTextConsumeQuota(c, info, parsed.Usage, nil)
			var consumeLog model.Log
			require.NoError(t, db.First(&consumeLog).Error)
			assert.Equal(t, expectedTokens, consumeLog.PromptTokens)
			require.NotNil(t, consumeLog.InputTokensTotal)
			assert.Equal(t, int64(expectedTokens), *consumeLog.InputTokensTotal)
			assert.Equal(t, expectedQuota, consumeLog.Quota)
			assert.NotContains(t, consumeLog.Content, "上游没有返回计费信息")
			var user model.User
			require.NoError(t, db.First(&user, info.UserId).Error)
			assert.Equal(t, 10000-expectedQuota, user.Quota)
		})
	}
}

func TestClaudeCachedUsageStatisticsAcrossResponseFormats(t *testing.T) {
	for _, format := range []relaytypes.RelayFormat{relaytypes.RelayFormatClaude, relaytypes.RelayFormatOpenAI} {
		for _, stream := range []bool{false, true} {
			name := string(format)
			if stream {
				name += "/stream"
			}
			t.Run(name, func(t *testing.T) {
				c, info, db := refusalSettlementFixture(t)
				info.RelayFormat, info.IsStream = format, stream
				info.PriceData.CacheRatio, info.PriceData.CacheCreationRatio = 0.1, 1.25
				info.PriceData.CacheCreation5mRatio = 1.25
				parsed := &ClaudeResponseInfo{Usage: &dto.Usage{}, ResponseText: strings.Builder{}}
				if stream {
					for _, event := range []string{
						`{"type":"message_start","message":{"type":"message","content":[],"usage":{"input_tokens":2,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"output_tokens":0}}}`,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
						`{"type":"message_stop"}`,
					} {
						require.Nil(t, HandleStreamResponseData(c, info, parsed, event))
					}
					HandleStreamFinalResponse(c, info, parsed)
				} else {
					response := `{"type":"message","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"output_tokens":5}}`
					require.Nil(t, HandleClaudeResponseData(c, info, parsed, nil, []byte(response)))
				}
				snapshot := dto.CloneBillingUsage(parsed.Usage.BillingUsage)
				require.Nil(t, service.PreConsumeBilling(c, 3000, info))
				service.PostTextConsumeQuota(c, info, parsed.Usage, nil)
				assert.Equal(t, snapshot, parsed.Usage.BillingUsage)
				var consumeLog model.Log
				require.NoError(t, db.First(&consumeLog).Error)
				require.NotNil(t, consumeLog.InputTokensTotal)
				assert.Equal(t, int64(1202), *consumeLog.InputTokensTotal)
				assert.Equal(t, 2, consumeLog.PromptTokens)
				assert.Equal(t, 5, consumeLog.CompletionTokens)
				assert.Equal(t, 1785, consumeLog.Quota)
				stat, err := model.SumUserUsedQuota(info.UserId, model.LogTypeConsume, 0, 0, "", "", 0, "")
				require.NoError(t, err)
				assert.Equal(t, int64(1207), stat.Tpm)
			})
		}
	}
}
