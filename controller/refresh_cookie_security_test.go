package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSecureRefreshIgnoresInjectedLegacyDomainCookie(t *testing.T) {
	previousDB, previousRedis := model.DB, common.RedisEnabled
	previousSecret, previousSecure := common.SessionSecret, common.SessionCookieSecure
	previousOrigins := common.SessionCookieTrustedURLs
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB, common.RedisEnabled = db, false
	common.SessionSecret, common.SessionCookieSecure = "refresh-cookie-security-test", true
	common.SessionCookieTrustedURLs = []string{"https://gateway.example.test"}
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		common.SessionSecret, common.SessionCookieSecure = previousSecret, previousSecure
		common.SessionCookieTrustedURLs = previousOrigins
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	owner := model.User{Username: "cookie-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default", AffCode: "owner"}
	attacker := model.User{Username: "cookie-attacker", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default", AffCode: "attacker"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&attacker).Error)
	ownerSession, err := service.CreateLoginSession(owner.Id, "password", "127.0.0.1", "owner")
	require.NoError(t, err)
	attackerSession, err := service.CreateLoginSession(attacker.Id, "password", "127.0.0.1", "attacker")
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/user/auth/refresh", middleware.SessionCookieOriginGuard(), RefreshAuth)

	request := func(cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "https://gateway.example.test/api/user/auth/refresh", nil)
		req.Header.Set("Origin", "https://gateway.example.test")
		// A normal page reload has no X-Auth-Session: session state lives in
		// memory. A sibling domain's longer-Path cookie is sent first, and the
		// browser does not transmit the Domain/Path attributes to the server.
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	injected := &http.Cookie{Name: "new_api_refresh", Value: attackerSession.RefreshToken}
	protected := &http.Cookie{Name: "__Host-new_api_refresh", Value: ownerSession.RefreshToken}
	response := request(injected, protected)
	require.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Data struct {
			User struct {
				ID int `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.Equal(t, owner.Id, result.Data.User.ID, "a legacy Domain cookie must never switch the dashboard to the attacker's account")
	assert.Equal(t, http.StatusUnauthorized, request(injected).Code)
	assert.Equal(t, http.StatusUnauthorized, request(protected, protected).Code)
}
