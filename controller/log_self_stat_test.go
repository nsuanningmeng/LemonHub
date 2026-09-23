package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetLogsSelfStatRestrictsBothAggregatesToAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalLogDB := model.LOG_DB
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	for _, username := range []string{"shared-name", "ad%"} {
		t.Run(username, func(t *testing.T) {
			require.NoError(t, db.Where("1 = 1").Delete(&model.Log{}).Error)
			now := common.GetTimestamp()
			logs := []model.Log{
				{UserId: 1, SiteId: 1, Username: username, CreatedAt: now, Type: model.LogTypeConsume, Quota: 11, PromptTokens: 2, CompletionTokens: 3},
				// Historical names must remain visible to the same account.
				{UserId: 1, SiteId: 1, Username: "old-name", CreatedAt: now, Type: model.LogTypeConsume, Quota: 7, PromptTokens: 5, CompletionTokens: 7},
				{UserId: 2, SiteId: 2, Username: username, CreatedAt: now, Type: model.LogTypeConsume, Quota: 100, PromptTokens: 100, CompletionTokens: 200},
				{UserId: 3, SiteId: 1, Username: "admin", CreatedAt: now, Type: model.LogTypeConsume, Quota: 1000, PromptTokens: 1000, CompletionTokens: 2000},
			}
			require.NoError(t, db.Create(&logs).Error)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/stat?username=admin&user_id=2&site_id=2", nil)
			ctx.Set("id", 1)
			ctx.Set("username", username)
			GetLogsSelfStat(ctx)

			var response struct {
				Success bool       `json:"success"`
				Data    model.Stat `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			assert.Equal(t, model.Stat{Quota: 18, Rpm: 2, Tpm: 17}, response.Data)
		})
	}
}

func TestSumUserUsedQuotaRejectsMissingIdentity(t *testing.T) {
	for _, userID := range []int{0, -1} {
		_, err := model.SumUserUsedQuota(userID, model.LogTypeConsume, 0, 0, "", "", 0, "")
		require.Error(t, err)
	}
}
