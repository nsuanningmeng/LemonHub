package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayToolConstraintsRejectAndRefundRealPreConsume(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, tc := range []struct {
			name, path, body string
			format           types.RelayFormat
			channelType      int
		}{
			{"Chat auto subset to Claude", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"private_prompt_Failed check: SAFETY_CHECK_TYPE"}],"tools":[{"type":"function","function":{"name":"alpha","parameters":{"type":"object","properties":{}}}},{"type":"function","function":{"name":"beta","parameters":{"type":"object","properties":{}}}}],"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"alpha"}]}}`, types.RelayFormatOpenAI, constant.ChannelTypeAnthropic},
			{"Chat auto subset to Gemini", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"private_prompt_Failed check: SAFETY_CHECK_TYPE"}],"tools":[{"type":"function","function":{"name":"alpha","parameters":{"type":"object","properties":{}}}},{"type":"function","function":{"name":"beta","parameters":{"type":"object","properties":{}}}}],"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"alpha"}]}}`, types.RelayFormatOpenAI, constant.ChannelTypeGemini},
			{"Chat unknown choice mode to Gemini", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"alpha","parameters":{"type":"object","properties":{}}}}],"tool_choice":{"type":"allowed_tools","mode":"private_unknown_Failed check: SAFETY_CHECK_TYPE","tools":[{"type":"function","name":"alpha"}]}}`, types.RelayFormatOpenAI, constant.ChannelTypeGemini},
			{"Chat required multi subset to Claude", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"alpha","parameters":{"type":"object","properties":{}}}},{"type":"function","function":{"name":"beta","parameters":{"type":"object","properties":{}}}}],"tool_choice":{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"alpha"},{"type":"function","name":"beta"}]}}`, types.RelayFormatOpenAI, constant.ChannelTypeAnthropic},
			{"Chat parallel false to Gemini", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"alpha","parameters":{"type":"object","properties":{}}}}],"parallel_tool_calls":false}`, types.RelayFormatOpenAI, constant.ChannelTypeGemini},
			{"Chat strict to Gemini", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"alpha","strict":true,"parameters":{"type":"object","properties":{"secret":{"type":"string","const":"private_schema_Failed check: SAFETY_CHECK_TYPE"}}}}}]}`, types.RelayFormatOpenAI, constant.ChannelTypeGemini},
			{"Claude parallel restriction to Gemini", "/v1/messages", `{"model":"gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"alpha","input_schema":{"type":"object","properties":{}}}],"tool_choice":{"type":"auto","disable_parallel_tool_use":true}}`, types.RelayFormatClaude, constant.ChannelTypeGemini},
			{"Claude caller restriction to Gemini", "/v1/messages", `{"model":"gpt-4o","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"alpha","input_schema":{"type":"object","properties":{}},"allowed_callers":["code_execution_20250825"]}]}`, types.RelayFormatClaude, constant.ChannelTypeGemini},
		} {
			t.Run(preference+"/"+tc.name, func(t *testing.T) {
				db, user, token, sub := mediaConversionBillingFixture(t)
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				t.Cleanup(upstream.Close)
				channel := model.Channel{Id: 7454, Type: tc.channelType, Name: "tool-conversion", Key: "fixture-key", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: channel.Id, Enabled: true}).Error)

				// Observe persisted debits inside the same SQL transaction. A balance-only
				// assertion after the response would also pass if validation rejected early.
				var mu sync.Mutex
				minWallet, minToken, maxSubscription := user.Quota, token.RemainQuota, int64(0)
				var observeErr error
				require.NoError(t, db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("test:observe_tool_preconsume", func(tx *gorm.DB) {
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
				t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:observe_tool_preconsume")) })
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
				assert.NotContains(t, response.Body.String(), "private_")
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

func runProtocolConversionRelay(t *testing.T, user model.User, token model.Token, channel model.Channel, path, body string, format types.RelayFormat) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
	c.Set("token_quota", token.RemainQuota)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-4o"))
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	Relay(c, format)
	return response, c
}

func TestRelayGeminiDeveloperDiagnosticPersistsAndIsAdminOnly(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, source, field string
		format                          types.RelayFormat
	}{
		{"Chat", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"system","content":"PRIVATE_SYSTEM\nline"},{"role":"developer","content":"PRIVATE_DEVELOPER_1"},{"role":"developer","content":"PRIVATE_DEVELOPER_2"},{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"PRIVATE_TOOL","description":"PRIVATE_TOOL_DESCRIPTION","parameters":{"type":"object","properties":{"value":{"type":"string","const":"PRIVATE_SCHEMA"}},"additionalProperties":false}}}]}`, string(types.RelayFormatOpenAI), "messages.role", types.RelayFormatOpenAI},
		{"Responses", "/v1/responses", `{"model":"gpt-4o","instructions":"PRIVATE_SYSTEM\nline","input":[{"role":"developer","content":"PRIVATE_DEVELOPER_1"},{"role":"developer","content":"PRIVATE_DEVELOPER_2"},{"role":"user","content":"hello"}]}`, string(types.RelayFormatOpenAIResponses), "input.role", types.RelayFormatOpenAIResponses},
	} {
		t.Run(tc.name, func(t *testing.T) {

			db, user, token, _ := mediaConversionBillingFixture(t)
			var received map[string]any
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(raw, &received))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`))
			}))
			t.Cleanup(upstream.Close)
			channel := model.Channel{Id: 7564, Type: constant.ChannelTypeGemini, Name: "developer-gemini", Key: "PRIVATE_API_KEY", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			body := tc.body
			response, _ := runProtocolConversionRelay(t, user, token, channel, tc.path, body, tc.format)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, int64(1), calls.Load())
			system := received["systemInstruction"].(map[string]any)
			parts := system["parts"].([]any)
			require.Len(t, parts, 1, "compatibility mapping must preserve existing single merged part")
			assert.Equal(t, "PRIVATE_SYSTEM\nline\nPRIVATE_DEVELOPER_1\nPRIVATE_DEVELOPER_2", parts[0].(map[string]any)["text"])
			var log model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&log).Error)
			var other struct {
				AdminInfo struct {
					Diagnostics []struct{ Code, Source, Target, Field, Message string } `json:"conversion_diagnostics"`
				} `json:"admin_info"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			require.Len(t, other.AdminInfo.Diagnostics, 1, "multiple developer messages produce one diagnostic")
			diagnostic := other.AdminInfo.Diagnostics[0]
			assert.Equal(t, "developer_role_merged", diagnostic.Code)
			assert.Equal(t, tc.source, diagnostic.Source)
			assert.Equal(t, string(types.RelayFormatGemini), diagnostic.Target)
			assert.Equal(t, tc.field, diagnostic.Field)
			assert.NotEmpty(t, diagnostic.Message)
			for _, secret := range []string{"PRIVATE_SYSTEM", "PRIVATE_DEVELOPER", "PRIVATE_TOOL", "PRIVATE_SCHEMA", "PRIVATE_API_KEY"} {
				assert.NotContains(t, log.Other, secret, "consume metadata must be redacted")
				assert.NotContains(t, response.Body.String(), secret, "no client notification leaks input")
			}
			adminLogs, total, err := model.GetAllLogs(model.LogTypeConsume, 0, 0, "", "", "", 0, 10, 0, "", "", "", model.SiteScopeAll)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, adminLogs, 1)
			assert.Contains(t, adminLogs[0].Other, "conversion_diagnostics")
			userLogs, total, err := model.GetUserLogs(user.Id, model.LogTypeConsume, 0, 0, "", "", 0, 10, "", "", "")
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, userLogs, 1)
			assert.NotContains(t, userLogs[0].Other, "conversion_diagnostics")
			assert.NotContains(t, userLogs[0].Other, "admin_info")
			// Privacy filtering must not erase the administrator's persisted evidence.
			require.NoError(t, db.First(&log, log.Id).Error)
			assert.Contains(t, log.Other, "conversion_diagnostics")

		})
	}
}

func TestRelayToolAllowedSubsetPreservesProviderWire(t *testing.T) {
	const body = `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"alpha","parameters":{"type":"object","properties":{"value":{"type":"string","const":"PRIVATE_SCHEMA","oneOf":[{"enum":["PRIVATE_SCHEMA"]},{"enum":["other"]}]}},"additionalProperties":false}}},{"type":"function","function":{"name":"beta","parameters":{"type":"object","properties":{}}}}],"tool_choice":{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"alpha"}]}}`
	for _, channelType := range []int{constant.ChannelTypeAnthropic, constant.ChannelTypeGemini} {
		t.Run(strconv.Itoa(channelType), func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			var wire map[string]any
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(raw, &wire))
				w.Header().Set("Content-Type", "application/json")
				if channelType == constant.ChannelTypeAnthropic {
					_, _ = w.Write([]byte(`{"id":"msg","type":"message","role":"assistant","model":"gpt-4o","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
				} else {
					_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`))
				}
			}))
			t.Cleanup(upstream.Close)
			channel := model.Channel{Id: 7456, Type: channelType, Name: "subset", Key: "fixture", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			response, _ := runProtocolConversionRelay(t, user, token, channel, "/v1/chat/completions", body, types.RelayFormatOpenAI)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			if channelType == constant.ChannelTypeAnthropic {
				choice := wire["tool_choice"].(map[string]any)
				assert.Equal(t, "tool", choice["type"])
				assert.Equal(t, "alpha", choice["name"])
			} else {
				config := wire["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)
				assert.Equal(t, "ANY", config["mode"])
				assert.Equal(t, []any{"alpha"}, config["allowedFunctionNames"])
				tools := wire["tools"].([]any)
				decls := tools[0].(map[string]any)["functionDeclarations"].([]any)
				declaration := decls[0].(map[string]any)
				require.Contains(t, declaration, "parametersJsonSchema")
				assert.NotContains(t, declaration, "parameters")
				var expected any
				require.NoError(t, common.Unmarshal([]byte(`{"type":"object","properties":{"value":{"type":"string","const":"PRIVATE_SCHEMA","oneOf":[{"enum":["PRIVATE_SCHEMA"]},{"enum":["other"]}]}},"additionalProperties":false}`), &expected))
				assert.Equal(t, expected, declaration["parametersJsonSchema"])
			}
		})
	}
}

func TestRelayRetryDropsFailedAttemptConversionDiagnostics(t *testing.T) {
	db, user, token, _ := mediaConversionBillingFixture(t)
	var firstCalls, secondCalls atomic.Int64
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary provider failure"}}`))
	}))
	t.Cleanup(first.Close)
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(second.Close)
	initial := model.Channel{Id: 7564, Type: constant.ChannelTypeGemini, Name: "lossy-first", Key: "fixture", BaseURL: common.GetPointer(first.URL), Status: common.ChannelStatusEnabled}
	backup := model.Channel{Id: 7565, Type: constant.ChannelTypeOpenAI, Name: "native-second", Key: "fixture", BaseURL: common.GetPointer(second.URL), Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&initial).Error)
	require.NoError(t, db.Create(&backup).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o", ChannelId: backup.Id, Enabled: true}).Error)
	response, c := runProtocolConversionRelay(t, user, token, initial, "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"developer","content":"PRIVATE_DEVELOPER"},{"role":"user","content":"hello"}]}`, types.RelayFormatOpenAI)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, int64(1), firstCalls.Load())
	assert.Equal(t, int64(1), secondCalls.Load())
	assert.Equal(t, []string{"7564", "7565"}, c.GetStringSlice("use_channel"))
	var log model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&log).Error)
	assert.NotContains(t, log.Other, "conversion_diagnostics", "native successful attempt must not inherit Gemini's failed mapping")
	assert.NotContains(t, log.Other, "PRIVATE_DEVELOPER")
}
