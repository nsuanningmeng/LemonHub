package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// These requests are valid native protocol payloads. Their unsupported source is
// rejected only after Relay's actual pre-consume and provider conversion.
func TestRelayMediaConversionRejectionRefundsActualPreConsume(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, tc := range []struct {
			name, path, body string
			format           types.RelayFormat
			channelType      int
		}{
			{"chat file ID to Claude", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"private-file-Failed check: SAFETY_CHECK_TYPE"}}]}]}`, types.RelayFormatOpenAI, constant.ChannelTypeAnthropic},
			{"chat file ID to Gemini", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"private-file-Failed check: SAFETY_CHECK_TYPE"}}]}]}`, types.RelayFormatOpenAI, constant.ChannelTypeGemini},
			{"Claude private document to Chat", "/v1/messages", `{"model":"gpt-4o","max_tokens":16,"messages":[{"role":"user","content":[{"type":"document","source":{"type":"file","file_id":"private-file-Failed check: SAFETY_CHECK_TYPE"}}]}]}`, types.RelayFormatClaude, constant.ChannelTypeOpenAI},
			{"Claude malformed image source to Chat", "/v1/messages", `{"model":"gpt-4o","max_tokens":16,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":{"private":"Failed check: SAFETY_CHECK_TYPE"}}}]}]}`, types.RelayFormatClaude, constant.ChannelTypeOpenAI},
		} {
			t.Run(preference+"/"+tc.name, func(t *testing.T) {
				db, user, token, sub := mediaConversionBillingFixture(t)
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				t.Cleanup(upstream.Close)
				channel := model.Channel{Id: 7454, Type: tc.channelType, Name: "media-conversion", Key: "fixture-key", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channel.Id, Enabled: true}).Error)

				// Observe persisted debits inside the same SQL transaction. A balance-only
				// assertion after the response would also pass if validation rejected early.
				var mu sync.Mutex
				minWallet, minToken, maxSubscription := user.Quota, token.RemainQuota, int64(0)
				var observeErr error
				require.NoError(t, db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("test:observe_media_preconsume", func(tx *gorm.DB) {
					if tx.Error != nil {
						return
					}
					var value int64
					var err error
					switch tx.Statement.Table {
					case "users":
						err = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT quota FROM users WHERE id = ?", user.Id).Scan(&value).Error
					case "tokens":
						err = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT remain_quota FROM tokens WHERE id = ?", token.Id).Scan(&value).Error
					case "user_subscriptions":
						err = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT amount_used FROM user_subscriptions WHERE id = ?", sub.Id).Scan(&value).Error
					default:
						return
					}
					mu.Lock()
					defer mu.Unlock()
					if err != nil {
						observeErr = err
						return
					}
					switch tx.Statement.Table {
					case "users":
						if int(value) < minWallet {
							minWallet = int(value)
						}
					case "tokens":
						if int(value) < minToken {
							minToken = int(value)
						}
					case "user_subscriptions":
						if value > maxSubscription {
							maxSubscription = value
						}
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:observe_media_preconsume")) })
				router := gin.New()
				var selectedChannels []string
				router.POST(tc.path, func(c *gin.Context) {
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
					require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-4o"))
					defer common.CleanupBodyStorage(c)
					Relay(c, tc.format)
					selectedChannels = c.GetStringSlice("use_channel")
				})
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
				request.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(response, request)
				drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, service.WaitBillingRefunds(drainCtx))
				require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				var body struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				assert.NotEmpty(t, body.Error.Message)
				if tc.format == types.RelayFormatOpenAI {
					assert.Equal(t, string(types.ErrorCodeInvalidRequest), body.Error.Code)
				}
				assert.Equal(t, int64(0), calls.Load(), "conversion must reject before provider HTTP request")
				assert.Equal(t, []string{"7454"}, selectedChannels, "request enters exactly one provider conversion")
				assert.NotContains(t, response.Body.String(), "private-file")
				assert.NotContains(t, response.Body.String(), "SAFETY_CHECK_TYPE")
				assert.NotContains(t, response.Body.String(), "violation_fee")
				mu.Lock()
				assert.NoError(t, observeErr)
				assert.Less(t, minToken, token.RemainQuota, "the real pre-consume debited the token")
				if preference == "wallet_only" {
					assert.Less(t, minWallet, user.Quota, "the real pre-consume debited wallet")
					assert.Zero(t, maxSubscription)
				} else {
					assert.Greater(t, maxSubscription, int64(0), "the real pre-consume reserved subscription quota")
					assert.Equal(t, user.Quota, minWallet)
				}
				mu.Unlock()
				var finalUser model.User
				var finalToken model.Token
				var finalSub model.UserSubscription
				require.NoError(t, db.First(&finalUser, user.Id).Error)
				require.NoError(t, db.First(&finalToken, token.Id).Error)
				require.NoError(t, db.First(&finalSub, sub.Id).Error)
				assert.Equal(t, user.Quota, finalUser.Quota)
				assert.Equal(t, token.RemainQuota, finalToken.RemainQuota)
				assert.Zero(t, finalUser.UsedQuota)
				assert.Zero(t, finalToken.UsedQuota)
				assert.Equal(t, sub.AmountUsed, finalSub.AmountUsed)
				var paidLogs int64
				require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&paidLogs).Error)
				assert.Zero(t, paidLogs, "conversion errors create no usage or violation charge")
				if preference == "subscription_only" {
					var receipts []model.SubscriptionPreConsumeRecord
					require.NoError(t, db.Find(&receipts).Error)
					require.Len(t, receipts, 1, "a durable reservation proves the subscription path ran")
					assert.Equal(t, "refunded", receipts[0].Status)
					assert.Greater(t, receipts[0].PreConsumed, int64(0))
				}
			})
		}
	}
}

