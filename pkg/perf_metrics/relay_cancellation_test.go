package perfmetrics

import (
	"fmt"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch10CancellationExcludedFromBothMetricCounts(t *testing.T) {
	setupPerfSummaryTest(t)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1}, OriginModelName: "cancel-model", UsingGroup: "default", StartTime: time.Now()}
	RecordRelaySample(info, true, 5)
	RecordRelaySample(info, false, 0)
	info.MarkDownstreamCancelled()
	RecordRelaySample(info, true, 2)
	RecordRelaySample(info, false, 0)
	result, err := QuerySummaryAll(24, map[string][]string{"cancel-model": {"default"}})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.Equal(t, int64(2), result.Models[0].RequestCount)
	assert.InDelta(t, 50, result.Models[0].SuccessRate, 0.001)
	info.MarkUpstreamFailure()
	RecordRelaySample(info, false, 0)
	info.ResetStreamOutcome()
	info.MarkUpstreamCompleted()
	info.MarkDownstreamCancelled()
	RecordRelaySample(info, true, 1)
	result, err = QuerySummaryAll(24, map[string][]string{"cancel-model": {"default"}})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.Equal(t, int64(4), result.Models[0].RequestCount)
	assert.InDelta(t, 50, result.Models[0].SuccessRate, 0.001)
}

func TestBatch10NativeFailureMetricsRetainOperatorWhitelist(t *testing.T) {
	setupPerfSummaryTest(t)
	original := perf_metrics_setting.GetSetting().ErrorCodeWhitelist
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.error_code_whitelist": original}))
	})
	for i, whitelist := range []string{"", "500", "503"} {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.error_code_whitelist": whitelist}))
		name := fmt.Sprintf("native-failure-%d", i)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1}, OriginModelName: name, UsingGroup: "default", StartTime: time.Now()}
		info.MarkUpstreamFailureStatus(503)
		info.MarkDownstreamCancelled()
		RecordRelaySample(info, true, 3) // Native failed-stream billing keeps its existing settlement path.
		result, err := QuerySummaryAll(24, map[string][]string{name: {"default"}})
		require.NoError(t, err)
		require.Len(t, result.Models, 1)
		assert.Equal(t, int64(1), result.Models[0].RequestCount)
		want := float64(0)
		if whitelist == "500" {
			want = 100
		}
		assert.InDelta(t, want, result.Models[0].SuccessRate, 0.001)
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1}, OriginModelName: "unknown-failure", UsingGroup: "default", StartTime: time.Now()}
	info.MarkUpstreamFailure()
	assert.True(t, BuildRelaySample(info, true, 0, time.Now()).Success) // No status means no guessed policy override.
	info.ResetStreamOutcome()
	info.MarkOtherUpstreamTerminal()
	info.MarkDownstreamCancelled()
	assert.Equal(t, "unknown-failure", BuildRelaySample(info, true, 0, time.Now()).Model)
}
