package controller

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNonstandardLoginRejectsCopiedCredentials(t *testing.T) {
	for _, provider := range []string{"wechat", "telegram"} {
		t.Run(provider, func(t *testing.T) {
			setupAuthFlowControllerTest(t)
			previousWeChat, previousTelegram := common.WeChatAuthEnabled, common.TelegramOAuthEnabled
			previousAddress, previousToken := common.WeChatServerAddress, common.TelegramBotToken
			common.WeChatAuthEnabled, common.TelegramOAuthEnabled = true, true
			common.TelegramBotToken = "browser-binding-telegram-test"
			t.Cleanup(func() {
				common.WeChatAuthEnabled, common.TelegramOAuthEnabled = previousWeChat, previousTelegram
				common.WeChatServerAddress, common.TelegramBotToken = previousAddress, previousToken
			})
			upstreamCalls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls++
				_, _ = w.Write([]byte(`{"success":false,"message":"unexpected foreign-browser exchange"}`))
			}))
			t.Cleanup(upstream.Close)
			common.WeChatServerAddress = upstream.URL
			router := gin.New()
			router.GET("/api/oauth/wechat", WeChatAuth)
			router.GET("/api/oauth/telegram/login", TelegramLogin)
			target := "/api/oauth/wechat?code=attacker-code"
			if provider == "telegram" {
				target = "/api/oauth/telegram/login?" + signedTelegramAuthorization(common.TelegramBotToken, time.Now()).Encode()
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Zero(t, upstreamCalls, "copied credentials must be rejected before an upstream exchange")
			assert.Empty(t, response.Result().Cookies())
		})
	}
}

func setupOAuthBrowserLoginUser(t *testing.T) *model.User {
	t.Helper()
	setupAuthFlowControllerTest(t)
	previousLogDB, previousRedis := model.LOG_DB, common.RedisEnabled
	previousActiveLimit, previousIssuanceLimit := common.UserSessionActiveLimit, common.UserSessionIssuanceLimit
	previousIssuanceWindow := common.UserSessionIssuanceWindowSeconds
	model.LOG_DB = model.DB
	common.RedisEnabled = false
	common.UserSessionActiveLimit = 5
	common.UserSessionIssuanceLimit = 10
	common.UserSessionIssuanceWindowSeconds = 3600
	t.Cleanup(func() {
		model.LOG_DB, common.RedisEnabled = previousLogDB, previousRedis
		common.UserSessionActiveLimit, common.UserSessionIssuanceLimit = previousActiveLimit, previousIssuanceLimit
		common.UserSessionIssuanceWindowSeconds = previousIssuanceWindow
	})
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.ExternalIdentityClaim{}, &model.UserSession{}, &model.Log{}))
	user := model.User{
		Username: "oauth-browser-owner", Password: "unused", GitHubId: "7654321",
		WeChatId: "wechat-browser-owner", TelegramId: "123456",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
		Group: "default",
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.UpdateUserBindColumn(user.Id, "github_id", user.GitHubId))
	require.NoError(t, model.UpdateUserBindColumn(user.Id, "wechat_id", user.WeChatId))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.ClaimExternalIdentityWithTx(tx, model.ExternalIdentityProviderTelegram, user.TelegramId, user.Id)
	}))
	return &user
}

