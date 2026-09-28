package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelPerfMigrationPreservesUnattributedLegacyHistory(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "perf.db")), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = originalDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	// Old installations store already-merged model/group totals with no channel
	// identity. Migration must preserve them, but cannot use them in scoped queries.
	type legacyMetric struct {
		ID           int    `gorm:"primaryKey"`
		ModelName    string `gorm:"size:128;uniqueIndex:idx_perf_model_group_bucket,priority:1"`
		Group        string `gorm:"column:group;size:64;uniqueIndex:idx_perf_model_group_bucket,priority:2"`
		BucketTs     int64  `gorm:"uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"`
		RequestCount int64
		SuccessCount int64
	}
	require.NoError(t, db.Table("perf_metrics").AutoMigrate(&legacyMetric{}))
	legacy := legacyMetric{ModelName: "gemini", Group: "Gemini", BucketTs: 1000, RequestCount: 100, SuccessCount: 3}
	require.NoError(t, db.Table("perf_metrics").Create(&legacy).Error)
	require.NoError(t, db.AutoMigrate(&PerfMetric{}))
	// Repeated startup migrations also leave the old table unchanged.
	require.NoError(t, db.AutoMigrate(&PerfMetric{}))
	var preserved legacyMetric
	require.NoError(t, db.Table("perf_metrics").First(&preserved).Error)
	assert.Equal(t, legacy, preserved)
	rows, err := GetPerfMetrics("gemini", "Gemini", 0, 2000, []int{1})
	require.NoError(t, err)
	assert.Empty(t, rows)

	for _, metric := range []PerfMetric{
		{ChannelID: 1, ModelName: "gemini", Group: "Gemini", BucketTs: 1000, RequestCount: 1, SuccessCount: 1, OutputTokens: 10, GenerationMs: 500},
		{ChannelID: 2, ModelName: "gemini", Group: "Gemini", BucketTs: 1000, RequestCount: 1},
		{ChannelID: 1, ModelName: "gemini", Group: "Gemini", BucketTs: 1000, RequestCount: 2, SuccessCount: 1, OutputTokens: 20, GenerationMs: 1000},
		{ModelName: "gemini", Group: "Gemini", BucketTs: 1000, RequestCount: 500},
	} {
		require.NoError(t, UpsertPerfMetric(&metric))
	}
	require.NoError(t, db.Order("channel_id").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, 1, rows[0].ChannelID)
	assert.Equal(t, int64(3), rows[0].RequestCount)
	assert.Equal(t, int64(2), rows[0].SuccessCount)
	assert.Equal(t, int64(30), rows[0].OutputTokens)
	assert.Equal(t, int64(1500), rows[0].GenerationMs)
	assert.Equal(t, 2, rows[1].ChannelID)
	assert.Equal(t, int64(1), rows[1].RequestCount)
	assert.Zero(t, rows[1].SuccessCount)
	// Restricting the snapshot to channel 1 excludes channel 2's failed history.
	rows, err = GetPerfMetrics("gemini", "Gemini", 0, 2000, []int{1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 1, rows[0].ChannelID)

	require.NoError(t, DeletePerfMetricsBefore(2000))
	require.NoError(t, db.Table("perf_metrics").First(&preserved).Error)
	assert.Equal(t, legacy, preserved)
}
