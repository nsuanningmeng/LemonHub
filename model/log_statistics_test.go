package model

import (
	"github.com/QuantumNous/new-api/common"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumUsedQuotaPreservesQuotaAndSiteScope(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	inclusive, explicitZero := int64(100), int64(0)
	logs := []Log{
		{SiteId: 0, CreatedAt: now - 10, Type: LogTypeConsume, Quota: 100, PromptTokens: 10, InputTokensTotal: &inclusive, CompletionTokens: 5},
		{SiteId: 0, CreatedAt: now - 3600, Type: LogTypeConsume, Quota: 50, PromptTokens: 30, CompletionTokens: 10},
		{SiteId: 7, CreatedAt: now - 10, Type: LogTypeConsume, Quota: 200, PromptTokens: 20, InputTokensTotal: &explicitZero, CompletionTokens: 7},
		{SiteId: 0, CreatedAt: now - 10, Type: LogTypeError, Quota: 900, PromptTokens: 90, CompletionTokens: 90},
	}
	for i := range logs {
		logs[i].Username = "usage-user"
		logs[i].ModelName = "usage-model"
		logs[i].TokenName = "usage-token"
		logs[i].ChannelId = 17
		logs[i].Group = "usage-group"
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	for _, tc := range []struct {
		name   string
		siteID int
		end    int64
		want   Stat
	}{
		{name: "all sites", siteID: SiteScopeAll, want: Stat{Quota: 350, Rpm: 2, Tpm: 112}},
		{name: "main site", siteID: 0, want: Stat{Quota: 150, Rpm: 1, Tpm: 105}},
		{name: "sub-site", siteID: 7, want: Stat{Quota: 200, Rpm: 1, Tpm: 7}},
		{name: "empty site", siteID: 8, want: Stat{}},
		// Historical quota uses the requested time range, while rates use the last minute.
		{name: "historical quota", siteID: 0, end: now - 60, want: Stat{Quota: 50, Rpm: 1, Tpm: 105}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stat, err := SumUsedQuota(LogTypeConsume, now-7200, tc.end, "usage-model", "usage-user", "usage-token", 17, "usage-group", tc.siteID)
			require.NoError(t, err)
			assert.Equal(t, tc.want, stat)
		})
	}
}

func TestNormalizedTokenStatisticsInt64Boundaries(t *testing.T) {
	for _, tt := range []struct {
		name        string
		inputs      []int64
		completions []int
		want        int64
	}{
		{"negative output cannot cancel input", []int64{25}, []int{-3}, 25},
		{"negative normalized input cannot cancel output", []int64{-3}, []int{25}, 25},
		{"valid zero overrides raw input", []int64{0}, []int{0}, 0},
		{"cross sum overflow saturates", []int64{math.MaxInt64}, []int{1}, math.MaxInt64},
		{"input aggregate overflow saturates", []int64{math.MaxInt64, 1}, []int{0, 0}, math.MaxInt64},
		{"output aggregate overflow saturates", []int64{0, 0}, []int{math.MaxInt64, 1}, math.MaxInt64},
		{"exact values above float integer precision", []int64{1 << 53, 1}, []int{1, 0}, (1 << 53) + 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			now := time.Now().Unix()
			for i := range tt.inputs {
				input := tt.inputs[i]
				require.NoError(t, LOG_DB.Create(&Log{UserId: 7290, Type: LogTypeConsume, CreatedAt: now,
					PromptTokens: 123, InputTokensTotal: &input, CompletionTokens: tt.completions[i], Quota: 1}).Error)
			}
			// This huge unrelated row must not leak through the fallback query filters.
			huge := int64(math.MaxInt64)
			require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Type: LogTypeConsume, CreatedAt: now,
				ModelName: "other", InputTokensTotal: &huge}).Error)
			stat, err := SumUserUsedQuota(7290, LogTypeConsume, 0, 0, "", "", 0, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, stat.Tpm)
			assert.Equal(t, len(tt.inputs), stat.Rpm)
			assert.Equal(t, len(tt.inputs), stat.Quota)
			// A distinct token filter verifies both independent SQL aggregation callers.
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ?", 7290).Update("token_name", "statistics").Error)
			assert.Equal(t, tt.want, SumUsedToken(LogTypeConsume, 0, 0, "", "", "statistics"))
		})
	}
}

func TestQuotaDataTokenAccumulationSaturates(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	before := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = before
		CacheQuotaDataLock.Unlock()
	})
	params := QuotaDataLogParams{UserID: 7290, Username: "stats", ModelName: "stats", CreatedAt: time.Now().Unix(), TokenUsed: math.MaxInt64}
	LogQuotaData(params)
	params.TokenUsed = 1
	LogQuotaData(params)
	CacheQuotaDataLock.Lock()
	for _, bucket := range CacheQuotaData {
		assert.Equal(t, int64(math.MaxInt64), bucket.TokenUsed)
	}
	CacheQuotaDataLock.Unlock()
	SaveQuotaDataCache()
	LogQuotaData(params)
	SaveQuotaDataCache()
	var bucket QuotaData
	require.NoError(t, DB.Where("user_id = ?", params.UserID).First(&bucket).Error)
	assert.Equal(t, int64(math.MaxInt64), bucket.TokenUsed)
	assert.Equal(t, 3, bucket.Count)
	assert.Equal(t, int64(math.MaxInt64), common.SumTokenCountsForStatistics(bucket.TokenUsed, 1))
}

func TestSQLiteTokenOverflowFallbackAllowsLegacyNullCounts(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	huge := int64(math.MaxInt64)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 7290, Type: LogTypeConsume, CreatedAt: now, InputTokensTotal: &huge}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 7290, Type: LogTypeConsume, CreatedAt: now, PromptTokens: 1}).Error)
	require.NoError(t, LOG_DB.Exec("INSERT INTO logs (user_id, type, created_at, prompt_tokens, completion_tokens) VALUES (?, ?, ?, NULL, NULL)", 7290, LogTypeConsume, now).Error)
	stat, err := SumUserUsedQuota(7290, LogTypeConsume, 0, 0, "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, int64(math.MaxInt64), stat.Tpm)
	assert.Equal(t, 3, stat.Rpm)
}
