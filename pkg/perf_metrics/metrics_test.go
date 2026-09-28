package perfmetrics

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPerfSummaryTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	originalSQLitePath, originalMaster := common.SQLitePath, common.IsMasterNode
	originalRedisEnabled, originalRDB := common.RedisEnabled, common.RDB
	originalPerfEnabled := perf_metrics_setting.GetSetting().Enabled
	originalHot := make(map[any]any)
	hotBuckets.Range(func(key, value any) bool {
		originalHot[key] = value
		return true
	})
	hotBuckets.Clear()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.SQLitePath, common.IsMasterNode = originalSQLitePath, originalMaster
		common.RedisEnabled, common.RDB = originalRedisEnabled, originalRDB
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"perf_metrics_setting.enabled": strconv.FormatBool(originalPerfEnabled),
		}))
		hotBuckets.Clear()
		for key, value := range originalHot {
			hotBuckets.Store(key, value)
		}
	})
	t.Setenv("SQL_DSN", "local")
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	common.IsMasterNode = false
	common.RedisEnabled = false
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.enabled": "true"}))
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}, &model.Channel{}))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Status: common.ChannelStatusEnabled}).Error)
	return db
}

func TestQueryMetricsReflectManualDisableRestoreAndDeletion(t *testing.T) {
	db := setupPerfSummaryTest(t)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
	}).Error)
	bucket := bucketStart(time.Now().Add(-time.Hour).Unix())
	for _, channelID := range []int{1, 2, 3} {
		successes := int64(0)
		if channelID == 1 {
			successes = 1
		}
		require.NoError(t, model.UpsertPerfMetric(&model.PerfMetric{
			ChannelID: channelID, ModelName: "gemini", Group: "Gemini", BucketTs: bucket,
			RequestCount: 1, SuccessCount: successes,
		}))
		hot := &atomicBucket{}
		hot.add(Sample{Success: channelID == 1})
		hotBuckets.Store(bucketKey{channelID: channelID, model: "gemini", group: "Gemini", bucketTs: bucket}, hot)
	}

	for _, step := range []struct {
		name      string
		channelID int
		status    int
		delete    bool
		wantCount int64
		wantRate  float64
	}{
		{name: "enabled and auto-disabled histories included", wantCount: 6, wantRate: 100.0 / 3},
		{name: "manual disable removes database and hot history", channelID: 3, status: common.ChannelStatusManuallyDisabled, wantCount: 4, wantRate: 50},
		{name: "restore includes its retained history", channelID: 3, status: common.ChannelStatusEnabled, wantCount: 6, wantRate: 100.0 / 3},
		{name: "delete removes its history", channelID: 3, delete: true, wantCount: 4, wantRate: 50},
		{name: "auto-disabled failures remain", channelID: 1, status: common.ChannelStatusManuallyDisabled, wantCount: 2},
		{name: "no eligible channel", channelID: 2, delete: true},
	} {
		t.Run(step.name, func(t *testing.T) {
			if step.delete {
				require.NoError(t, db.Delete(&model.Channel{}, step.channelID).Error)
			} else if step.channelID != 0 {
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", step.channelID).Update("status", step.status).Error)
			}
			detail, err := Query(QueryParams{Model: "gemini", Group: "Gemini", Hours: 24})
			require.NoError(t, err)
			summary, err := QuerySummaryAll(24, map[string][]string{"gemini": {"Gemini"}})
			require.NoError(t, err)
			if step.wantCount == 0 {
				assert.Empty(t, detail.Groups)
				assert.Empty(t, summary.Models)
				return
			}
			require.Len(t, detail.Groups, 1)
			require.Len(t, detail.Groups[0].Series, 1)
			assert.InDelta(t, step.wantRate, detail.Groups[0].SuccessRate, 0.0001)
			assert.InDelta(t, step.wantRate, detail.Groups[0].Series[0].SuccessRate, 0.0001)
			require.Len(t, summary.Models, 1)
			assert.Equal(t, step.wantCount, summary.Models[0].RequestCount)
			assert.InDelta(t, step.wantRate, summary.Models[0].SuccessRate, 0.005)
		})
	}
}

func TestQueryMetricsRejectsUnavailableChannelState(t *testing.T) {
	db := setupPerfSummaryTest(t)
	Record(Sample{ChannelID: 1, Model: "gemini", Group: "Gemini", Success: true})
	require.NoError(t, db.Migrator().DropTable(&model.Channel{}))
	detail, err := Query(QueryParams{Model: "gemini"})
	require.Error(t, err)
	assert.Empty(t, detail.Groups)
	summary, err := QuerySummaryAll(24, map[string][]string{"gemini": {"Gemini"}})
	require.Error(t, err)
	assert.Empty(t, summary.Models)
}

