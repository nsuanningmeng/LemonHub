package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The in-memory rate limiter is shared across routers, including test fixtures.
var performanceRouterUserID atomic.Int64

func setupModelPerformanceRouter(t *testing.T) (*gin.Engine, *model.User) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMaster, previousPath := common.IsMasterNode, common.SQLitePath
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousLimit, previousSecret := common.GlobalApiRateLimitEnable, common.SessionSecret
	previousGinMode := gin.Mode()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousGroups := setting.UserUsableGroups2JSONString()
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	previousSpecialGroups := specialGroups.ReadAll()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previousNav, hadNav := common.OptionMap["HeaderNavModules"]
	common.OptionMap["HeaderNavModules"] = `{"pricing":{"enabled":true,"requireAuth":false}}`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.IsMasterNode, common.SQLitePath = previousMaster, previousPath
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		common.GlobalApiRateLimitEnable, common.SessionSecret = previousLimit, previousSecret
		gin.SetMode(previousGinMode)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		specialGroups.Clear()
		specialGroups.AddAll(previousSpecialGroups)
		model.InvalidatePricingCache()
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if hadNav {
			common.OptionMap["HeaderNavModules"] = previousNav
		} else {
			delete(common.OptionMap, "HeaderNavModules")
		}
	})

	// InitDB initializes the database-specific column quoting as in production.
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false
	common.SQLitePath = filepath.Join(t.TempDir(), "performance.db")
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.GlobalApiRateLimitEnable = false
	common.SessionSecret = "model-performance-router-test-secret"
	require.NoError(t, model.InitDB())
	db := model.DB
	model.LOG_DB = db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Token{}, &model.Channel{},
		&model.Ability{}, &model.Model{}, &model.Vendor{}, &model.PerfMetric{},
	))
	require.NoError(t, i18n.Init())
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"public":1,"private":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"public":"Public"}`))
	specialGroups.Clear()
	specialGroups.Set("member", map[string]string{"-:public": "", "+:private": "Private"})
	user := &model.User{
		Id:       int(performanceRouterUserID.Add(1)),
		Username: "performance-member", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "member", AuthVersion: 1,
		AffCode: "performance-member-aff",
	}
	user.SetAccessToken("router-performance-pat")
	require.NoError(t, db.Create(user).Error)
	channel := &model.Channel{Name: "performance source", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: "router-performance-model", Group: "public", ChannelId: channel.Id, Enabled: true},
		{Model: "router-performance-model", Group: "private", ChannelId: channel.Id, Enabled: true},
	}).Error)
	require.NoError(t, db.Create(&model.PerfMetric{
		ChannelID: channel.Id, ModelName: "router-performance-model", Group: "private",
		BucketTs: time.Now().Add(-time.Hour).Unix(), RequestCount: 4, SuccessCount: 3,
	}).Error)
	model.InvalidatePricingCache()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	return engine, user
}

func TestModelPerformanceRequiresUserAccessTokenWhenPricingIsPublic(t *testing.T) {
	engine, user := setupModelPerformanceRouter(t)
	disabled := &model.User{
		Username: "performance-disabled", Role: common.RoleCommonUser,
		Status: common.UserStatusDisabled, Group: "member", AuthVersion: 1,
		AffCode: "performance-disabled-aff",
	}
	disabled.SetAccessToken("disabled-performance-pat")
	require.NoError(t, model.DB.Create(disabled).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: "performancerelaykey", Name: "valid relay key",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	for _, tc := range []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "invalid", authorization: "Bearer unknown-performance-pat"},
		{name: "disabled user", authorization: "Bearer disabled-performance-pat"},
		{name: "relay key", authorization: "Bearer sk-performancerelaykey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models", nil)
			request.Header.Set("Authorization", tc.authorization)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
			var body struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			assert.False(t, body.Success)
		})
	}
}

func TestModelPerformanceAcceptsUserAccessTokensAndAppliesGroupPermissions(t *testing.T) {
	engine, user := setupModelPerformanceRouter(t)
	session := &model.UserSession{
		SID: "performance-router-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "performance-router-refresh",
		LoginMethod: "password", LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.CreateUserSession(session))
	accessToken, _, err := service.IssueAccessToken(service.AuthIdentity{
		UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1,
	})
	require.NoError(t, err)

	for _, tc := range []struct {
		name          string
		authorization string
	}{
		{name: "PAT", authorization: "Bearer " + user.GetAccessToken()},
		{name: "legacy PAT", authorization: user.GetAccessToken()},
		{name: "session JWT", authorization: "Bearer " + accessToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models", nil)
			request.Header.Set("Authorization", tc.authorization)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
			assert.Contains(t, response.Header().Get("Cache-Control"), "private")
			var body struct {
				Success bool                     `json:"success"`
				Data    perfmetrics.ModelsResult `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			require.True(t, body.Success, response.Body.String())
			require.Len(t, body.Data.Models, 1)
			assert.Equal(t, "router-performance-model", body.Data.Models[0].ModelName)
			require.Len(t, body.Data.Models[0].Groups, 1)
			group := body.Data.Models[0].Groups[0]
			assert.Equal(t, "private", group.Group, "the authenticated member's permissions replace anonymous visibility")
			assert.True(t, group.HasData)
			require.NotNil(t, group.SuccessRate)
			assert.Equal(t, 75.0, *group.SuccessRate)
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models?group=public", nil)
	request.Header.Set("Authorization", "Bearer "+user.GetAccessToken())
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Success bool                     `json:"success"`
		Data    perfmetrics.ModelsResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Empty(t, body.Data.Models, "query parameters cannot restore a group excluded by account permissions")
}
