package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PerfMetric stores aggregated relay performance metrics for the model square.
type PerfMetric struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	ChannelID      int    `json:"-" gorm:"uniqueIndex:idx_channel_perf_identity_bucket,priority:1;index:idx_channel_perf_lookup,priority:1"`
	ModelName      string `json:"model_name" gorm:"size:128;index:idx_channel_perf_lookup,priority:2"`
	Group          string `json:"group" gorm:"column:group;size:64;index:idx_channel_perf_lookup,priority:3"`
	IdentityHash   string `json:"-" gorm:"size:64;uniqueIndex:idx_channel_perf_identity_bucket,priority:2"`
	BucketTs       int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_channel_perf_identity_bucket,priority:3;index:idx_channel_perf_lookup,priority:4;index:idx_channel_perf_bucket_ts"`
	RequestCount   int64  `json:"-" gorm:"default:0"`
	SuccessCount   int64  `json:"-" gorm:"default:0"`
	TotalLatencyMs int64  `json:"-" gorm:"default:0"`
	TtftSumMs      int64  `json:"-" gorm:"default:0"`
	TtftCount      int64  `json:"-" gorm:"default:0"`
	OutputTokens   int64  `json:"-" gorm:"default:0"`
	GenerationMs   int64  `json:"-" gorm:"default:0"`
}

func (PerfMetric) TableName() string {
	// Legacy perf_metrics rows have no channel identity and cannot be filtered
	// after a channel is disabled. Keep that table intact and start scoped data here.
	return "channel_perf_metrics"
}

func perfMetricIdentityHash(modelName, group string) (string, error) {
	identity, err := common.Marshal([2]string{modelName, group})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:]), nil
}

func (metric *PerfMetric) BeforeCreate(_ *gorm.DB) error {
	identityHash, err := perfMetricIdentityHash(metric.ModelName, metric.Group)
	if err != nil {
		return err
	}
	metric.IdentityHash = identityHash
	return nil
}

func UpsertPerfMetric(metric *PerfMetric) error {
	if metric == nil || metric.ChannelID <= 0 || metric.RequestCount == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "identity_hash"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count":    gorm.Expr("channel_perf_metrics.request_count + ?", metric.RequestCount),
			"success_count":    gorm.Expr("channel_perf_metrics.success_count + ?", metric.SuccessCount),
			"total_latency_ms": gorm.Expr("channel_perf_metrics.total_latency_ms + ?", metric.TotalLatencyMs),
			"ttft_sum_ms":      gorm.Expr("channel_perf_metrics.ttft_sum_ms + ?", metric.TtftSumMs),
			"ttft_count":       gorm.Expr("channel_perf_metrics.ttft_count + ?", metric.TtftCount),
			"output_tokens":    gorm.Expr("channel_perf_metrics.output_tokens + ?", metric.OutputTokens),
			"generation_ms":    gorm.Expr("channel_perf_metrics.generation_ms + ?", metric.GenerationMs),
		}),
	}).Create(metric).Error
}

// Read current database state rather than routing caches so direct callers see
// manual disables/deletions immediately. HTTP response caching can delay their
// effect on historical and in-flight performance samples by up to 30 seconds.
// Auto-disabled failures remain visible while their channels await recovery.
func GetPerfMetricsEligibleChannelIDs() ([]int, error) {
	var channelIDs []int
	err := DB.Model(&Channel{}).
		Where("status IN ?", []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled}).
		Pluck("id", &channelIDs).Error
	return channelIDs, err
}

// Permission keys are byte-exact strings. MySQL's default collation can merge
// different models/groups, so use binary identities when filtering or grouping.
// Convert to UTF-8 before comparing bytes so legacy GBK/Big5 columns also match
// the UTF-8 permission keys and return correctly encoded model/group names.
// Keep ordinary indexed predicates alongside the exact MySQL predicates.
func perfMetricIdentityColumns() (string, string) {
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		return "CAST(CONVERT(model_name USING utf8mb4) AS BINARY)", "CAST(CONVERT(" + commonGroupCol + " USING utf8mb4) AS BINARY)"
	}
	return "model_name", commonGroupCol
}

