package perfmetrics

import (
	"sort"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

type GroupMetrics struct {
	Group        string   `json:"group"`
	HasData      bool     `json:"has_data"`
	SuccessRate  *float64 `json:"success_rate"`
	AvgLatencyMs *int64   `json:"avg_latency_ms"`
	AvgTtftMs    *int64   `json:"avg_ttft_ms"`
	AvgTps       *float64 `json:"avg_tps"`
}

type ModelGroupMetrics struct {
	ModelName string         `json:"model_name"`
	Groups    []GroupMetrics `json:"groups"`
}

type ModelsResult struct {
	Hours         int                 `json:"hours"`
	StartTime     int64               `json:"start_time"`
	EndTime       int64               `json:"end_time"`
	BucketSeconds int64               `json:"bucket_seconds"`
	Models        []ModelGroupMetrics `json:"models"`
}

// QueryModels returns health metrics for the supplied visible model/group pairs.
// It includes pairs without samples, using null metrics to distinguish missing
// measurements from measured zero values.
func QueryModels(hours int, modelGroups map[string][]string) (ModelsResult, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	endTs := time.Now().Unix()
	result := ModelsResult{
		Hours: hours,
		// Match Query and QuerySummaryAll: an oldest bucket that starts before
		// the requested window is excluded rather than partially counted.
		StartTime:     endTs - int64(hours)*3600,
		EndTime:       endTs,
		BucketSeconds: perf_metrics_setting.GetBucketSeconds(),
		Models:        make([]ModelGroupMetrics, 0),
	}
	allowed := make(map[string]map[string]struct{}, len(modelGroups))
	modelNames := make([]string, 0, len(modelGroups))
	groupSet := make(map[string]struct{})
	for modelName, groups := range modelGroups {
		if len(groups) == 0 {
			continue
		}
		modelNames = append(modelNames, modelName)
		allowed[modelName] = allowedGroupSet(groups)
		for _, group := range groups {
			groupSet[group] = struct{}{}
		}
	}
	if len(modelNames) == 0 {
		return result, nil
	}
	sort.Strings(modelNames)
	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}

	channelIDs, err := model.GetPerfMetricsEligibleChannelIDs()
	if err != nil {
		return ModelsResult{}, err
	}
	allowedChannels := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		allowedChannels[channelID] = struct{}{}
	}
	rows, hot, err := queryWithHotSnapshot(func() ([]model.PerfMetricGroupSummary, error) {
		return model.GetPerfMetricsByModelGroup(result.StartTime, result.EndTime, modelNames, groups, channelIDs)
	}, func(k bucketKey) bool {
		_, channelAllowed := allowedChannels[k.channelID]
		_, groupAllowed := allowed[k.model][k.group]
		return channelAllowed && groupAllowed && k.bucketTs >= result.StartTime && k.bucketTs <= result.EndTime
	})
	if err != nil {
		return ModelsResult{}, err
	}
	totals := make(map[bucketKey]counters)
	for _, row := range rows {
		if _, ok := allowed[row.ModelName][row.Group]; !ok {
			continue
		}
		key := bucketKey{model: row.ModelName, group: row.Group}
		mergeCounters(totals, key, counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			ttftSumMs:      row.TtftSumMs,
			ttftCount:      row.TtftCount,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		})
	}
	for k, snapshot := range hot {
		pairKey := bucketKey{model: k.model, group: k.group}
		mergeCounters(totals, pairKey, snapshot)
	}

	for _, modelName := range modelNames {
		groups := make([]string, 0, len(allowed[modelName]))
		for group := range allowed[modelName] {
			groups = append(groups, group)
		}
		sort.Strings(groups)
		entry := ModelGroupMetrics{ModelName: modelName, Groups: make([]GroupMetrics, 0, len(groups))}
		for _, group := range groups {
			key := bucketKey{model: modelName, group: group}
			total := totals[key]
			metrics := GroupMetrics{Group: group, HasData: total.requestCount > 0}
			if metrics.HasData {
				rate := successRate(total)
				latency := avg(total.totalLatencyMs, total.requestCount)
				metrics.SuccessRate, metrics.AvgLatencyMs = &rate, &latency
				if total.ttftCount > 0 {
					ttft := avg(total.ttftSumMs, total.ttftCount)
					metrics.AvgTtftMs = &ttft
				}
				if total.outputTokens > 0 && total.generationMs > 0 {
					tps := avgTps(total)
					metrics.AvgTps = &tps
				}
			}
			entry.Groups = append(entry.Groups, metrics)
		}
		result.Models = append(result.Models, entry)
	}
	return result, nil
}
