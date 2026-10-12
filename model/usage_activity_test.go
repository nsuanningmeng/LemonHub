package model

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUsageActivityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := LOG_DB
	LOG_DB = db
	t.Cleanup(func() {
		LOG_DB = previous
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Log{}))
	return db
}

func TestGetUserUsageActivityRetainedLogsAndNormalizedTokens(t *testing.T) {
	db := setupUsageActivityTestDB(t)
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	inclusive, zero := int64(100), int64(0)
	logs := []Log{
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume, PromptTokens: 5, InputTokensTotal: &inclusive, CompletionTokens: 7, UseTime: 77},
		{UserId: 1, CreatedAt: time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC).Unix(), Type: LogTypeConsume, PromptTokens: 20, CompletionTokens: 3},
		{UserId: 1, CreatedAt: time.Date(2026, 10, 10, 15, 59, 59, 0, time.UTC).Unix(), Type: LogTypeConsume, PromptTokens: 10, CompletionTokens: 2},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume, PromptTokens: 999, InputTokensTotal: &zero, CompletionTokens: 8},
		{UserId: 1, CreatedAt: time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC).Unix(), Type: LogTypeConsume, PromptTokens: 2},
		{UserId: 1, CreatedAt: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC).Unix(), Type: LogTypeConsume, PromptTokens: 3},
		// A successful zero-token request can still be the longest request.
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume, UseTime: 180},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume, UseTime: 999, Other: `{"violation_fee":true}`},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeConsume, PromptTokens: 4, UseTime: 999, Other: `{"stream_status":{"status":"error"}}`},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeError, PromptTokens: 1000, UseTime: 999},
		{UserId: 1, CreatedAt: now.Unix(), Type: LogTypeTopup, PromptTokens: 1000, UseTime: 999},
		{UserId: 2, CreatedAt: now.Unix(), Type: LogTypeConsume, PromptTokens: 1000, UseTime: 999},
		{UserId: 1, CreatedAt: now.Unix() + 1, Type: LogTypeConsume, PromptTokens: 1000, UseTime: 999},
	}
	require.NoError(t, db.Create(&logs).Error)
	previousExport, previousLog := common.DataExportEnabled, common.LogConsumeEnabled
	common.DataExportEnabled, common.LogConsumeEnabled = false, false
	t.Cleanup(func() { common.DataExportEnabled, common.LogConsumeEnabled = previousExport, previousLog })

	activity, err := GetUserUsageActivity(context.Background(), 1, location, now)
	require.NoError(t, err)
	assert.Equal(t, "Asia/Shanghai", activity.Timezone)
	assert.Equal(t, "2026-10-12T18:00:00+08:00", activity.AsOf)
	assert.Equal(t, UsageActivitySummary{TotalTokens: 159, PeakDailyTokens: 119, CurrentStreakDays: 3, LongestStreakDays: 3, LongestRequestSeconds: 180}, activity.Summary)
	assert.Equal(t, []UsageActivityDay{
		{Date: "2025-12-30", Tokens: 2},
		{Date: "2025-12-31", Tokens: 3},
		{Date: "2026-10-10", Tokens: 12},
		{Date: "2026-10-11", Tokens: 23},
		{Date: "2026-10-12", Tokens: 119},
	}, activity.DailyUsage)
}

