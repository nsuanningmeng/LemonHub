package perfmetrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPerfSummaryTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	originalSQLitePath, originalMaster := common.SQLitePath, common.IsMasterNode
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
		hotBuckets.Clear()
		for key, value := range originalHot {
			hotBuckets.Store(key, value)
		}
	})
	t.Setenv("SQL_DSN", "local")
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	common.IsMasterNode = false
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	return db
}

func TestQuerySummaryAllFiltersModelGroupsBeforeMergingDBAndHotBuckets(t *testing.T) {
	db := setupPerfSummaryTest(t)
	bucket := time.Now().Add(-3 * time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ModelName: "gemini", Group: "Gemini", BucketTs: bucket, RequestCount: 2, SuccessCount: 1, TotalLatencyMs: 2000, OutputTokens: 20, GenerationMs: 2000},
		{ModelName: "gemini", Group: "Gemini", BucketTs: bucket + 3600, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 1000, OutputTokens: 10, GenerationMs: 1000},
		{ModelName: "gemini", Group: "default", BucketTs: bucket, RequestCount: 6},
		{ModelName: "gemini", Group: "default", BucketTs: bucket + 7200, RequestCount: 4},
		{ModelName: "default-model", Group: "default", BucketTs: bucket, RequestCount: 3, SuccessCount: 2, TotalLatencyMs: 3000, OutputTokens: 30, GenerationMs: 3000},
		{ModelName: "deleted-model", Group: "Gemini", BucketTs: bucket, RequestCount: 5},
		{ModelName: "no-groups-model", Group: "default", BucketTs: bucket, RequestCount: 7},
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
		hotBuckets.Store(bucketKey{model: fixture.model, group: fixture.group, bucketTs: fixture.ts}, hot)
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
