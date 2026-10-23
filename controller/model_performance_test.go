package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelPerformanceValidatesHours(t *testing.T) {
	for _, query := range []string{
		"hours=", "hours=0", "hours=-1", "hours=721", "hours=1.5",
		"hours=abc", "hours=99999999999999999999999", "hours=24&hours=48",
	} {
		t.Run(query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models?"+query, nil)
			GetModelPerformance(ctx)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, false, body["success"])
			assert.Equal(t, "INVALID_HOURS", body["code"])
		})
	}
}

func TestGetModelPerformanceFiltersCurrentModelGroups(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: "model-a", Group: "default", ChannelId: 1, Enabled: true},
		{Model: "model-a", Group: "Gemini", ChannelId: 1, Enabled: true},
		{Model: "model-a", Group: "private", ChannelId: 1, Enabled: true},
		{Model: "model-a", Group: "retired", ChannelId: 1, Enabled: true},
		{Model: "model-a", Group: "disabled", ChannelId: 1, Enabled: false},
		{Model: "model-b", Group: "default", ChannelId: 2, Enabled: true},
	}).Error)
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ChannelID: 1, ModelName: "model-a", Group: "default", BucketTs: time.Now().Add(-30 * time.Minute).Unix(), RequestCount: 4, SuccessCount: 3},
		{ChannelID: 1, ModelName: "model-a", Group: "private", BucketTs: time.Now().Add(-time.Hour).Unix(), RequestCount: 1},
		{ChannelID: 1, ModelName: "deleted-model", Group: "default", BucketTs: time.Now().Add(-time.Hour).Unix(), RequestCount: 1},
	}).Error)
	for _, tc := range []struct {
		name   string
		query  string
		hours  int
		groups map[string][]string
	}{
		{name: "all", hours: 24, groups: map[string][]string{"model-a": {"Gemini", "default"}, "model-b": {"default"}}},
		{name: "model", query: "model=model-a&hours=1", hours: 1, groups: map[string][]string{"model-a": {"Gemini", "default"}}},
		{name: "group", query: "group=default&hours=720", hours: 720, groups: map[string][]string{"model-a": {"default"}, "model-b": {"default"}}},
		{name: "both", query: "model=model-a&group=Gemini", hours: 24, groups: map[string][]string{"model-a": {"Gemini"}}},
		{name: "private group", query: "group=private", hours: 24, groups: map[string][]string{}},
		{name: "removed group", query: "group=retired", hours: 24, groups: map[string][]string{}},
		{name: "disabled group", query: "group=disabled", hours: 24, groups: map[string][]string{}},
		{name: "deleted model", query: "model=deleted-model", hours: 24, groups: map[string][]string{}},
		{name: "unknown model", query: "model=unknown", hours: 24, groups: map[string][]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models?"+tc.query, nil)
			ctx.Set("group", "default")
			GetModelPerformance(ctx)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var body struct {
				Success bool                     `json:"success"`
				Data    perfmetrics.ModelsResult `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			require.True(t, body.Success)
			assert.Equal(t, tc.hours, body.Data.Hours)
			assert.NotNil(t, body.Data.Models)
			actual := make(map[string][]string)
			for _, item := range body.Data.Models {
				for _, group := range item.Groups {
					actual[item.ModelName] = append(actual[item.ModelName], group.Group)
					if item.ModelName == "model-a" && group.Group == "default" {
						assert.True(t, group.HasData)
						require.NotNil(t, group.SuccessRate)
						assert.Equal(t, 75.0, *group.SuccessRate)
						continue
					}
					assert.False(t, group.HasData)
					assert.Nil(t, group.SuccessRate)
					assert.Nil(t, group.AvgLatencyMs)
					assert.Nil(t, group.AvgTtftMs)
					assert.Nil(t, group.AvgTps)
				}
			}
			assert.Equal(t, tc.groups, actual)
		})
	}
}

func TestGetModelPerformanceReturnsQueryFailure(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	require.NoError(t, db.Create(&model.Ability{Model: "model-a", Group: "default", ChannelId: 1, Enabled: true}).Error)
	require.NoError(t, db.Migrator().DropTable(&model.PerfMetric{}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models", nil)
	ctx.Set("group", "default")
	GetModelPerformance(ctx)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, false, body["success"])
	assert.Equal(t, "PERF_METRICS_QUERY_FAILED", body["code"])
	assert.NotContains(t, recorder.Body.String(), "channel_perf_metrics", "database internals must not appear in the public response")
}

func TestGetModelPerformanceDefaultsTo24HoursLikeModelSquare(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	require.NoError(t, db.Create(&model.Ability{Model: "model-a", Group: "default", ChannelId: 1, Enabled: true}).Error)
	latest := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ChannelID: 1, ModelName: "model-a", Group: "default", BucketTs: latest - 3600, RequestCount: 9, SuccessCount: 9},
		{ChannelID: 1, ModelName: "model-a", Group: "default", BucketTs: latest, RequestCount: 1},
	}).Error)
	var square struct {
		Data perfmetrics.QueryResult `json:"data"`
	}
	requestPerfMetrics(t, "/api/perf-metrics?model=model-a&hours=24", 0, GetPerfMetrics, &square)
	require.Len(t, square.Data.Groups, 1)
	assert.Equal(t, 90.0, square.Data.Groups[0].SuccessRate)

	for _, tc := range []struct {
		name  string
		query string
	}{
		{name: "default includes all 24-hour samples"},
		{name: "explicit 24-hour window", query: "?hours=24"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/models"+tc.query, nil)
			ctx.Set("group", "default")
			GetModelPerformance(ctx)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var body struct {
				Success bool                     `json:"success"`
				Data    perfmetrics.ModelsResult `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			require.True(t, body.Success)
			assert.Equal(t, 24, body.Data.Hours)
			require.Len(t, body.Data.Models, 1)
			require.Len(t, body.Data.Models[0].Groups, 1)
			group := body.Data.Models[0].Groups[0]
			require.NotNil(t, group.SuccessRate)
			assert.Equal(t, square.Data.Groups[0].SuccessRate, *group.SuccessRate)
		})
	}
}
