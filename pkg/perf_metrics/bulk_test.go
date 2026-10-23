package perfmetrics

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryModelsAggregatesVisiblePairsAndMissingMeasurements(t *testing.T) {
	db := setupPerfSummaryTest(t)
	require.NoError(t, db.Create(&model.Channel{Id: 2, Status: common.ChannelStatusEnabled}).Error)
	bucket := bucketStart(time.Now().Add(-time.Hour).Unix())
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ChannelID: 1, ModelName: "a", Group: "first", BucketTs: bucket, RequestCount: 4, SuccessCount: 3, TotalLatencyMs: 4000, TtftSumMs: 400, TtftCount: 2, OutputTokens: 100, GenerationMs: 2000},
		{ChannelID: 2, ModelName: "a", Group: "first", BucketTs: bucket, RequestCount: 5, SuccessCount: 4, TotalLatencyMs: 10000, TtftSumMs: 800, TtftCount: 3, OutputTokens: 200, GenerationMs: 4000},
		{ChannelID: 1, ModelName: "b", Group: "second", BucketTs: bucket, RequestCount: 1},
		// A group visible for another model does not grant access to this pair.
		{ChannelID: 1, ModelName: "b", Group: "first", BucketTs: bucket, RequestCount: 10, SuccessCount: 10},
		{ChannelID: 1, ModelName: "hidden", Group: "first", BucketTs: bucket, RequestCount: 10},
		{ChannelID: 1, ModelName: "a", Group: "private", BucketTs: bucket, RequestCount: 10},
		{ChannelID: 1, ModelName: "a", Group: "first", BucketTs: bucket - 48*3600, RequestCount: 10},
		{ChannelID: 1, ModelName: "a", Group: "first", BucketTs: bucket + 48*3600, RequestCount: 10},
	}).Error)
	hot := &metricBucket{}
	hot.add(Sample{LatencyMs: 6000, HasTtft: true, TtftMs: 0, OutputTokens: 100, GenerationMs: 4000})
	hotBuckets.Store(bucketKey{channelID: 1, model: "a", group: "first", bucketTs: bucket}, hot)
	hiddenHot := &metricBucket{}
	hiddenHot.add(Sample{Success: true})
	hotBuckets.Store(bucketKey{channelID: 1, model: "b", group: "first", bucketTs: bucket}, hiddenHot)
	hotBuckets.Store(bucketKey{channelID: 1, model: "a", group: "first", bucketTs: bucket - 48*3600}, hiddenHot)
	hotBuckets.Store(bucketKey{channelID: 1, model: "a", group: "first", bucketTs: bucket + 48*3600}, hiddenHot)

	result, err := QueryModels(24, map[string][]string{
		"c": {"first"}, "b": {"second"}, "a": {"second", "first", "first"}, "empty": {},
	})
	require.NoError(t, err)
	require.Len(t, result.Models, 3)
	assert.Equal(t, "a", result.Models[0].ModelName)
	assert.Equal(t, "b", result.Models[1].ModelName)
	assert.Equal(t, "c", result.Models[2].ModelName)
	require.Len(t, result.Models[0].Groups, 2)
	metrics := result.Models[0].Groups[0]
	assert.Equal(t, "first", metrics.Group)
	assert.True(t, metrics.HasData)
	require.NotNil(t, metrics.SuccessRate)
	require.NotNil(t, metrics.AvgLatencyMs)
	require.NotNil(t, metrics.AvgTtftMs)
	require.NotNil(t, metrics.AvgTps)
	assert.Equal(t, 70.0, *metrics.SuccessRate)
	assert.Equal(t, int64(2000), *metrics.AvgLatencyMs)
	assert.Equal(t, int64(200), *metrics.AvgTtftMs)
	assert.Equal(t, 40.0, *metrics.AvgTps)
	assert.Equal(t, GroupMetrics{Group: "second"}, result.Models[0].Groups[1])
	assert.Equal(t, []GroupMetrics{{Group: "first"}}, result.Models[2].Groups)
	require.Len(t, result.Models[1].Groups, 1)
	failed := result.Models[1].Groups[0]
	assert.True(t, failed.HasData)
	require.NotNil(t, failed.SuccessRate)
	require.NotNil(t, failed.AvgLatencyMs)
	assert.Zero(t, *failed.SuccessRate)
	assert.Zero(t, *failed.AvgLatencyMs)
	assert.Nil(t, failed.AvgTtftMs)
	assert.Nil(t, failed.AvgTps)

	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	var wire struct {
		Models []struct {
			Groups []map[string]any `json:"groups"`
		} `json:"models"`
	}
	require.NoError(t, common.Unmarshal(encoded, &wire))
	assert.Equal(t, map[string]any{
		"group": "second", "has_data": false, "success_rate": nil,
		"avg_latency_ms": nil, "avg_ttft_ms": nil, "avg_tps": nil,
	}, wire.Models[0].Groups[1])
	assert.NotContains(t, string(encoded), "request_count")
	assert.NotContains(t, string(encoded), "success_count")
	assert.NotContains(t, string(encoded), "channel_id")
}

