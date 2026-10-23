package perfmetrics

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPerformanceQueriesStayConsistentDuringFlush(t *testing.T) {
	for _, query := range []string{"detail", "summary", "models"} {
		for _, phase := range []string{"before database read", "after database read"} {
			t.Run(query+"/"+phase, func(t *testing.T) {
				db := setupPerfSummaryTest(t)
				bucket := bucketStart(time.Now().Add(-time.Hour).Unix())
				require.NoError(t, model.UpsertPerfMetric(&model.PerfMetric{
					ChannelID: 1, ModelName: "model", Group: "default", BucketTs: bucket,
					RequestCount: 9, SuccessCount: 9, TotalLatencyMs: 9000,
				}))
				hot := &metricBucket{}
				hot.add(Sample{Success: false, LatencyMs: 2000})
				hotBuckets.Store(bucketKey{channelID: 1, model: "model", group: "default", bucketTs: bucket}, hot)

				// Move the same samples at either side of the actual database read.
				// Without a consistent snapshot these schedules respectively double
				// count the hot sample or lose it when reading hot buckets afterward.
				flushed := false
				flushDuringRead := func(tx *gorm.DB) {
					if tx.Statement.Table == "channel_perf_metrics" && !flushed {
						flushed = true
						flushCompletedBuckets()
					}
				}
				const callbackName = "test:flush_during_read"
				if phase == "before database read" {
					require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, flushDuringRead))
				} else {
					require.NoError(t, db.Callback().Query().After("gorm:query").Register(callbackName, flushDuringRead))
				}
				t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callbackName)) })

				var rate float64
				var latency int64
				switch query {
				case "detail":
					result, err := Query(QueryParams{Model: "model", Group: "default", Hours: 24})
					require.NoError(t, err)
					require.Len(t, result.Groups, 1)
					rate, latency = result.Groups[0].SuccessRate, result.Groups[0].AvgLatencyMs
				case "summary":
					result, err := QuerySummaryAll(24, map[string][]string{"model": {"default"}})
					require.NoError(t, err)
					require.Len(t, result.Models, 1)
					assert.Equal(t, int64(10), result.Models[0].RequestCount)
					rate, latency = result.Models[0].SuccessRate, result.Models[0].AvgLatencyMs
				case "models":
					result, err := QueryModels(24, map[string][]string{"model": {"default"}})
					require.NoError(t, err)
					require.Len(t, result.Models, 1)
					require.Len(t, result.Models[0].Groups, 1)
					metrics := result.Models[0].Groups[0]
					require.NotNil(t, metrics.SuccessRate)
					require.NotNil(t, metrics.AvgLatencyMs)
					rate, latency = *metrics.SuccessRate, *metrics.AvgLatencyMs
				}
				assert.True(t, flushed)
				assert.Equal(t, 90.0, rate)
				assert.Equal(t, int64(1100), latency)
			})
		}
	}
}

func TestFailedFlushPreservesSamplesWhileRecordingContinues(t *testing.T) {
	db := setupPerfSummaryTest(t)
	bucket := bucketStart(time.Now().Add(-time.Hour).Unix())
	hot := &metricBucket{}
	hot.add(Sample{Success: true, LatencyMs: 1000})
	hot.add(Sample{Success: false, LatencyMs: 3000})
	hotBuckets.Store(bucketKey{channelID: 1, model: "model", group: "default", bucketTs: bucket}, hot)

	const callbackName = "test:failed_persistence"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "channel_perf_metrics" {
			return
		}
		// This runs while the persistence transfer is active. Recording must
		// finish without waiting for that database operation to return.
		Record(Sample{ChannelID: 1, Model: "live", Group: "default", Success: true})
		// Also retain a sample arriving on the bucket being transferred.
		hot.add(Sample{Success: true, LatencyMs: 2000})
		tx.AddError(errors.New("test persistence failure"))
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callbackName)) })
	flushCompletedBuckets()
	require.NoError(t, db.Callback().Create().Remove(callbackName))

	for _, phase := range []string{"restored in memory", "persisted after retry"} {
		if phase == "persisted after retry" {
			flushCompletedBuckets()
		}
		result, err := QuerySummaryAll(24, map[string][]string{"model": {"default"}, "live": {"default"}})
		require.NoError(t, err, phase)
		require.Len(t, result.Models, 2, phase)
		byModel := make(map[string]ModelSummary)
		for _, summary := range result.Models {
			byModel[summary.ModelName] = summary
		}
		assert.Equal(t, int64(3), byModel["model"].RequestCount, phase)
		assert.Equal(t, 66.67, byModel["model"].SuccessRate, phase)
		assert.Equal(t, int64(2000), byModel["model"].AvgLatencyMs, phase)
		assert.Equal(t, int64(1), byModel["live"].RequestCount, phase)
		assert.Equal(t, 100.0, byModel["live"].SuccessRate, phase)
	}
}
