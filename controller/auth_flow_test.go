package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type authFlowTestOAuthProvider struct {
	exchangeErr   error
	userInfoErr   error
	exchangeCalls int
	userInfoCalls int
}

func (*authFlowTestOAuthProvider) GetName() string { return "Auth Flow Test" }
func (*authFlowTestOAuthProvider) IsEnabled() bool { return true }
func (provider *authFlowTestOAuthProvider) ExchangeToken(context.Context, string, *gin.Context) (*oauth.OAuthToken, error) {
	provider.exchangeCalls++
	if provider.exchangeErr != nil {
		return nil, provider.exchangeErr
	}
	return &oauth.OAuthToken{}, nil
}
func (provider *authFlowTestOAuthProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	provider.userInfoCalls++
	if provider.userInfoErr != nil {
		return nil, provider.userInfoErr
	}
	return &oauth.OAuthUser{ProviderUserID: "external-user"}, nil
}
func (*authFlowTestOAuthProvider) IsUserIDTaken(string, int) bool                      { return false }
func (*authFlowTestOAuthProvider) FillUserByProviderID(*model.User, string, int) error { return nil }
func (*authFlowTestOAuthProvider) SetProviderUserID(*model.User, string)               {}
func (*authFlowTestOAuthProvider) GetProviderPrefix() string                           { return "flow_" }
func (*authFlowTestOAuthProvider) ProviderUserIDColumn() string                        { return "" }

func setupAuthFlowControllerTest(t *testing.T) *authFlowTestOAuthProvider {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRegisterEnabled := common.RegisterEnabled
	previousSecret := common.SessionSecret
	previousCookieSecure := common.SessionCookieSecure
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.AuthFlow{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RegisterEnabled = false
	common.SessionSecret = "oauth-browser-binding-test-secret"
	common.SessionCookieSecure = true
	provider := &authFlowTestOAuthProvider{}
	oauth.Register("auth-flow-test", provider)
	t.Cleanup(func() {
		oauth.Unregister("auth-flow-test")
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RegisterEnabled = previousRegisterEnabled
		common.SessionSecret = previousSecret
		common.SessionCookieSecure = previousCookieSecure
		assert.NoError(t, sqlDB.Close())
	})
	return provider
}

func startOAuthLoginFlow(t *testing.T, provider string) (string, []*http.Cookie) {
	t.Helper()
	body, err := common.Marshal(oauthStateRequest{Provider: provider, Intent: model.AuthFlowIntentLogin})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	GenerateOAuthCode(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	require.NotEmpty(t, response.Data.FlowToken)
	return response.Data.FlowToken, recorder.Result().Cookies()
}

func TestOAuthLoginCallbackRejectsDifferentBrowserBeforeCodeExchange(t *testing.T) {
	provider := setupAuthFlowControllerTest(t)
	provider.exchangeErr = errors.New("unexpected exchange for another browser")
	flowToken, _ := startOAuthLoginFlow(t, "auth-flow-test")
	router := gin.New()
	router.GET("/api/oauth/:provider", HandleOAuth)
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+flowToken+"&code=attacker-code", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Zero(t, provider.exchangeCalls, "a callback copied to another browser must not exchange the attacker's code")
	assert.Empty(t, response.Result().Cookies(), "a rejected callback must not replace the browser's login session")
	_, err := model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
	assert.NoError(t, err, "the original browser can still finish its login")
}

func TestOAuthLoginCallbackRejectsMismatchedAndDuplicateCookies(t *testing.T) {
	for _, scenario := range []string{"wrong secret", "other flow", "duplicate cookie"} {
		t.Run(scenario, func(t *testing.T) {
			provider := setupAuthFlowControllerTest(t)
			flowToken, cookies := startOAuthLoginFlow(t, "auth-flow-test")
			require.Len(t, cookies, 1)
			request := httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+flowToken+"&code=attacker-code", nil)
			switch scenario {
			case "wrong secret":
				cookies[0].Value = "attacker-chosen-secret"
			case "other flow":
				_, cookies = startOAuthLoginFlow(t, "auth-flow-test")
			case "duplicate cookie":
				request.AddCookie(cookies[0])
			}
			request.AddCookie(cookies[0])
			router := gin.New()
			router.GET("/api/oauth/:provider", HandleOAuth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Zero(t, provider.exchangeCalls)
			assert.Empty(t, response.Result().Cookies())
			_, err := model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
			assert.NoError(t, err)
		})
	}
}

func TestOAuthLoginCookieUsesHostProtectionInSecureMode(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "local HTTP", true: "secure HTTPS"}[secure], func(t *testing.T) {
			setupAuthFlowControllerTest(t)
			common.SessionCookieSecure = secure
			state, cookies := startOAuthLoginFlow(t, "auth-flow-test")
			require.Len(t, cookies, 1)
			cookie := cookies[0]
			assert.True(t, cookie.HttpOnly)
			assert.Equal(t, secure, cookie.Secure)
			assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
			assert.Empty(t, cookie.Domain)
			assert.Equal(t, 600, cookie.MaxAge)
			if secure {
				assert.True(t, strings.HasPrefix(cookie.Name, "__Host-"))
				assert.Equal(t, "/", cookie.Path)
			} else {
				assert.False(t, strings.HasPrefix(cookie.Name, "__Host-"))
				assert.Equal(t, "/api/oauth", cookie.Path)
			}
			flow, err := model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
			require.NoError(t, err)
			assert.NotContains(t, flow.Payload, cookie.Value, "the database must not store the browser secret")
		})
	}
}

