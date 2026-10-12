package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetUserUsageActivityResponseAndAccountIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = previous
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	created := time.Date(2026, 1, 1, 16, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 1, Username: "old-name", SiteId: 1, Type: model.LogTypeConsume, CreatedAt: created.Unix(), PromptTokens: 7, CompletionTokens: 3, UseTime: 12},
		{UserId: 2, Username: "shared-name", SiteId: 2, Type: model.LogTypeConsume, CreatedAt: created.Unix(), PromptTokens: 999, CompletionTokens: 999, UseTime: 999},
	}).Error)

	for _, tc := range []struct {
		name, query, zone, date string
		userID                  int
		tokens                  int64
	}{
		{name: "authenticated account ignores query identity", query: "?timezone=Asia%2FShanghai&user_id=2&username=shared-name&site_id=2", zone: "Asia/Shanghai", date: "2026-01-02", userID: 1, tokens: 10},
		{name: "default UTC", zone: "UTC", date: "2026-01-01", userID: 1, tokens: 10},
		{name: "empty account", zone: "UTC", userID: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/activity"+tc.query, nil)
			ctx.Set("id", tc.userID)
			ctx.Set("username", "shared-name")
			GetUserUsageActivity(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool                `json:"success"`
				Data    model.UsageActivity `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			assert.Equal(t, tc.zone, response.Data.Timezone)
			assert.Equal(t, tc.tokens, response.Data.Summary.TotalTokens)
			asOf, err := time.Parse(time.RFC3339, response.Data.AsOf)
			require.NoError(t, err)
			location, err := time.LoadLocation(tc.zone)
			require.NoError(t, err)
			assert.Equal(t, asOf.In(location).Format(time.RFC3339), response.Data.AsOf)
			if tc.tokens == 0 {
				assert.Contains(t, recorder.Body.String(), `"daily_usage":[]`)
				assert.Equal(t, model.UsageActivitySummary{}, response.Data.Summary)
			} else {
				assert.Equal(t, []model.UsageActivityDay{{Date: tc.date, Tokens: tc.tokens}}, response.Data.DailyUsage)
				assert.Equal(t, int64(12), response.Data.Summary.LongestRequestSeconds)
			}
		})
	}

	// Database errors must not appear as an empty successful response or expose SQL.
	require.NoError(t, db.Migrator().DropTable(&model.Log{}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/activity", nil)
	ctx.Set("id", 1)
	GetUserUsageActivity(ctx)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, `{"success":false,"message":"Unable to load usage activity"}`, recorder.Body.String())
}

func TestGetUserUsageActivityRejectsInvalidIdentityAndTimezone(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		userID      int
		status      int
	}{
		{name: "missing identity", query: "?user_id=1", status: http.StatusUnauthorized},
		{name: "negative identity", userID: -1, status: http.StatusUnauthorized},
		{name: "unknown zone", userID: 1, query: "?timezone=Invalid%2FZone", status: http.StatusBadRequest},
		{name: "empty zone", userID: 1, query: "?timezone=", status: http.StatusBadRequest},
		{name: "server local zone", userID: 1, query: "?timezone=Local", status: http.StatusBadRequest},
		{name: "path traversal", userID: 1, query: "?timezone=..%2FUTC", status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/activity"+tc.query, nil)
			ctx.Set("id", tc.userID)
			GetUserUsageActivity(ctx)
			assert.Equal(t, tc.status, recorder.Code)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.NotContains(t, recorder.Body.String(), `"data"`)
		})
	}
}
