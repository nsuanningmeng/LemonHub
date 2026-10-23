package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Keep the pre-identity schema independent of PerfMetric so this exercises a
// real upgrade with existing counters and the original unique constraint.
type legacyChannelPerfMetric struct {
	Id             int    `gorm:"primaryKey"`
	ChannelID      int    `gorm:"uniqueIndex:idx_channel_perf_bucket,priority:1"`
	ModelName      string `gorm:"size:128;uniqueIndex:idx_channel_perf_bucket,priority:2"`
	Group          string `gorm:"column:group;size:64;uniqueIndex:idx_channel_perf_bucket,priority:3"`
	BucketTs       int64  `gorm:"uniqueIndex:idx_channel_perf_bucket,priority:4;index:idx_channel_perf_bucket_ts"`
	RequestCount   int64  `gorm:"default:0"`
	SuccessCount   int64  `gorm:"default:0"`
	TotalLatencyMs int64  `gorm:"default:0"`
	TtftSumMs      int64  `gorm:"default:0"`
	TtftCount      int64  `gorm:"default:0"`
	OutputTokens   int64  `gorm:"default:0"`
	GenerationMs   int64  `gorm:"default:0"`
}

func (legacyChannelPerfMetric) TableName() string { return "channel_perf_metrics" }

func TestPerfMetricIdentityMigrationPreservesHistory(t *testing.T) {
	for _, tc := range []struct {
		name         string
		dialect      common.DatabaseType
		tableOptions string
	}{
		{name: "sqlite", dialect: common.DatabaseTypeSQLite},
		{name: "mysql_utf8mb4", dialect: common.DatabaseTypeMySQL, tableOptions: "CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"},
		{name: "mysql_gbk", dialect: common.DatabaseTypeMySQL, tableOptions: "CHARACTER SET gbk COLLATE gbk_chinese_ci"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupPerfMetricIdentityDatabase(t, tc.dialect, tc.tableOptions)
			require.NoError(t, db.Migrator().DropTable(&PerfMetric{}))
			require.NoError(t, db.Set("gorm:table_options", tc.tableOptions).AutoMigrate(&legacyChannelPerfMetric{}))
			original := []legacyChannelPerfMetric{
				{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 100, TtftSumMs: 25, TtftCount: 1, OutputTokens: 10, GenerationMs: 75},
				{ChannelID: 2, ModelName: "módel", Group: "públic", BucketTs: 1000, RequestCount: 7, SuccessCount: 5},
			}
			require.NoError(t, db.Create(&original).Error)
			// Simulate a restart after part of the nullable column was backfilled.
			require.NoError(t, db.Migrator().AddColumn(&PerfMetric{}, "IdentityHash"))
			require.NoError(t, db.Migrator().CreateIndex(&PerfMetric{}, "idx_channel_perf_identity_bucket"))
			partialHash, err := perfMetricIdentityHash(original[1].ModelName, original[1].Group)
			require.NoError(t, err)
			require.NoError(t, db.Model(&PerfMetric{}).Where("id = ?", original[1].Id).Update("identity_hash", partialHash).Error)
			require.NoError(t, migratePerfMetricIdentities(db))
			require.NoError(t, db.AutoMigrate(&PerfMetric{}))
			var preserved []legacyChannelPerfMetric
			require.NoError(t, db.Order("id").Find(&preserved).Error)
			assert.Equal(t, original, preserved, "backfill must not rewrite historical names, IDs, or counters")

			// Case, accent, and trailing-space differences on the SAME channel and
			// bucket must remain independent even with MySQL's default collation.
			for _, metric := range []PerfMetric{
				{ChannelID: 1, ModelName: "model", Group: "public", IdentityHash: "untrusted", BucketTs: 1000, RequestCount: 2, SuccessCount: 2},
				{ChannelID: 1, ModelName: "model", Group: "PUBLIC", BucketTs: 1000, RequestCount: 9},
				{ChannelID: 1, ModelName: "MODEL", Group: "public", BucketTs: 1000, RequestCount: 11},
				{ChannelID: 1, ModelName: "módel", Group: "public", BucketTs: 1000, RequestCount: 13},
				{ChannelID: 1, ModelName: "model", Group: "públic", BucketTs: 1000, RequestCount: 15},
				{ChannelID: 1, ModelName: "model ", Group: "public", BucketTs: 1000, RequestCount: 17},
				{ChannelID: 1, ModelName: "model", Group: "public ", BucketTs: 1000, RequestCount: 19},
			} {
				require.NoError(t, UpsertPerfMetric(&metric))
			}
			// Normal startup must neither recreate the old constraint nor alter data.
			require.NoError(t, migratePerfMetricIdentities(db))
			require.NoError(t, db.AutoMigrate(&PerfMetric{}))
			var exactRows []PerfMetric
			require.NoError(t, db.Where("channel_id = ?", 1).Find(&exactRows).Error)
			assert.Len(t, exactRows, 7, "same-identity writes must update the backfilled historical row")
			buckets, err := GetPerfMetricsByModelGroup(1000, 1000,
				[]string{"model", "MODEL", "módel", "model "}, []string{"public", "PUBLIC", "públic", "public "}, []int{1})
			require.NoError(t, err)
			assert.ElementsMatch(t, []PerfMetricGroupSummary{
				{ModelName: "model", Group: "public", RequestCount: 3, SuccessCount: 3, TotalLatencyMs: 100, TtftSumMs: 25, TtftCount: 1, OutputTokens: 10, GenerationMs: 75},
				{ModelName: "model", Group: "PUBLIC", RequestCount: 9},
				{ModelName: "MODEL", Group: "public", RequestCount: 11},
				{ModelName: "módel", Group: "public", RequestCount: 13},
				{ModelName: "model", Group: "públic", RequestCount: 15},
				{ModelName: "model ", Group: "public", RequestCount: 17},
				{ModelName: "model", Group: "public ", RequestCount: 19},
			}, buckets)

			// A MySQL node on the old binary can append without the new nullable
			// column. Preserve that row independently on later startups, without
			// forcing its hash to collide with the current writer's aggregate.
			lateLegacy := legacyChannelPerfMetric{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 4}
			require.NoError(t, db.Create(&lateLegacy).Error)
			require.NoError(t, migratePerfMetricIdentities(db))
			require.NoError(t, db.AutoMigrate(&PerfMetric{}))
			var latePreserved legacyChannelPerfMetric
			require.NoError(t, db.First(&latePreserved, lateLegacy.Id).Error)
			assert.Equal(t, lateLegacy, latePreserved)
			require.NoError(t, UpsertPerfMetric(&PerfMetric{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 2, SuccessCount: 2}))
			buckets, err = GetPerfMetricsByModelGroup(1000, 1000, []string{"model"}, []string{"public"}, []int{1})
			require.NoError(t, err)
			require.Len(t, buckets, 1)
			assert.Equal(t, int64(9), buckets[0].RequestCount)
			assert.Equal(t, int64(5), buckets[0].SuccessCount)
		})
	}
}

