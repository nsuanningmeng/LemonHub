package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var retryLoggingChannelID atomic.Int64

func TestRelayRetryLogsExposeOnlyFinalOutcome(t *testing.T) {
	for _, tc := range []struct {
		name          string
		statuses      []int
		poolExhausted bool
	}{
		{name: "success after two failures", statuses: []int{503, 502, 200}},
		{name: "all attempts fail", statuses: []int{503, 502, 500}},
		{name: "no channel after first failure", statuses: []int{503}, poolExhausted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, token, _ := mediaConversionBillingFixture(t)
			require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
			oldErrorLog := constant.ErrorLogEnabled
			constant.ErrorLogEnabled = true
			t.Cleanup(func() { constant.ErrorLogEnabled = oldErrorLog })
			perfSetting := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
			oldPerf := *perfSetting
			perfSetting.Enabled, perfSetting.ErrorCodeWhitelist = true, ""
			t.Cleanup(func() { *perfSetting = oldPerf })
			common.RetryTimes = 2

			channels := make([]model.Channel, len(tc.statuses))
			calls := make([]atomic.Int64, len(tc.statuses))
			for i, status := range tc.statuses {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls[i].Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					if status == http.StatusOK {
						_, _ = io.WriteString(w, `{"id":"retry-success","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"backup success"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
						return
					}
					_, _ = fmt.Fprintf(w, `{"error":{"code":"fixture_upstream_error","message":"fixture upstream failure %d"}}`, status)
				}))
				t.Cleanup(server.Close)
				channels[i] = model.Channel{
					Id: 945000 + int(retryLoggingChannelID.Add(1)), Type: constant.ChannelTypeOpenAI,
					Key: "fixture", Name: fmt.Sprintf("retry-attempt-%d", i), BaseURL: common.GetPointer(server.URL),
					Status: common.ChannelStatusEnabled, AutoBan: common.GetPointer(0),
				}
				require.NoError(t, db.Create(&channels[i]).Error)
				if !tc.poolExhausted {
					// One channel at each priority makes both retries deterministic.
					require.NoError(t, db.Create(&model.Ability{
						Group: "default", Model: "gpt-4o", ChannelId: channels[i].Id,
						Enabled: true, Priority: common.GetPointer(int64(len(tc.statuses) - i)),
					}).Error)
				}
			}

			t.Cleanup(func() { waitCancellationHostWork(t) })
			response := httptest.NewRecorder()
			runOpenAIErrorHost(t, user, token, channels[0], "wallet_only", false, response)
			waitCancellationHostWork(t)
			finalStatus := tc.statuses[len(tc.statuses)-1]
			require.Equal(t, finalStatus, response.Code, response.Body.String())
			for i := range calls {
				assert.Equal(t, int64(1), calls[i].Load(), "attempt %d", i)
			}

			adminLogs, adminTotal, err := model.GetAllLogs(model.LogTypeUnknown, 0, 0, "", "", "", 0, 20, 0, "", "", "", model.SiteScopeAll)
			require.NoError(t, err)
			require.Len(t, adminLogs, len(tc.statuses))
			assert.Equal(t, int64(len(tc.statuses)), adminTotal)
			byChannel := make(map[int]*model.Log, len(adminLogs))
			for _, entry := range adminLogs {
				byChannel[entry.ChannelId] = entry
			}
			for i, channel := range channels {
				entry := byChannel[channel.Id]
				require.NotNil(t, entry, "administrator retains attempt %d", i)
				assert.Equal(t, i < len(channels)-1, entry.IsRetry, "attempt %d", i)
				if tc.statuses[i] != http.StatusOK {
					assert.Equal(t, model.LogTypeError, entry.Type)
					assert.Contains(t, entry.Content, fmt.Sprintf("fixture upstream failure %d", tc.statuses[i]))
				}
			}

			userLogs, userTotal, err := model.GetUserLogs(user.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "")
			require.NoError(t, err)
			require.Len(t, userLogs, 1)
			assert.Equal(t, int64(1), userTotal)
			assert.False(t, userLogs[0].IsRetry)
			assert.Equal(t, channels[len(channels)-1].Id, userLogs[0].ChannelId)
			wantType, wantErrorCount, wantRate := model.LogTypeError, int64(1), 0.0
			if finalStatus == http.StatusOK {
				wantType, wantErrorCount, wantRate = model.LogTypeConsume, 0, 100
				assert.Contains(t, response.Body.String(), "backup success")
				assert.NotContains(t, response.Body.String(), "fixture upstream failure")
			} else {
				assert.Contains(t, userLogs[0].Content, fmt.Sprintf("fixture upstream failure %d", finalStatus))
			}
			assert.Equal(t, wantType, userLogs[0].Type)
			userErrors, errorTotal, err := model.GetUserLogs(user.Id, model.LogTypeError, 0, 0, "", "", 0, 20, "", "", "")
			require.NoError(t, err)
			assert.Len(t, userErrors, int(wantErrorCount))
			assert.Equal(t, wantErrorCount, errorTotal, "hidden attempts must not inflate pagination totals")
			tokenLogs, err := model.GetLogByTokenId(token.Id)
			require.NoError(t, err)
			require.Len(t, tokenLogs, 1)
			assert.Equal(t, wantType, tokenLogs[0].Type)
			assert.False(t, tokenLogs[0].IsRetry)

			metrics, err := perfmetrics.QuerySummaryAll(24, map[string][]string{"gpt-4o": {"default"}})
			require.NoError(t, err)
			require.Len(t, metrics.Models, 1)
			assert.Equal(t, int64(1), metrics.Models[0].RequestCount, "one client request produces one performance sample")
			assert.Equal(t, wantRate, metrics.Models[0].SuccessRate)
		})
	}
}

func TestRelayRetryLogsDoNotExposeEarlierFailureWhenFinalErrorIsLocal(t *testing.T) {
	db, user, token, _ := mediaConversionBillingFixture(t)
	oldErrorLog := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = oldErrorLog })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", user.Id)
	c.Set("token_id", token.Id)
	c.Set("original_model", "gpt-4o")
	c.Set("group", "default")
	c.Set("channel_id", 1)
	c.Set("channel_name", "first-channel")
	c.Set("channel_type", constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyUpstreamModelName, "private-model-a")
	pending := &relayChannelErrorLogs{}
	c.Set(relayChannelErrorLogsKey, pending)
	upstreamErr := types.NewOpenAIError(errors.New("private-model-a is unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	upstreamErr.SetUserMessageOverride("Upstream temporarily unavailable")
	processChannelError(c, types.ChannelError{ChannelId: 1}, upstreamErr)

	// The next channel cannot reserve quota. Its error deliberately opts out of
	// logging and must not promote the previous upstream failure to final status.
	c.Set("channel_id", 2)
	c.Set("channel_name", "second-channel")
	common.SetContextKey(c, constant.ContextKeyUpstreamModelName, "private-model-b")
	finalErr := types.NewErrorWithStatusCode(errors.New("insufficient quota"), types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden, types.ErrOptionWithNoRecordErrorLog())
	processChannelError(c, types.ChannelError{ChannelId: 2}, finalErr)
	pending.record(finalErr)

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.True(t, logs[0].IsRetry)
	assert.Equal(t, 1, logs[0].ChannelId)
	assert.Contains(t, logs[0].Content, "gpt-4o is unavailable")
	assert.NotContains(t, logs[0].Content, "private-model-a")
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	assert.Equal(t, "private-model-a", other["upstream_model_name"])
	assert.Equal(t, "first-channel", other["channel_name"])
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Upstream temporarily unavailable", adminInfo["error_override_text"])

	userLogs, total, err := model.GetUserLogs(user.Id, model.LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	assert.Empty(t, userLogs)
	assert.Zero(t, total)
	tokenLogs, err := model.GetLogByTokenId(token.Id)
	require.NoError(t, err)
	assert.Empty(t, tokenLogs)
}
