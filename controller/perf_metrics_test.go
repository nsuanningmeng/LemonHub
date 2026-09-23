package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPerfMetricsControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	originalRedis, originalMemory := common.RedisEnabled, common.MemoryCacheEnabled
	originalGinMode := gin.Mode()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	originalSpecial := specialGroups.ReadAll()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.RedisEnabled, common.MemoryCacheEnabled = originalRedis, originalMemory
		gin.SetMode(originalGinMode)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecial)
		model.InvalidatePricingCache()
	})
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	specialGroups.Clear()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"Gemini":1,"Gemini混合":3,"private":1,"disabled":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","Gemini":"Gemini","Gemini混合":"Mixed","disabled":"Disabled","retired":"Retired"}`))
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	model.InvalidatePricingCache()
	return db
}

func requestPerfMetrics(t *testing.T, path string, userID int, handler gin.HandlerFunc, result any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if userID != 0 {
		ctx.Set("id", userID)
	}
	handler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var envelope struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, recorder.Body.String())
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), result))
}

func TestGetPerfMetricsRemovesDeletedModelGroup(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	const modelName = "perf-gemini-model"
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: modelName, Group: "Gemini", ChannelId: 1, Enabled: true},
		{Model: modelName, Group: "Gemini混合", ChannelId: 1, Enabled: true},
		{Model: modelName, Group: "default", ChannelId: 2, Enabled: true},
		{Model: "perf-default-model", Group: "default", ChannelId: 2, Enabled: true},
	}).Error)
	bucket := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ModelName: modelName, Group: "Gemini", BucketTs: bucket, RequestCount: 2, SuccessCount: 2},
		{ModelName: modelName, Group: "default", BucketTs: bucket, RequestCount: 3, SuccessCount: 0},
	}).Error)
	var response struct {
		Data perfmetrics.QueryResult `json:"data"`
	}
	path := "/api/perf_metrics?model=" + modelName
	requestPerfMetrics(t, path, 0, GetPerfMetrics, &response)
	require.Len(t, response.Data.Groups, 2)

	require.NoError(t, db.Where(&model.Ability{Model: modelName, Group: "default"}).Delete(&model.Ability{}).Error)
	model.InvalidatePricingCache()
	requestPerfMetrics(t, path, 0, GetPerfMetrics, &response)
	require.Len(t, response.Data.Groups, 1)
	assert.Equal(t, "Gemini", response.Data.Groups[0].Group)
	assert.Equal(t, 100.0, response.Data.Groups[0].SuccessRate)
	assert.ElementsMatch(t, []string{"Gemini", "Gemini混合"}, pricingByModelName(model.GetPricing())[modelName].EnableGroup)

	requestPerfMetrics(t, path+"&group=default", 0, GetPerfMetrics, &response)
	assert.Empty(t, response.Data.Groups)
	requestPerfMetrics(t, path+"&group="+url.QueryEscape("Gemini混合"), 0, GetPerfMetrics, &response)
	assert.Empty(t, response.Data.Groups, "enabled groups without samples must not acquire fabricated zero metrics")
}