func TestOAuthLoginConcurrentTabsPreserveBindingsAndIssueValidSessions(t *testing.T) {
	user := setupOAuthBrowserLoginUser(t)
	provider := &githubIdentityTestProvider{identity: oauth.OAuthUser{ProviderUserID: user.GitHubId}}
	oauth.Register("github-browser-test", provider)
	t.Cleanup(func() { oauth.Unregister("github-browser-test") })
	router := gin.New()
	router.GET("/api/oauth/:provider", HandleOAuth)

	firstState, firstCookies := startOAuthLoginFlow(t, "github-browser-test")
	secondState, secondCookies := startOAuthLoginFlow(t, "github-browser-test")
	require.Len(t, firstCookies, 1)
	require.Len(t, secondCookies, 1)
	assert.NotEqual(t, firstCookies[0].Name, secondCookies[0].Name)
	browser, err := cookiejar.New(nil)
	require.NoError(t, err)
	callbackURL, err := url.Parse("https://gateway.example/api/oauth/github-browser-test")
	require.NoError(t, err)
	browser.SetCookies(callbackURL, firstCookies)
	browser.SetCookies(callbackURL, secondCookies)

	// The same state/code copied into a fresh browser cannot create a session.
	foreignResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignResponse, httptest.NewRequest(http.MethodGet, callbackURL.String()+"?state="+firstState+"&code=valid-code", nil))
	require.Equal(t, http.StatusForbidden, foreignResponse.Code)
	var sessionCount int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&sessionCount).Error)
	assert.Zero(t, sessionCount)

	for _, state := range []string{secondState, firstState} {
		request := httptest.NewRequest(http.MethodGet, callbackURL.String()+"?state="+state+"&code=valid-code", nil)
		request.Header.Set("Referer", "https://github.com/")
		for _, cookie := range browser.Cookies(callbackURL) {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var result struct {
			Success bool               `json:"success"`
			Data    service.AuthBundle `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		identity, err := service.ParseAccessToken(result.Data.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, user.Id, identity.UserID)
		_, authenticatedUser, err := service.ValidateLoginSession(identity)
		require.NoError(t, err)
		assert.Equal(t, user.Id, authenticatedUser.Id)
		browser.SetCookies(callbackURL, response.Result().Cookies())
	}
	assert.Empty(t, browser.Cookies(callbackURL), "both flow cookies are cleared; the refresh cookie is scoped to a different path")
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&sessionCount).Error)
	assert.EqualValues(t, 2, sessionCount)

	replay := httptest.NewRequest(http.MethodGet, callbackURL.String()+"?state="+firstState+"&code=valid-code", nil)
	replay.AddCookie(firstCookies[0])
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replay)
	assert.Equal(t, http.StatusForbidden, replayResponse.Code)
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&sessionCount).Error)
	assert.EqualValues(t, 2, sessionCount)
}

func TestNonstandardLoginRequiresMatchingHeaderAndBrowserCookie(t *testing.T) {
	for _, provider := range []string{"wechat", "telegram"} {
		for _, scenario := range []string{"missing header", "missing cookie", "wrong cookie", "wrong provider"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				setupAuthFlowControllerTest(t)
				previousWeChat, previousTelegram := common.WeChatAuthEnabled, common.TelegramOAuthEnabled
				common.WeChatAuthEnabled, common.TelegramOAuthEnabled = true, true
				t.Cleanup(func() {
					common.WeChatAuthEnabled, common.TelegramOAuthEnabled = previousWeChat, previousTelegram
				})
				flowProvider := provider
				if scenario == "wrong provider" {
					flowProvider = "auth-flow-test"
				}
				state, cookies := startOAuthLoginFlow(t, flowProvider)
				require.Len(t, cookies, 1)
				target := "/api/oauth/wechat?code=attacker-code"
				if provider == "telegram" {
					target = "/api/oauth/telegram/login?id=123456&hash=attacker-signature"
				}
				request := httptest.NewRequest(http.MethodGet, target, nil)
				if scenario != "missing header" {
					request.Header.Set("X-OAuth-State", state)
				}
				if scenario != "missing cookie" {
					if scenario == "wrong cookie" {
						cookies[0].Value = "attacker-secret"
					}
					request.AddCookie(cookies[0])
				}
				router := gin.New()
				router.GET("/api/oauth/wechat", WeChatAuth)
				router.GET("/api/oauth/telegram/login", TelegramLogin)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assert.Equal(t, http.StatusForbidden, response.Code)
				assert.Empty(t, response.Result().Cookies())
				_, err := model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
				assert.NoError(t, err)
			})
		}
	}
}

func TestNonstandardLoginIssuesSessionAndRejectsCredentialReplay(t *testing.T) {
	for _, provider := range []string{"wechat", "telegram"} {
		t.Run(provider, func(t *testing.T) {
			user := setupOAuthBrowserLoginUser(t)
			previousWeChat, previousTelegram := common.WeChatAuthEnabled, common.TelegramOAuthEnabled
			previousAddress, previousToken := common.WeChatServerAddress, common.TelegramBotToken
			common.WeChatAuthEnabled, common.TelegramOAuthEnabled = true, true
			common.TelegramBotToken = "browser-binding-telegram-test"
			t.Cleanup(func() {
				common.WeChatAuthEnabled, common.TelegramOAuthEnabled = previousWeChat, previousTelegram
				common.WeChatServerAddress, common.TelegramBotToken = previousAddress, previousToken
			})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "valid-code", r.URL.Query().Get("code"))
				body, err := common.Marshal(wechatLoginResponse{Success: true, Data: user.WeChatId})
				assert.NoError(t, err)
				_, _ = w.Write(body)
			}))
			t.Cleanup(upstream.Close)
			common.WeChatServerAddress = upstream.URL
			state, cookies := startOAuthLoginFlow(t, provider)
			require.Len(t, cookies, 1)
			target := "/api/oauth/wechat?code=valid-code"
			if provider == "telegram" {
				target = "/api/oauth/telegram/login?" + signedTelegramAuthorization(common.TelegramBotToken, time.Now()).Encode()
			}
			router := gin.New()
			router.GET("/api/oauth/wechat", WeChatAuth)
			router.GET("/api/oauth/telegram/login", TelegramLogin)
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.Header.Set("X-OAuth-State", state)
			request.AddCookie(cookies[0])
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			var result struct {
				Success bool               `json:"success"`
				Data    service.AuthBundle `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			require.True(t, result.Success, response.Body.String())
			identity, err := service.ParseAccessToken(result.Data.AccessToken)
			require.NoError(t, err)
			_, authenticatedUser, err := service.ValidateLoginSession(identity)
			require.NoError(t, err)
			assert.Equal(t, user.Id, authenticatedUser.Id)
			_, err = model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
			assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
			var cleared bool
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == cookies[0].Name {
					cleared = cookie.MaxAge == -1
				}
			}
			assert.True(t, cleared, "successful login clears only its flow cookie")

			replayResponse := httptest.NewRecorder()
			router.ServeHTTP(replayResponse, request)
			assert.Equal(t, http.StatusForbidden, replayResponse.Code)
			if provider == "telegram" {
				// A new browser-bound flow cannot reuse a consumed Telegram assertion.
				state, cookies = startOAuthLoginFlow(t, provider)
				require.Len(t, cookies, 1)
				replay := httptest.NewRequest(http.MethodGet, target, nil)
				replay.Header.Set("X-OAuth-State", state)
				replay.AddCookie(cookies[0])
				replayResponse = httptest.NewRecorder()
				router.ServeHTTP(replayResponse, replay)
				assert.Equal(t, http.StatusForbidden, replayResponse.Code)
				_, err = model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
				assert.NoError(t, err, "assertion rejection rolls back flow consumption")
			}
			var sessions int64
			require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&sessions).Error)
			assert.EqualValues(t, 1, sessions)
		})
	}
}