func TestRecordAndFlushKeepChannelBucketsSeparate(t *testing.T) {
	db := setupPerfSummaryTest(t)
	redisServer := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() { require.NoError(t, common.RDB.Close()) })
	Record(Sample{Model: "gemini", Group: "Gemini", Success: false})
	Record(Sample{ChannelID: 1, Model: "gemini", Group: "Gemini", Success: true})
	Record(Sample{ChannelID: 2, Model: "gemini", Group: "Gemini", Success: false})
	assert.Len(t, redisServer.Keys(), 2)
	// Move the actual recorded buckets to an explicitly completed interval.
	completedBucket := bucketStart(time.Now().Add(-time.Hour).Unix())
	recorded := make(map[bucketKey]any)
	hotBuckets.Range(func(key, value any) bool {
		recorded[key.(bucketKey)] = value
		return true
	})
	hotBuckets.Clear()
	for k, value := range recorded {
		k.bucketTs = completedBucket
		hotBuckets.Store(k, value)
	}
	flushCompletedBuckets()
	var rows []model.PerfMetric
	require.NoError(t, db.Order("channel_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, 1, rows[0].ChannelID)
	assert.Equal(t, int64(1), rows[0].RequestCount)
	assert.Equal(t, int64(1), rows[0].SuccessCount)
	assert.Equal(t, 2, rows[1].ChannelID)
	assert.Equal(t, int64(1), rows[1].RequestCount)
	assert.Zero(t, rows[1].SuccessCount)
}

func TestBuildRelaySampleUsesMeasuredTimings(t *testing.T) {
	started := time.Date(2026, time.September, 28, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		stream     bool
		first      time.Time
		ttft       int64
		hasTtft    bool
		generation int64
	}{
		{name: "streaming output", stream: true, first: started.Add(200 * time.Millisecond), ttft: 200, hasTtft: true, generation: 800},
		{name: "nonstreaming has no first-token timing", first: started.Add(200 * time.Millisecond), generation: 1000},
		{name: "stream without first response", stream: true, first: started.Add(-time.Second), generation: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: 7},
				OriginModelName: "gemini", UsingGroup: "Gemini",
				StartTime: started, FirstResponseTime: tc.first, IsStream: tc.stream,
			}
			sample := BuildRelaySample(info, true, 40, started.Add(time.Second))
			assert.Equal(t, Sample{
				ChannelID: 7, Model: "gemini", Group: "Gemini", Success: true,
				LatencyMs: 1000, TtftMs: tc.ttft, HasTtft: tc.hasTtft,
				OutputTokens: 40, GenerationMs: tc.generation,
			}, sample)
		})
	}
	assert.Equal(t, Sample{}, BuildRelaySample(nil, true, 40, started))
	assert.Equal(t, Sample{}, BuildRelaySample(&relaycommon.RelayInfo{}, true, 40, started))
}

func TestQuerySummaryAllFiltersModelGroupsBeforeMergingDBAndHotBuckets(t *testing.T) {
	db := setupPerfSummaryTest(t)
	bucket := time.Now().Add(-3 * time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ChannelID: 1, ModelName: "gemini", Group: "Gemini", BucketTs: bucket, RequestCount: 2, SuccessCount: 1, TotalLatencyMs: 2000, OutputTokens: 20, GenerationMs: 2000},
		{ChannelID: 1, ModelName: "gemini", Group: "Gemini", BucketTs: bucket + 3600, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 1000, OutputTokens: 10, GenerationMs: 1000},
		{ChannelID: 1, ModelName: "gemini", Group: "default", BucketTs: bucket, RequestCount: 6},
		{ChannelID: 1, ModelName: "gemini", Group: "default", BucketTs: bucket + 7200, RequestCount: 4},
		{ChannelID: 1, ModelName: "default-model", Group: "default", BucketTs: bucket, RequestCount: 3, SuccessCount: 2, TotalLatencyMs: 3000, OutputTokens: 30, GenerationMs: 3000},
		{ChannelID: 1, ModelName: "deleted-model", Group: "Gemini", BucketTs: bucket, RequestCount: 5},
		{ChannelID: 1, ModelName: "no-groups-model", Group: "default", BucketTs: bucket, RequestCount: 7},
	}).Error)
	for _, fixture := range []struct {
		model   string
		group   string
		ts      int64
		success bool
	}{
		{model: "gemini", group: "Gemini", ts: bucket + 3600, success: true},
		{model: "gemini", group: "default", ts: bucket + 3600},
		{model: "gemini", group: "default", ts: bucket + 7200},
		{model: "default-model", group: "default", ts: bucket + 3600, success: true},
		{model: "deleted-model", group: "Gemini", ts: bucket + 3600},
		{model: "no-groups-model", group: "default", ts: bucket + 3600},
		{model: "hot-only-deleted-model", group: "default", ts: bucket + 3600},
	} {
		hot := &atomicBucket{}
		hot.add(Sample{Success: fixture.success, LatencyMs: 1000, OutputTokens: 10, GenerationMs: 1000})
		hotBuckets.Store(bucketKey{channelID: 1, model: fixture.model, group: fixture.group, bucketTs: fixture.ts}, hot)
	}

	result, err := QuerySummaryAll(24, map[string][]string{
		"gemini":          {"Gemini"},
		"default-model":   {"default"},
		"no-groups-model": {},
	})
	require.NoError(t, err)
	require.Len(t, result.Models, 2)
	byModel := make(map[string]ModelSummary)
	for _, summary := range result.Models {
		byModel[summary.ModelName] = summary
	}
	require.Contains(t, byModel, "gemini")
	assert.Equal(t, int64(4), byModel["gemini"].RequestCount)
	assert.Equal(t, 75.0, byModel["gemini"].SuccessRate)
	assert.Equal(t, []float64{50, 100}, byModel["gemini"].RecentSuccessRates)
	assert.Equal(t, int64(1000), byModel["gemini"].AvgLatencyMs)
	assert.Equal(t, 10.0, byModel["gemini"].AvgTps)
	require.Contains(t, byModel, "default-model")
	assert.Equal(t, int64(4), byModel["default-model"].RequestCount)
	assert.Equal(t, 75.0, byModel["default-model"].SuccessRate)
	assert.Equal(t, []float64{66.67, 100}, byModel["default-model"].RecentSuccessRates)

	for _, tc := range []struct {
		name   string
		groups map[string][]string
	}{
		{name: "nil"},
		{name: "empty", groups: map[string][]string{}},
		{name: "model-without-groups", groups: map[string][]string{"gemini": {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := QuerySummaryAll(24, tc.groups)
			require.NoError(t, err)
			assert.NotNil(t, result.Models)
			assert.Empty(t, result.Models)
		})
	}
}
