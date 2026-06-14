package perf_metrics_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type PerfMetricsSetting struct {
	Enabled       bool   `json:"enabled"`
	FlushInterval int    `json:"flush_interval"`
	BucketTime    string `json:"bucket_time"`
	RetentionDays int    `json:"retention_days"`

	// SuccessRateGreenThreshold: success rate >= this value renders green (healthy).
	SuccessRateGreenThreshold float64 `json:"success_rate_green_threshold"`
	// SuccessRateYellowThreshold: success rate >= this value (but below green) renders
	// yellow (warning); below this value renders red (critical).
	SuccessRateYellowThreshold float64 `json:"success_rate_yellow_threshold"`
	// ErrorCodeWhitelist: HTTP status codes (single codes and ranges, e.g.
	// "429,500,499-520") that count as failures lowering the success rate. When
	// empty, every failed relay counts (legacy behavior). Failed relays whose
	// status code is NOT whitelisted are ignored and do not affect the metric.
	ErrorCodeWhitelist string `json:"error_code_whitelist"`
	// NoDataAsFull: when true, a model with no requests in the window is treated
	// as 100% success (green) on the frontend instead of "no data".
	NoDataAsFull bool `json:"no_data_as_full"`
}

var perfMetricsSetting = PerfMetricsSetting{
	Enabled:       true,
	FlushInterval: 5,
	BucketTime:    "hour",
	RetentionDays: 0,

	SuccessRateGreenThreshold:  99.9,
	SuccessRateYellowThreshold: 99,
	ErrorCodeWhitelist:         "",
	NoDataAsFull:               true,
}

func init() {
	config.GlobalConfig.Register("perf_metrics_setting", &perfMetricsSetting)
}

func GetSetting() PerfMetricsSetting {
	return perfMetricsSetting
}

// ShouldCountErrorAsFailure reports whether a failed relay with the given HTTP
// status code should be counted as a failure for model success-rate metrics.
//
// An empty whitelist means "count every error" (legacy behavior). Otherwise only
// status codes inside the configured whitelist ranges are counted; other failed
// relays are not recorded and therefore do not lower the success rate. On an
// invalid/unparsable whitelist it fails open (counts the error) so a typo never
// silently hides outages.
func ShouldCountErrorAsFailure(statusCode int) bool {
	raw := strings.TrimSpace(perfMetricsSetting.ErrorCodeWhitelist)
	if raw == "" {
		return true
	}
	ranges, err := operation_setting.ParseHTTPStatusCodeRanges(raw)
	if err != nil || len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if statusCode >= r.Start && statusCode <= r.End {
			return true
		}
	}
	return false
}

func GetBucketSeconds() int64 {
	switch perfMetricsSetting.BucketTime {
	case "minute":
		return 60
	case "5min":
		return 300
	case "hour":
		return 3600
	default:
		return 3600
	}
}

func GetFlushIntervalMinutes() int {
	if perfMetricsSetting.FlushInterval < 1 {
		return 1
	}
	return perfMetricsSetting.FlushInterval
}