func TestPerfMetricIdentityMigrationRetainsConstraintOnFailure(t *testing.T) {
	db := setupPerfMetricIdentityDatabase(t, common.DatabaseTypeSQLite, "")
	require.NoError(t, db.Migrator().DropTable(&PerfMetric{}))
	require.NoError(t, db.AutoMigrate(&legacyChannelPerfMetric{}))
	original := legacyChannelPerfMetric{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 9, SuccessCount: 5}
	require.NoError(t, db.Create(&original).Error)
	failure := errors.New("injected backfill failure")
	const callback = "test:reject_perf_identity_backfill"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	require.ErrorIs(t, migratePerfMetricIdentities(db), failure)
	assert.True(t, db.Migrator().HasIndex(&PerfMetric{}, "idx_channel_perf_bucket"), "a failed backfill must retain the old uniqueness constraint")
	var preserved legacyChannelPerfMetric
	require.NoError(t, db.First(&preserved, original.Id).Error)
	assert.Equal(t, original, preserved)
	require.NoError(t, db.Callback().Update().Remove(callback))
	require.NoError(t, migratePerfMetricIdentities(db))
	require.NoError(t, db.AutoMigrate(&PerfMetric{}))
	require.NoError(t, UpsertPerfMetric(&PerfMetric{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 1, SuccessCount: 1}))
	rows, err := GetPerfMetricsByModelGroup(1000, 1000, []string{"model"}, []string{"public"}, []int{1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(10), rows[0].RequestCount)
	assert.Equal(t, int64(6), rows[0].SuccessCount)
}