func GetPerfMetrics(modelName string, group string, startTs int64, endTs int64, channelIDs []int) ([]PerfMetric, error) {
	var metrics []PerfMetric
	modelColumn, groupColumn := perfMetricIdentityColumns()
	query := DB.Model(&PerfMetric{}).
		Where("model_name = ? AND bucket_ts >= ? AND bucket_ts <= ?", modelName, startTs, endTs).
		Where("channel_id IN ?", channelIDs)
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		// Retain the indexed predicate above; this additionally enforces exact identity.
		query = query.Where(modelColumn+" = ?", modelName)
	}
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			query = query.Where(groupColumn+" = ?", group)
		}
	}
	err := query.Order("bucket_ts ASC").Find(&metrics).Error
	return metrics, err
}

type PerfMetricSummary struct {
	ModelName      string `json:"model_name"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
}

type PerfMetricSummaryBucket struct {
	ModelName      string `json:"model_name"`
	Group          string `json:"-"`
	BucketTs       int64  `json:"bucket_ts"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
}

// PerfMetricGroupSummary keeps raw totals so callers can combine persisted and
// in-memory samples without averaging averages.
type PerfMetricGroupSummary struct {
	ModelName      string
	Group          string
	RequestCount   int64
	SuccessCount   int64
	TotalLatencyMs int64
	TtftSumMs      int64
	TtftCount      int64
	OutputTokens   int64
	GenerationMs   int64
}

func GetPerfMetricsByModelGroup(startTs, endTs int64, modelNames, groups []string, channelIDs []int) ([]PerfMetricGroupSummary, error) {
	summaries := make([]PerfMetricGroupSummary, 0)
	if len(modelNames) == 0 || len(groups) == 0 || len(channelIDs) == 0 {
		return summaries, nil
	}
	modelColumn, groupColumn := perfMetricIdentityColumns()
	query := DB.Model(&PerfMetric{}).
		Select(modelColumn+" AS model_name, "+groupColumn+" AS "+commonGroupCol+", SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(ttft_sum_ms) as ttft_sum_ms, SUM(ttft_count) as ttft_count, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Where("model_name IN ?", modelNames).
		Where(commonGroupCol+" IN ?", groups).
		Where("channel_id IN ?", channelIDs)
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		query = query.Where(modelColumn+" IN ?", modelNames).Where(groupColumn+" IN ?", groups)
	}
	err := query.
		Group(modelColumn + ", " + groupColumn).
		Having("SUM(request_count) > 0").
		Find(&summaries).Error
	return summaries, err
}

func GetPerfMetricsSummaryAll(startTs int64, endTs int64, groups []string, channelIDs []int) ([]PerfMetricSummary, error) {
	var summaries []PerfMetricSummary
	modelColumn, groupColumn := perfMetricIdentityColumns()
	query := DB.Model(&PerfMetric{}).
		Select(modelColumn+" AS model_name, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Where("channel_id IN ?", channelIDs)
	if groups != nil {
		if len(groups) == 0 {
			return summaries, nil
		}
		query = query.Where(commonGroupCol+" IN ?", groups)
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			query = query.Where(groupColumn+" IN ?", groups)
		}
	}
	err := query.
		Group(modelColumn).
		Having("SUM(request_count) > 0").
		Find(&summaries).Error
	return summaries, err
}

// Keep group identity so callers can exclude retired model-group memberships
// before combining buckets into model-wide success rates and recent history.
func GetPerfMetricsSummaryBucketsAll(startTs int64, endTs int64, groups []string, channelIDs []int) ([]PerfMetricSummaryBucket, error) {
	var summaries []PerfMetricSummaryBucket
	modelColumn, groupColumn := perfMetricIdentityColumns()
	query := DB.Model(&PerfMetric{}).
		Select(modelColumn+" AS model_name, "+groupColumn+" AS "+commonGroupCol+", bucket_ts, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Where("channel_id IN ?", channelIDs)
	if groups != nil {
		if len(groups) == 0 {
			return summaries, nil
		}
		query = query.Where(commonGroupCol+" IN ?", groups)
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			query = query.Where(groupColumn+" IN ?", groups)
		}
	}
	err := query.
		Group(modelColumn + ", " + groupColumn + ", bucket_ts").
		Having("SUM(request_count) > 0").
		Order("bucket_ts ASC").
		Find(&summaries).Error
	return summaries, err
}

func DeletePerfMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Where("bucket_ts < ?", cutoffTs).Delete(&PerfMetric{}).Error
}

func PerfMetricStartTime(hours int) int64 {
	if hours <= 0 {
		hours = 24
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}
