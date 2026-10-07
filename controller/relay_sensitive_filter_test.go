package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelaySensitiveFilterRejectsBeforeAnyBilling(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		format           types.RelayFormat
		channelType      int
	}{
		{"chat", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"PRIVATE_ADMIN_WORD Failed check: SAFETY_CHECK_TYPE"}]}`, types.RelayFormatOpenAI, constant.ChannelTypeOpenAI},
		{"responses native tool output", "/v1/responses", `{"model":"gpt-4o","input":[{"role":"user","content":"hello"},{"type":"function_call_output","call_id":"call1","output":"PRIVATE_ADMIN_WORD Failed check: SAFETY_CHECK_TYPE"}]}`, types.RelayFormatOpenAIResponses, constant.ChannelTypeOpenAI},
		{"responses converted tool output", "/v1/responses", `{"model":"gpt-4o","input":[{"role":"user","content":"hello"},{"type":"function_call_output","call_id":"call1","output":"PRIVATE_ADMIN_WORD Failed check: SAFETY_CHECK_TYPE"}]}`, types.RelayFormatOpenAIResponses, constant.ChannelTypeAnthropic},
		{"responses tool output block", "/v1/responses", `{"model":"gpt-4o","input":[{"type":"function_call_output","call_id":"call1","output":[{"type":"input_text","text":"PRIVATE_ADMIN_WORD Failed check: SAFETY_CHECK_TYPE"},{"type":"input_image","image_url":"data:image/png;base64,cchh"}]}]}`, types.RelayFormatOpenAIResponses, constant.ChannelTypeOpenAI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, sub := mediaConversionBillingFixture(t)
			oldPrompt, oldWords := setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWords
			t.Cleanup(func() { setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWords = oldPrompt, oldWords })
			setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled = true, true
			setting.SensitiveWords = []string{"private_admin_word"}
			var calls, updates atomic.Int64
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:sensitive_no_debit", func(tx *gorm.DB) {
				switch tx.Statement.Table {
				case "users", "tokens", "user_subscriptions":
					updates.Add(1)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove("test:sensitive_no_debit")) })
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"resp","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
			}))
			t.Cleanup(upstream.Close)
			channel := model.Channel{Id: 7432, Type: tc.channelType, Name: "sensitive-fixture", Key: "fixture", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
			router := gin.New()
			var selected []string
			router.POST(tc.path, func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
				c.Set("token_quota", token.RemainQuota)
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-4o"))
				defer common.CleanupBodyStorage(c)
				Relay(c, tc.format)
				selected = c.GetStringSlice("use_channel")
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, service.WaitBillingRefunds(ctx))
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			var body struct {
				Error struct{ Code, Message string }
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, string(types.ErrorCodeSensitiveWordsDetected), body.Error.Code)
			assert.NotEmpty(t, body.Error.Message)
			assert.NotContains(t, response.Body.String(), "private_admin_word")
			assert.NotContains(t, response.Body.String(), "PRIVATE_ADMIN_WORD")
			assert.NotContains(t, response.Body.String(), "SAFETY_CHECK_TYPE")
			assert.Zero(t, calls.Load())
			assert.Zero(t, updates.Load(), "no debit followed by refund can satisfy this test")
			assert.Empty(t, selected, "filter runs before channel selection and conversion")
			var finalUser model.User
			var finalToken model.Token
			var finalSub model.UserSubscription
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			require.NoError(t, db.First(&finalToken, token.Id).Error)
			require.NoError(t, db.First(&finalSub, sub.Id).Error)
			assert.Equal(t, user.Quota, finalUser.Quota)
			assert.Equal(t, token.RemainQuota, finalToken.RemainQuota)
			assert.Equal(t, sub.AmountUsed, finalSub.AmountUsed)
			var logs, receipts int64
			require.NoError(t, db.Model(&model.Log{}).Count(&logs).Error)
			require.NoError(t, db.Model(&model.SubscriptionPreConsumeRecord{}).Count(&receipts).Error)
			assert.Zero(t, logs)
			assert.Zero(t, receipts)
		})
	}
}

func TestRelaySensitiveFilterKeepsMediaOpaqueAndHonorsSavedWholeWordPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		format           types.RelayFormat
		whole            bool
	}{
		{"saved whole-word allows identifier", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"AgenticChat"}]}`, types.RelayFormatOpenAI, true},
		{"Responses image is not prompt text", "/v1/responses", `{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,cchh"}]}]}`, types.RelayFormatOpenAIResponses, false},
		{"Responses tool image is not prompt text", "/v1/responses", `{"model":"gpt-4o","input":[{"type":"function_call_output","call_id":"call1","output":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,cchh"}]}]}`, types.RelayFormatOpenAIResponses, false},
		{"Responses encrypted compaction is opaque", "/v1/responses", `{"model":"gpt-4o","input":[{"type":"compaction","encrypted_content":"cch"},{"role":"user","content":"hello"}]}`, types.RelayFormatOpenAIResponses, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			require.NoError(t, db.AutoMigrate(&model.Option{}))
			oldWords, oldPrompt, oldWhole := setting.SensitiveWords, setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWordsWholeWordEnabled
			common.OptionMapRWMutex.Lock()
			oldMap := common.OptionMap
			common.OptionMap = make(map[string]string, len(oldMap))
			for key, value := range oldMap {
				common.OptionMap[key] = value
			}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				setting.SensitiveWords, setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWordsWholeWordEnabled = oldWords, oldPrompt, oldWhole
				common.OptionMapRWMutex.Lock()
				common.OptionMap = oldMap
				common.OptionMapRWMutex.Unlock()
			})
			setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled = true, true
			setting.SensitiveWords = []string{"cch"}
			value := "false"
			if tc.whole {
				value = "true"
			}
			require.NoError(t, model.UpdateOption("SensitiveWordsWholeWordEnabled", value))
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if tc.format == types.RelayFormatOpenAI {
					_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
				} else {
					_, _ = w.Write([]byte(`{"id":"resp","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
				}
			}))
			t.Cleanup(upstream.Close)
			channel := model.Channel{Id: 7432, Type: constant.ChannelTypeOpenAI, Name: "sensitive-media", Key: "fixture", BaseURL: common.GetPointer(upstream.URL), Status: common.ChannelStatusEnabled}
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
			common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
			common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
			common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
			c.Set("token_quota", token.RemainQuota)
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-4o"))
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			Relay(c, tc.format)
			assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, int64(1), calls.Load())
			assert.NotContains(t, response.Body.String(), "sensitive_words_detected")
		})
	}
}
