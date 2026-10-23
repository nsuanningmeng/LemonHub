package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupPerfMetricIdentityDatabase(t *testing.T, dialect common.DatabaseType, tableOptions string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	if dialect == common.DatabaseTypeMySQL {
		dsn := strings.TrimSpace(os.Getenv("MYSQL_PERF_METRIC_IDENTITY_DSN"))
		if dsn == "" {
			t.Skip("set MYSQL_PERF_METRIC_IDENTITY_DSN to an empty lemonhub_perf_metric_identity_test_* database")
		}
		config, err := mysqldriver.ParseDSN(dsn)
		require.NoError(t, err)
		require.Regexp(t, `^lemonhub_perf_metric_identity_test_[a-z0-9_]+$`, config.DBName)
		config.ParseTime = true
		driver = mysql.Open(config.FormatDSN())
	} else {
		driver = sqlite.Open(filepath.Join(t.TempDir(), "perf_identity.db"))
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	if dialect == common.DatabaseTypeMySQL {
		var tableCount int64
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tableCount).Error)
		require.Zero(t, tableCount, "refusing to modify a nonempty schema")
		require.NoError(t, db.Exec("SET SESSION sql_mode = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES'").Error)
		require.NoError(t, db.Set("gorm:table_options", tableOptions).AutoMigrate(&PerfMetric{}))
	} else {
		require.NoError(t, db.AutoMigrate(&PerfMetric{}))
	}
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&PerfMetric{})) })
	previousDB, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(dialect)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		initCol()
	})
	return db
}

func TestPerfMetricQueriesPreserveModelGroupIdentity(t *testing.T) {
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
			metrics := []PerfMetric{
				{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 3, SuccessCount: 2, TotalLatencyMs: 300, TtftSumMs: 100, TtftCount: 1, OutputTokens: 30, GenerationMs: 1500},
				{ChannelID: 2, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 2, SuccessCount: 2, TotalLatencyMs: 200, TtftSumMs: 200, TtftCount: 2, OutputTokens: 20, GenerationMs: 1000},
				{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1100, RequestCount: 5, SuccessCount: 3, TotalLatencyMs: 500, TtftSumMs: 300, TtftCount: 3, OutputTokens: 50, GenerationMs: 2500},
				// These are distinct permission keys even under an insensitive database collation.
				{ChannelID: 3, ModelName: "model", Group: "PUBLIC", BucketTs: 1000, RequestCount: 20},
				{ChannelID: 4, ModelName: "model", Group: "públic", BucketTs: 1000, RequestCount: 30},
				{ChannelID: 5, ModelName: "model", Group: "public ", BucketTs: 1000, RequestCount: 40},
				{ChannelID: 6, ModelName: "MODEL", Group: "public", BucketTs: 1000, RequestCount: 50},
				{ChannelID: 7, ModelName: "módel", Group: "public", BucketTs: 1000, RequestCount: 60},
				{ChannelID: 8, ModelName: "model ", Group: "public", BucketTs: 1000, RequestCount: 70},
				{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 900, RequestCount: 100},
				{ChannelID: 1, ModelName: "model", Group: "public", BucketTs: 1201, RequestCount: 100},
				{ChannelID: 99, ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 100},
			}
			require.NoError(t, db.Create(&metrics).Error)
			channelIDs := []int{1, 2, 3, 4, 5, 6, 7, 8}
			modelNames := []string{"model", "MODEL", "módel", "model "}
			groups := []string{"public", "PUBLIC", "públic", "public "}
			publicSummary := PerfMetricGroupSummary{ModelName: "model", Group: "public", RequestCount: 10, SuccessCount: 7, TotalLatencyMs: 1000, TtftSumMs: 600, TtftCount: 6, OutputTokens: 100, GenerationMs: 5000}

			bulk, err := GetPerfMetricsByModelGroup(1000, 1200, []string{"model"}, []string{"public"}, channelIDs)
			require.NoError(t, err)
			assert.Equal(t, []PerfMetricGroupSummary{publicSummary}, bulk, "unselected model/group variants must not contribute to accessible totals")
			bulk, err = GetPerfMetricsByModelGroup(1000, 1200, modelNames, groups, channelIDs)
			require.NoError(t, err)
			assert.ElementsMatch(t, []PerfMetricGroupSummary{
				publicSummary,
				{ModelName: "model", Group: "PUBLIC", RequestCount: 20},
				{ModelName: "model", Group: "públic", RequestCount: 30},
				{ModelName: "model", Group: "public ", RequestCount: 40},
				{ModelName: "MODEL", Group: "public", RequestCount: 50},
				{ModelName: "módel", Group: "public", RequestCount: 60},
				{ModelName: "model ", Group: "public", RequestCount: 70},
			}, bulk, "selected model/group variants must remain separate")

			rows, err := GetPerfMetrics("model", "public", 1000, 1200, channelIDs)
			require.NoError(t, err)
			assert.ElementsMatch(t, metrics[:3], rows, "detail filters use exact model and group identities")
			rows, err = GetPerfMetrics("model", "", 1000, 1200, channelIDs)
			require.NoError(t, err)
			assert.ElementsMatch(t, metrics[:6], rows, "an unfiltered group must not broaden the requested model")

			summaries, err := GetPerfMetricsSummaryAll(1000, 1200, []string{"public"}, channelIDs)
			require.NoError(t, err)
			assert.ElementsMatch(t, []PerfMetricSummary{
				{ModelName: "model", RequestCount: 10, SuccessCount: 7, TotalLatencyMs: 1000, OutputTokens: 100, GenerationMs: 5000},
				{ModelName: "MODEL", RequestCount: 50},
				{ModelName: "módel", RequestCount: 60},
				{ModelName: "model ", RequestCount: 70},
			}, summaries, "model-wide summaries preserve model identity and exclude unselected groups")

			publicBuckets := []PerfMetricSummaryBucket{
				{ModelName: "model", Group: "public", BucketTs: 1000, RequestCount: 5, SuccessCount: 4, TotalLatencyMs: 500, OutputTokens: 50, GenerationMs: 2500},
				{ModelName: "model", Group: "public", BucketTs: 1100, RequestCount: 5, SuccessCount: 3, TotalLatencyMs: 500, OutputTokens: 50, GenerationMs: 2500},
				{ModelName: "MODEL", Group: "public", BucketTs: 1000, RequestCount: 50},
				{ModelName: "módel", Group: "public", BucketTs: 1000, RequestCount: 60},
				{ModelName: "model ", Group: "public", BucketTs: 1000, RequestCount: 70},
			}
			buckets, err := GetPerfMetricsSummaryBucketsAll(1000, 1200, []string{"public"}, channelIDs)
			require.NoError(t, err)
			assert.ElementsMatch(t, publicBuckets, buckets, "model-square buckets exclude unselected groups")
			buckets, err = GetPerfMetricsSummaryBucketsAll(1000, 1200, groups, channelIDs)
			require.NoError(t, err)
			assert.ElementsMatch(t, append(publicBuckets,
				PerfMetricSummaryBucket{ModelName: "model", Group: "PUBLIC", BucketTs: 1000, RequestCount: 20},
				PerfMetricSummaryBucket{ModelName: "model", Group: "públic", BucketTs: 1000, RequestCount: 30},
				PerfMetricSummaryBucket{ModelName: "model", Group: "public ", BucketTs: 1000, RequestCount: 40},
			), buckets, "model-square buckets preserve selected group identities")
		})
	}
}