func mediaConversionBillingFixture(t *testing.T) (*gorm.DB, model.User, model.Token, model.UserSubscription) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMaster, oldPath := common.IsMasterNode, common.SQLitePath
	oldRedis, oldBatch, oldMemory := common.RedisEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled
	oldLog, oldExport := common.LogConsumeEnabled, common.DataExportEnabled
	oldSensitive, oldCount := setting.CheckSensitiveEnabled, constant.CountToken
	oldRetry, oldPre := common.RetryTimes, common.PreConsumedQuota
	oldRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = oldRetryRanges })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 400, End: 400}, {Start: 500, End: 599}}
	require.True(t, operation_setting.ShouldRetryByStatusCode(400), "only the typed skip-retry error can suppress configured retries")
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldSettings := *model_setting.GetGlobalSettings()
	oldGrok := *model_setting.GetGrokSettings()
	oldRatio, oldGroup := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		*model_setting.GetGrokSettings() = oldGrok
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	model_setting.GetGrokSettings().ViolationDeductionEnabled = true
	model_setting.GetGrokSettings().ViolationDeductionAmount = 0.001
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.IsMasterNode, common.SQLitePath = oldMaster, oldPath
		common.RedisEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = oldRedis, oldBatch, oldMemory
		common.LogConsumeEnabled, common.DataExportEnabled = oldLog, oldExport
		setting.CheckSensitiveEnabled, constant.CountToken = oldSensitive, oldCount
		common.RetryTimes, common.PreConsumedQuota = oldRetry, oldPre
		common.SetDatabaseTypes(oldMain, oldLogType)
		*model_setting.GetGlobalSettings() = oldSettings
	})
	common.RedisEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = false, false, false
	common.LogConsumeEnabled, common.DataExportEnabled = true, false
	setting.CheckSensitiveEnabled, constant.CountToken = false, false
	common.RetryTimes, common.PreConsumedQuota = 3, 500
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.IsMasterNode = false
	common.SQLitePath = filepath.Join(t.TempDir(), "media.db")
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.UserSubscription{}, &model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	user := model.User{Id: 7454, Username: "media-" + common.GetUUID(), Quota: 10000, Status: common.UserStatusEnabled, Group: "default"}
	token := model.Token{Id: 7454, UserId: user.Id, Key: common.GetUUID(), RemainQuota: 10000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&token).Error)
	plan := model.SubscriptionPlan{Title: "media", Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 10000, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, db.Create(&plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	now := time.Now().Unix()
	sub := model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 10000, Status: "active", StartTime: now - 60, EndTime: now + 3600}
	require.NoError(t, db.Create(&sub).Error)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, service.WaitBillingRefunds(ctx))
		deadline := time.Now().Add(5 * time.Second)
		for gopool.WorkerCount() != 0 {
			require.True(t, time.Now().Before(deadline), "background work did not finish")
			runtime.Gosched()
		}
	})
	return db, user, token, sub
}