func TestGetPerfMetricsSummaryExcludesUnavailableModelGroups(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	const modelName = "perf-summary-model"
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: modelName, Group: "Gemini", ChannelId: 1, Enabled: true},
		{Model: modelName, Group: "retired", ChannelId: 1, Enabled: true},
		{Model: modelName, Group: "private", ChannelId: 1, Enabled: true},
		{Model: modelName, Group: "disabled", ChannelId: 2, Enabled: false},
		{Model: "perf-default-model", Group: "default", ChannelId: 3, Enabled: true},
	}).Error)
	bucket := time.Now().Add(-3 * time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ModelName: modelName, Group: "Gemini", BucketTs: bucket, RequestCount: 2, SuccessCount: 1},
		{ModelName: modelName, Group: "Gemini", BucketTs: bucket + 3600, RequestCount: 1, SuccessCount: 1},
		{ModelName: modelName, Group: "default", BucketTs: bucket, RequestCount: 8},
		{ModelName: modelName, Group: "default", BucketTs: bucket + 7200, RequestCount: 5},
		{ModelName: modelName, Group: "private", BucketTs: bucket, RequestCount: 6},
		{ModelName: modelName, Group: "retired", BucketTs: bucket, RequestCount: 7},
		{ModelName: modelName, Group: "disabled", BucketTs: bucket, RequestCount: 9},
		{ModelName: "perf-default-model", Group: "default", BucketTs: bucket, RequestCount: 4, SuccessCount: 3},
		{ModelName: "perf-deleted-model", Group: "Gemini", BucketTs: bucket, RequestCount: 10},
	}).Error)
	var response struct {
		Data perfmetrics.SummaryAllResult `json:"data"`
	}
	requestPerfMetrics(t, "/api/perf_metrics/summary", 0, GetPerfMetricsSummary, &response)
	require.Len(t, response.Data.Models, 2)
	byModel := make(map[string]perfmetrics.ModelSummary)
	for _, summary := range response.Data.Models {
		byModel[summary.ModelName] = summary
	}
	assert.Equal(t, 66.67, byModel[modelName].SuccessRate)
	assert.Equal(t, []float64{50, 100}, byModel[modelName].RecentSuccessRates)
	assert.Equal(t, 75.0, byModel["perf-default-model"].SuccessRate)
	assert.Equal(t, []float64{75}, byModel["perf-default-model"].RecentSuccessRates)

	require.NoError(t, db.Where("enabled = ?", true).Delete(&model.Ability{}).Error)
	model.InvalidatePricingCache()
	requestPerfMetrics(t, "/api/perf_metrics/summary", 0, GetPerfMetricsSummary, &response)
	assert.NotNil(t, response.Data.Models)
	assert.Empty(t, response.Data.Models)
	var detail struct {
		Data perfmetrics.QueryResult `json:"data"`
	}
	requestPerfMetrics(t, "/api/perf_metrics?model="+modelName, 0, GetPerfMetrics, &detail)
	assert.NotNil(t, detail.Data.Groups)
	assert.Empty(t, detail.Data.Groups)
}

func TestGetPerfMetricsUsesCurrentUsersUsableGroups(t *testing.T) {
	db := setupPerfMetricsControllerTest(t)
	const modelName = "perf-user-visible-model"
	require.NoError(t, db.Create(&model.User{Id: 2101, Username: "perf-member", Group: "member", Status: common.UserStatusEnabled}).Error)
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Set("member", map[string]string{
		"-:Gemini":  "",
		"+:private": "Private",
	})
	require.NoError(t, db.Create(&[]model.Ability{
		{Model: modelName, Group: "Gemini", ChannelId: 1, Enabled: true},
		{Model: modelName, Group: "private", ChannelId: 2, Enabled: true},
	}).Error)
	bucket := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ModelName: modelName, Group: "Gemini", BucketTs: bucket, RequestCount: 1, SuccessCount: 1},
		{ModelName: modelName, Group: "private", BucketTs: bucket, RequestCount: 1, SuccessCount: 0},
	}).Error)
	for _, tc := range []struct {
		name   string
		userID int
		group  string
		rate   float64
	}{
		{name: "anonymous", group: "Gemini", rate: 100},
		{name: "member", userID: 2101, group: "private", rate: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var detail struct {
				Data perfmetrics.QueryResult `json:"data"`
			}
			requestPerfMetrics(t, "/api/perf_metrics?model="+modelName, tc.userID, GetPerfMetrics, &detail)
			require.Len(t, detail.Data.Groups, 1)
			assert.Equal(t, tc.group, detail.Data.Groups[0].Group)
			var summary struct {
				Data perfmetrics.SummaryAllResult `json:"data"`
			}
			requestPerfMetrics(t, "/api/perf_metrics/summary", tc.userID, GetPerfMetricsSummary, &summary)
			require.Len(t, summary.Data.Models, 1)
			assert.Equal(t, tc.rate, summary.Data.Models[0].SuccessRate)
			assert.Equal(t, []float64{tc.rate}, summary.Data.Models[0].RecentSuccessRates)
		})
	}
}