func TestQueryModelsAppliesCurrentChannelStateToPersistedAndHotSamples(t *testing.T) {
	db := setupPerfSummaryTest(t)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}).Error)
	bucket := bucketStart(time.Now().Add(-time.Hour).Unix())
	for _, channelID := range []int{1, 2, 3, 4} {
		successes := int64(0)
		if channelID == 1 {
			successes = 1
		}
		require.NoError(t, db.Create(&model.PerfMetric{
			ChannelID: channelID, ModelName: "model", Group: "group", BucketTs: bucket, RequestCount: 1, SuccessCount: successes,
		}).Error)
		hot := &metricBucket{}
		hot.add(Sample{Success: channelID == 1})
		hotBuckets.Store(bucketKey{channelID: channelID, model: "model", group: "group", bucketTs: bucket}, hot)
	}
	for _, step := range []struct {
		name      string
		channelID int
		status    int
		delete    bool
		hasData   bool
		rate      float64
	}{
		{name: "auto disabled failures remain; manual and deleted channels excluded", hasData: true, rate: 50},
		{name: "manual disable removes successful history", channelID: 1, status: common.ChannelStatusManuallyDisabled, hasData: true},
		{name: "no eligible samples", channelID: 2, status: common.ChannelStatusManuallyDisabled},
		{name: "restoring a channel restores its samples", channelID: 1, status: common.ChannelStatusEnabled, hasData: true, rate: 100},
		{name: "deleted channel history is removed", channelID: 1, delete: true},
	} {
		t.Run(step.name, func(t *testing.T) {
			if step.delete {
				require.NoError(t, db.Delete(&model.Channel{}, step.channelID).Error)
			} else if step.channelID > 0 {
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", step.channelID).Update("status", step.status).Error)
			}
			result, err := QueryModels(24, map[string][]string{"model": {"group"}})
			require.NoError(t, err)
			require.Len(t, result.Models, 1)
			require.Len(t, result.Models[0].Groups, 1)
			metrics := result.Models[0].Groups[0]
			assert.Equal(t, step.hasData, metrics.HasData)
			if step.hasData {
				require.NotNil(t, metrics.SuccessRate)
				assert.Equal(t, step.rate, *metrics.SuccessRate)
			} else {
				assert.Nil(t, metrics.SuccessRate)
			}
		})
	}
}

