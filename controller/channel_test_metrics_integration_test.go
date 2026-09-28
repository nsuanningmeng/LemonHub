package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualChannelTestMetricsRequireEnabledStateThroughoutProbe(t *testing.T) {
	for _, tc := range []struct {
		name          string
		initialStatus int
		responseState int
		statusCode    int
		wantSample    bool
		wantRate      float64
	}{
		{
			name:          "disabled while probe is running",
			initialStatus: common.ChannelStatusEnabled,
			responseState: common.ChannelStatusManuallyDisabled,
			statusCode:    http.StatusTooManyRequests,
		},
		{
			name:          "disabled probe reenabled before response",
			initialStatus: common.ChannelStatusManuallyDisabled,
			responseState: common.ChannelStatusEnabled,
			statusCode:    http.StatusTooManyRequests,
		},
		{
			name:          "enabled listed failure",
			initialStatus: common.ChannelStatusEnabled,
			responseState: common.ChannelStatusEnabled,
			statusCode:    http.StatusTooManyRequests,
			wantSample:    true,
		},
		{
			name:          "enabled unlisted failure counts as healthy",
			initialStatus: common.ChannelStatusEnabled,
			responseState: common.ChannelStatusEnabled,
			statusCode:    http.StatusBadRequest,
			wantSample:    true,
			wantRate:      100,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupPerfMetricsControllerTest(t)
			originalMetrics := perf_metrics_setting.GetSetting()
			originalModelRatios := ratio_setting.ModelRatio2JSONString()
			fetchSetting := system_setting.GetFetchSetting()
			originalFetchSetting := *fetchSetting
			t.Cleanup(func() {
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
					"perf_metrics_setting.enabled":              strconv.FormatBool(originalMetrics.Enabled),
					"perf_metrics_setting.error_code_whitelist": originalMetrics.ErrorCodeWhitelist,
				}))
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatios))
				*fetchSetting = originalFetchSetting
			})
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"perf_metrics_setting.enabled":              "true",
				"perf_metrics_setting.error_code_whitelist": "429",
			}))
			*fetchSetting = system_setting.FetchSetting{
				EnableSSRFProtection: true,
				AllowPrivateIp:       true,
				AllowedPorts:         []string{"1-65535"},
			}
			modelName := "manual-probe-metrics-" + strings.ReplaceAll(tc.name, " ", "-")
			ratios, err := common.Marshal(map[string]float64{modelName: 1})
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratios)))
			user := model.User{Username: "probe-metrics-admin", Group: "default", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&user).Error)
			channel := model.Channel{
				Name:   "manual probe metrics",
				Type:   constant.ChannelTypeOpenAI,
				Status: tc.initialStatus,
				Key:    "test-channel-key",
				Models: modelName,
				Group:  "Gemini,Gemini混合",
			}
			require.NoError(t, db.Create(&channel).Error)
			upstreamResult := make(chan error, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
					upstreamResult <- fmt.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
				} else {
					upstreamResult <- db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", tc.responseState).Error
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(`{"error":{"message":"test upstream rejection","type":"test_error","code":"test_error"}}`))
			}))
			t.Cleanup(upstream.Close)
			require.NoError(t, db.Model(&channel).Update("base_url", upstream.URL).Error)

			// Compare increments so repeated test runs do not depend on a package-private
			// metrics-cache reset. Models are isolated from other controller fixtures.
			modelGroups := map[string][]string{modelName: {"Gemini", "Gemini混合"}}
			before, err := perfmetrics.QuerySummaryAll(24, modelGroups)
			require.NoError(t, err)
			var beforeCount int64
			if len(before.Models) > 0 {
				beforeCount = before.Models[0].RequestCount
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/"+strconv.Itoa(channel.Id), nil)
			ctx.Set("id", user.Id)
			TestChannel(ctx)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			select {
			case err := <-upstreamResult:
				require.NoError(t, err)
			default:
				t.Fatalf("channel test did not reach the upstream: %s", recorder.Body.String())
			}
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success, "health metrics must not change the actual test result")
			assert.Contains(t, response.Message, "test upstream rejection")

			// Restore availability before querying: filtering a disabled channel must
			// not conceal a sample that the test handler incorrectly recorded.
			require.NoError(t, db.Model(&channel).Update("status", common.ChannelStatusEnabled).Error)
			detail, err := perfmetrics.Query(perfmetrics.QueryParams{Model: modelName})
			require.NoError(t, err)
			summary, err := perfmetrics.QuerySummaryAll(24, modelGroups)
			require.NoError(t, err)
			if !tc.wantSample {
				assert.Empty(t, detail.Groups)
				assert.Empty(t, summary.Models)
				return
			}
			require.Len(t, detail.Groups, 2)
			for _, group := range detail.Groups {
				assert.Equal(t, tc.wantRate, group.SuccessRate)
				assert.Zero(t, group.AvgTps)
				assert.Zero(t, group.AvgTtftMs)
			}
			require.Len(t, summary.Models, 1)
			assert.Equal(t, beforeCount+2, summary.Models[0].RequestCount)
			assert.Equal(t, tc.wantRate, summary.Models[0].SuccessRate)
		})
	}
}
