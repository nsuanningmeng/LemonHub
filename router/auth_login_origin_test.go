package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLoginOriginRouter(t *testing.T) *gin.Engine {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousDBType := common.MainDatabaseType()
	previousRedis, previousSecure := common.RedisEnabled, common.SessionCookieSecure
	previousSecret, previousTrusted := common.SessionSecret, common.SessionCookieTrustedURLs
	previousPassword, previousCaptcha := common.PasswordLoginEnabled, common.TurnstileCheckEnabled
	previousGlobalLimit, previousCriticalLimit := common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.TwoFA{}, &model.Log{}))
	require.NoError(t, i18n.Init())
	model.DB, model.LOG_DB = db, db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled, common.SessionCookieSecure = false, true
	common.SessionSecret, common.SessionCookieTrustedURLs = "login-origin-regression-test-secret", nil
	common.PasswordLoginEnabled, common.TurnstileCheckEnabled = true, false
	common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetMainDatabaseType(previousDBType)
		common.RedisEnabled, common.SessionCookieSecure = previousRedis, previousSecure
		common.SessionSecret, common.SessionCookieTrustedURLs = previousSecret, previousTrusted
		common.PasswordLoginEnabled, common.TurnstileCheckEnabled = previousPassword, previousCaptcha
		common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = previousGlobalLimit, previousCriticalLimit
		assert.NoError(t, sqlDB.Close())
	})
	password, err := common.Password2Hash("valid-attacker-password")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "attacker", Password: password, Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, Group: "default", AuthVersion: 1,
	}).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// A proxy adding permissive CORS must not bypass the server's Origin check.
	engine.Use(middleware.CORS())
	SetApiRouter(engine)
	return engine
}

func TestPasswordLoginRejectsCrossSiteCredentials(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		secure      bool
		status      int
	}{
		{name: "HTML text plain form", contentType: "text/plain", secure: true, status: http.StatusUnsupportedMediaType},
		{name: "JSON with permissive CORS", contentType: "application/json", secure: true, status: http.StatusForbidden},
		{name: "insecure mode still rejects foreign JSON", contentType: "application/json", status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := setupLoginOriginRouter(t)
			common.SessionCookieSecure = test.secure
			// A text/plain HTML form can supply this valid JSON: the final field
			// absorbs the form encoding's '=' and the trailing CRLF is whitespace.
			request := httptest.NewRequest(http.MethodPost, "https://panel.example/api/user/login", strings.NewReader("{\"username\":\"attacker\",\"password\":\"valid-attacker-password\",\"padding\":\"=\"}\r\n"))
			request.Header.Set("Origin", "https://attacker.example")
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code)
			assert.Empty(t, response.Result().Cookies(), "rejected credentials must not install a refresh cookie")
			var count int64
			require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&count).Error)
			assert.Zero(t, count, "cross-site login must not create a session")
		})
	}
}

func TestPasswordLoginAcceptsJSONBrowserAndNonBrowserClients(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
		origin string
		secure bool
	}{
		{name: "same origin browser", target: "https://panel.example", origin: "https://panel.example", secure: true},
		{name: "JSON CLI without Origin", target: "https://panel.example", secure: true},
		{name: "Rsbuild development proxy", target: "http://localhost:3000", origin: "http://localhost:5173"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := setupLoginOriginRouter(t)
			common.SessionCookieSecure = test.secure
			request := httptest.NewRequest(http.MethodPost, test.target+"/api/user/login", strings.NewReader(`{"username":"attacker","password":"valid-attacker-password"}`))
			request.Header.Set("Content-Type", "application/json; charset=utf-8")
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					AccessToken string `json:"access_token"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			require.True(t, result.Success, response.Body.String())
			_, err := service.ParseAccessToken(result.Data.AccessToken)
			require.NoError(t, err)
			require.Len(t, response.Result().Cookies(), 1)
			assert.Equal(t, service.RefreshCookieName, response.Result().Cookies()[0].Name)
		})
	}
}

func TestAnonymousSessionAndSetupRoutesRejectCrossSiteRequests(t *testing.T) {
	engine := setupLoginOriginRouter(t)
	for _, path := range []string{"/api/setup", "/api/user/login/2fa", "/api/user/passkey/login/finish"} {
		for _, test := range []struct {
			contentType string
			status      int
		}{
			{contentType: "text/plain", status: http.StatusUnsupportedMediaType},
			{contentType: "application/json", status: http.StatusForbidden},
		} {
			t.Run(path+"/"+test.contentType, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, "http://localhost:3000"+path, strings.NewReader(`{}`))
				request.Header.Set("Origin", "https://attacker.example")
				request.Header.Set("Content-Type", test.contentType)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Equal(t, test.status, response.Code)
				assert.Empty(t, response.Result().Cookies())
			})
		}
	}
}