func TestGetUserUsageActivityCivilDateStreaks(t *testing.T) {
	for _, tc := range []struct {
		name, zone, now string
		timestamps      []string
		current         int
		longest         int
		dates           []string
	}{
		{
			name: "spring DST today", zone: "America/New_York", now: "2026-03-09T23:00:00-04:00",
			timestamps: []string{"2026-03-07T12:00:00-05:00", "2026-03-08T12:00:00-04:00", "2026-03-09T12:00:00-04:00"},
			current:    3, longest: 3, dates: []string{"2026-03-07", "2026-03-08", "2026-03-09"},
		},
		{
			name: "fall DST yesterday", zone: "America/New_York", now: "2026-11-03T10:00:00-05:00",
			timestamps: []string{"2026-10-31T12:00:00-04:00", "2026-11-01T01:30:00-04:00", "2026-11-01T01:30:00-05:00", "2026-11-02T12:00:00-05:00"},
			current:    3, longest: 3, dates: []string{"2026-10-31", "2026-11-01", "2026-11-02"},
		},
		{
			name: "ended streak", zone: "UTC", now: "2026-10-12T23:00:00Z",
			timestamps: []string{"2026-10-08T12:00:00Z", "2026-10-09T12:00:00Z", "2026-10-10T12:00:00Z"},
			current:    0, longest: 3, dates: []string{"2026-10-08", "2026-10-09", "2026-10-10"},
		},
		{
			name: "gap resets current", zone: "UTC", now: "2026-01-03T23:00:00Z",
			timestamps: []string{"2025-12-30T12:00:00Z", "2025-12-31T12:00:00Z", "2026-01-01T12:00:00Z", "2026-01-03T12:00:00Z"},
			current:    1, longest: 3, dates: []string{"2025-12-30", "2025-12-31", "2026-01-01", "2026-01-03"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageActivityTestDB(t)
			location, err := time.LoadLocation(tc.zone)
			require.NoError(t, err)
			now, err := time.Parse(time.RFC3339, tc.now)
			require.NoError(t, err)
			for _, timestamp := range tc.timestamps {
				created, err := time.Parse(time.RFC3339, timestamp)
				require.NoError(t, err)
				require.NoError(t, db.Create(&Log{UserId: 1, Type: LogTypeConsume, CreatedAt: created.Unix(), PromptTokens: 1}).Error)
			}
			activity, err := GetUserUsageActivity(context.Background(), 1, location, now)
			require.NoError(t, err)
			assert.Equal(t, tc.current, activity.Summary.CurrentStreakDays)
			assert.Equal(t, tc.longest, activity.Summary.LongestStreakDays)
			var dates []string
			for _, day := range activity.DailyUsage {
				dates = append(dates, day.Date)
			}
			assert.Equal(t, tc.dates, dates)
			assert.Equal(t, int64(len(tc.timestamps)), activity.Summary.TotalTokens)
		})
	}
}

func TestGetUserUsageActivityInvalidCountsAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		inputs     []int64
		outputs    []int
		wantTokens int64
	}{
		{name: "negative counts never subtract", inputs: []int64{-20, 7}, outputs: []int{3, -40}, wantTokens: 10},
		{name: "individual addition saturates", inputs: []int64{math.MaxInt64}, outputs: []int{1}, wantTokens: math.MaxInt64},
		{name: "daily and total addition saturates", inputs: []int64{math.MaxInt64, 1}, outputs: []int{0, 0}, wantTokens: math.MaxInt64},
		{name: "invalid counts do not create active day", inputs: []int64{-1}, outputs: []int{-1}, wantTokens: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageActivityTestDB(t)
			now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
			for i := range tc.inputs {
				require.NoError(t, db.Create(&Log{UserId: 1, Type: LogTypeConsume, CreatedAt: now.Unix(), InputTokensTotal: &tc.inputs[i], CompletionTokens: tc.outputs[i], UseTime: -10}).Error)
			}
			// Legacy nullable token/duration columns must remain readable.
			require.NoError(t, db.Exec("INSERT INTO logs (user_id, type, created_at, prompt_tokens, completion_tokens, use_time) VALUES (?, ?, ?, NULL, NULL, NULL)", 1, LogTypeConsume, now.Unix()).Error)
			activity, err := GetUserUsageActivity(context.Background(), 1, nil, now)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTokens, activity.Summary.TotalTokens)
			assert.Equal(t, tc.wantTokens, activity.Summary.PeakDailyTokens)
			assert.Zero(t, activity.Summary.LongestRequestSeconds)
			if tc.wantTokens == 0 {
				assert.Empty(t, activity.DailyUsage)
				assert.Zero(t, activity.Summary.CurrentStreakDays)
				assert.Zero(t, activity.Summary.LongestStreakDays)
			} else {
				assert.Equal(t, []UsageActivityDay{{Date: "2026-10-12", Tokens: tc.wantTokens}}, activity.DailyUsage)
			}
		})
	}
}

func TestGetUserUsageActivityEmptyAndFailures(t *testing.T) {
	db := setupUsageActivityTestDB(t)
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	activity, err := GetUserUsageActivity(context.Background(), 1, nil, now)
	require.NoError(t, err)
	assert.Equal(t, "UTC", activity.Timezone)
	assert.Equal(t, UsageActivitySummary{}, activity.Summary)
	assert.NotNil(t, activity.DailyUsage)
	assert.Empty(t, activity.DailyUsage)
	for _, userID := range []int{0, -1} {
		activity, err = GetUserUsageActivity(context.Background(), userID, nil, now)
		require.Error(t, err)
		assert.Nil(t, activity)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	activity, err = GetUserUsageActivity(ctx, 1, nil, now)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, activity)
	require.NoError(t, db.Migrator().DropTable(&Log{}))
	activity, err = GetUserUsageActivity(context.Background(), 1, nil, now)
	require.Error(t, err)
	assert.Nil(t, activity)
}
