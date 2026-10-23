package model

import (
	"math"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaDataCacheTokensPersistAndAggregate(t *testing.T) {
	truncateTables(t)
	resetQuotaDataDurabilityState(t)
	params := QuotaDataLogParams{UserID: 8203, Username: "cache-user", ModelName: "cache-model", CreatedAt: 3600,
		TokenUsed: 100, CacheReadTokens: 30, CacheCreationTokens: 20}
	LogQuotaData(params)
	LogQuotaData(params)
	require.NoError(t, SaveQuotaDataCache())
	params.ChannelID = 17
	LogQuotaData(params)
	require.NoError(t, SaveQuotaDataCache())
	for name, query := range map[string]func() ([]*QuotaData, error){
		"user id":      func() ([]*QuotaData, error) { return GetQuotaDataByUserId(params.UserID, 0, 7200) },
		"username":     func() ([]*QuotaData, error) { return GetQuotaDataByUsername(params.Username, 0, 7200) },
		"grouped user": func() ([]*QuotaData, error) { return GetQuotaDataGroupByUser(0, 7200) },
		"all":          func() ([]*QuotaData, error) { return GetAllQuotaDates(0, 7200, "") },
	} {
		t.Run(name, func(t *testing.T) {
			rows, err := query()
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, int64(300), rows[0].TokenUsed, "cache counts are a breakdown, not extra total tokens")
			assert.Equal(t, int64(90), rows[0].CacheReadTokens)
			assert.Equal(t, int64(60), rows[0].CacheCreationTokens)
		})
	}
}

func TestQuotaDataCacheTokensClampInvalidCountsAndSaturate(t *testing.T) {
	truncateTables(t)
	resetQuotaDataDurabilityState(t)
	params := QuotaDataLogParams{UserID: 8203, Username: "cache-user", CreatedAt: 3600,
		CacheReadTokens: -1, CacheCreationTokens: -2}
	LogQuotaData(params)
	params.CacheReadTokens, params.CacheCreationTokens = math.MaxInt64, math.MaxInt64
	LogQuotaData(params)
	params.CacheReadTokens, params.CacheCreationTokens = 1, 2
	LogQuotaData(params)
	require.NoError(t, SaveQuotaDataCache())
	LogQuotaData(params)
	require.NoError(t, SaveQuotaDataCache())
	var row QuotaData
	require.NoError(t, DB.Where("user_id = ?", params.UserID).First(&row).Error)
	assert.Equal(t, int64(math.MaxInt64), row.CacheReadTokens)
	assert.Equal(t, int64(math.MaxInt64), row.CacheCreationTokens)
}

func TestRecordConsumeLogExportsNormalizedCacheTotalOnce(t *testing.T) {
	truncateTables(t)
	resetQuotaDataDurabilityState(t)
	previousExport, previousConsume := common.DataExportEnabled, common.LogConsumeEnabled
	common.DataExportEnabled, common.LogConsumeEnabled = true, true
	t.Cleanup(func() { common.DataExportEnabled, common.LogConsumeEnabled = previousExport, previousConsume })
	user := User{Username: "cache-user", Password: "test", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&user).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Set("username", user.Username)
	input := int64(100)
	RecordConsumeLog(c, user.Id, RecordConsumeLogParams{
		ModelName: "cache-model", PromptTokens: 50, InputTokensTotal: &input, CompletionTokens: 10,
		Other: map[string]interface{}{"cache_tokens": 30, "cache_write_tokens": 20,
			"cache_creation_tokens_5m": 5, "cache_creation_tokens_1h": 15},
	})
	require.NoError(t, SaveQuotaDataCache())
	var row QuotaData
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&row).Error)
	assert.Equal(t, int64(110), row.TokenUsed)
	assert.Equal(t, int64(30), row.CacheReadTokens)
	assert.Equal(t, int64(20), row.CacheCreationTokens)
}