func TestQueryModelsMatchesModelSquareWindowAndMetrics(t *testing.T) {
	db := setupPerfSummaryTest(t)
	// The oldest bucket straddles the cutoff (or ends at it when the clock is
	// exactly on a boundary). Like the model square, exclude that whole bucket.
	oldest := bucketStart(time.Now().Add(-24*time.Hour).Unix() - 1)
	inside := bucketStart(time.Now().Add(-time.Hour).Unix())
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ChannelID: 1, ModelName: "model", Group: "current", BucketTs: oldest, RequestCount: 100, SuccessCount: 100},
		{ChannelID: 1, ModelName: "model", Group: "old-only", BucketTs: oldest, RequestCount: 100, SuccessCount: 100},
		{ChannelID: 1, ModelName: "model", Group: "current", BucketTs: inside, RequestCount: 3, SuccessCount: 1, TotalLatencyMs: 6000, TtftSumMs: 300, TtftCount: 2, OutputTokens: 200, GenerationMs: 10000},
	}).Error)
	oldHot := &metricBucket{}
	oldHot.addCounters(counters{requestCount: 100, successCount: 100})
	hotBuckets.Store(bucketKey{channelID: 1, model: "model", group: "current", bucketTs: oldest}, oldHot)
	currentHot := &metricBucket{}
	currentHot.add(Sample{Success: true, LatencyMs: 2000, HasTtft: true, TtftMs: 300, OutputTokens: 100, GenerationMs: 5000})
	hotBuckets.Store(bucketKey{channelID: 1, model: "model", group: "current", bucketTs: inside}, currentHot)

	visible := map[string][]string{"model": {"current", "old-only"}}
	result, err := QueryModels(0, visible)
	require.NoError(t, err)
	assert.Equal(t, 24, result.Hours)
	assert.Positive(t, result.BucketSeconds)
	assert.Equal(t, int64(24*3600), result.EndTime-result.StartTime)
	require.Len(t, result.Models, 1)
	require.Len(t, result.Models[0].Groups, 2)
	metrics := result.Models[0].Groups[0]
	require.NotNil(t, metrics.SuccessRate)
	require.NotNil(t, metrics.AvgLatencyMs)
	require.NotNil(t, metrics.AvgTtftMs)
	require.NotNil(t, metrics.AvgTps)
	assert.Equal(t, 50.0, *metrics.SuccessRate)
	assert.Equal(t, int64(2000), *metrics.AvgLatencyMs)
	assert.Equal(t, int64(200), *metrics.AvgTtftMs)
	assert.Equal(t, 20.0, *metrics.AvgTps)
	assert.Equal(t, GroupMetrics{Group: "old-only"}, result.Models[0].Groups[1])

	detail, err := Query(QueryParams{Model: "model", Hours: 24})
	require.NoError(t, err)
	require.Len(t, detail.Groups, 1)
	assert.Equal(t, detail.Groups[0].SuccessRate, *metrics.SuccessRate)
	assert.Equal(t, detail.Groups[0].AvgLatencyMs, *metrics.AvgLatencyMs)
	assert.Equal(t, detail.Groups[0].AvgTtftMs, *metrics.AvgTtftMs)
	assert.Equal(t, detail.Groups[0].AvgTps, *metrics.AvgTps)
	summary, err := QuerySummaryAll(24, visible)
	require.NoError(t, err)
	require.Len(t, summary.Models, 1)
	assert.Equal(t, summary.Models[0].SuccessRate, *metrics.SuccessRate)
	assert.Equal(t, int64(4), summary.Models[0].RequestCount)

	empty, err := QueryModels(24, nil)
	require.NoError(t, err)
	assert.NotNil(t, empty.Models)
	assert.Empty(t, empty.Models)
}

func TestQueryModelsRejectsUnavailableDatabaseState(t *testing.T) {
	for _, missing := range []struct {
		name  string
		table any
	}{
		{name: "channel state", table: &model.Channel{}},
		{name: "performance data", table: &model.PerfMetric{}},
	} {
		t.Run(missing.name, func(t *testing.T) {
			db := setupPerfSummaryTest(t)
			hot := &metricBucket{}
			hot.add(Sample{Success: true})
			hotBuckets.Store(bucketKey{channelID: 1, model: "model", group: "group", bucketTs: bucketStart(time.Now().Unix())}, hot)
			require.NoError(t, db.Migrator().DropTable(missing.table))
			result, err := QueryModels(24, map[string][]string{"model": {"group"}})
			require.Error(t, err)
			assert.Empty(t, result.Models)
		})
	}
}