func TestOAuthLoginRejectsLegacyUnboundFlow(t *testing.T) {
	provider := setupAuthFlowControllerTest(t)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: model.AuthFlowIntentLogin,
		Payload: `{}`, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+state+"&code=test", nil)
	request.AddCookie(oauthLoginBrowserCookie(state, "legacy-secret", 600))
	router := gin.New()
	router.GET("/api/oauth/:provider", HandleOAuth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Zero(t, provider.exchangeCalls)
}

func TestGenerateOAuthCodeCarriesAffiliateInLoginFlow(t *testing.T) {
	setupAuthFlowControllerTest(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(`{"provider":"auth-flow-test","intent":"login","aff":"invite-code"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	GenerateOAuthCode(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	flow, err := model.GetAuthFlow(response.Data.FlowToken, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: model.AuthFlowIntentLogin,
	})
	require.NoError(t, err)
	var payload oauthFlowPayload
	require.NoError(t, common.UnmarshalJsonStr(flow.Payload, &payload))
	assert.Equal(t, "invite-code", payload.AffiliateCode)
	assert.Zero(t, flow.UserId)
	assert.Empty(t, flow.SessionId)
}

func TestGenerateOAuthCodeBindsFlowToAuthenticatedSession(t *testing.T) {
	setupAuthFlowControllerTest(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(`{"provider":"auth-flow-test","intent":"bind"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 42)
	c.Set("session_id", "session-42")
	c.Set("auth_version", int64(3))
	c.Set("session_version", int64(2))

	GenerateOAuthCode(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	flow, err := model.GetAuthFlow(response.Data.FlowToken, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: model.AuthFlowIntentBind,
		UserId: 42, SessionId: "session-42",
	})
	require.NoError(t, err)
	assert.Equal(t, 42, flow.UserId)
	assert.Equal(t, "session-42", flow.SessionId)
}

func TestOAuthLoginConsumesFlowOnlyAfterProviderIdentity(t *testing.T) {
	provider := setupAuthFlowControllerTest(t)

	tests := []struct {
		name        string
		exchangeErr error
		userInfoErr error
	}{
		{name: "exchange failure", exchangeErr: errors.New("exchange failed")},
		{name: "user info failure", userInfoErr: errors.New("user info failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider.exchangeErr = test.exchangeErr
			provider.userInfoErr = test.userInfoErr
			token, cookies := startOAuthLoginFlow(t, "auth-flow-test")
			require.Len(t, cookies, 1)

			router := gin.New()
			router.GET("/api/oauth/:provider", HandleOAuth)
			request := httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+token+"&code=test", nil)
			request.AddCookie(cookies[0])
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			flow, err := model.GetAuthFlow(token, model.AuthFlowMatch{
				Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: model.AuthFlowIntentLogin,
			})
			require.NoError(t, err)
			assert.Nil(t, flow.ConsumedAt)
			assert.Empty(t, response.Result().Cookies(), "transient provider failures must preserve the browser binding for retry")
		})
	}
}

func TestOAuthLoginConsumesFlowAfterProviderIdentityAndOnProviderError(t *testing.T) {
	provider := setupAuthFlowControllerTest(t)

	provider.exchangeErr = nil
	provider.userInfoErr = nil
	successToken, cookies := startOAuthLoginFlow(t, "auth-flow-test")
	require.Len(t, cookies, 1)
	router := gin.New()
	router.GET("/api/oauth/:provider", HandleOAuth)
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+successToken+"&code=test", nil)
	request.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	_, err := model.GetAuthFlow(successToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
	assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
	assert.Equal(t, 1, provider.exchangeCalls)
	assert.Equal(t, 1, provider.userInfoCalls)
	cleared := response.Result().Cookies()
	require.Len(t, cleared, 1)
	assert.Equal(t, cookies[0].Name, cleared[0].Name)
	assert.Equal(t, -1, cleared[0].MaxAge)

	providerErrorToken, cookies := startOAuthLoginFlow(t, "auth-flow-test")
	require.Len(t, cookies, 1)
	request = httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+providerErrorToken+"&error=access_denied", nil)
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	_, err = model.GetAuthFlow(providerErrorToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
	assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
	assert.Equal(t, 1, provider.exchangeCalls)
	assert.Equal(t, 1, provider.userInfoCalls)
	cleared = response.Result().Cookies()
	require.Len(t, cleared, 1)
	assert.Equal(t, cookies[0].Name, cleared[0].Name)
	assert.Equal(t, -1, cleared[0].MaxAge)
}

func TestOAuthBindProviderErrorConsumesSessionBoundFlow(t *testing.T) {
	provider := setupAuthFlowControllerTest(t)
	flowToken, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeOAuth, Provider: "auth-flow-test", Intent: model.AuthFlowIntentBind,
		UserId: 42, SessionId: "session-42", Payload: `{}`, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 42)
		c.Set("session_id", "session-42")
		c.Set("auth_version", int64(1))
		c.Set("session_version", int64(1))
		c.Next()
	})
	router.GET("/api/oauth/:provider", HandleOAuth)
	request := httptest.NewRequest(http.MethodGet, "/api/oauth/auth-flow-test?state="+flowToken+"&error=access_denied&error_description=cancelled", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	_, err = model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
	assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
	assert.Zero(t, provider.exchangeCalls)
	assert.Zero(t, provider.userInfoCalls)
}
