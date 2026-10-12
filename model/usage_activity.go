package model

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type UsageActivitySummary struct {
	TotalTokens           int64 `json:"total_tokens"`
	PeakDailyTokens       int64 `json:"peak_daily_tokens"`
	CurrentStreakDays     int   `json:"current_streak_days"`
	LongestStreakDays     int   `json:"longest_streak_days"`
	LongestRequestSeconds int64 `json:"longest_request_seconds"`
}

type UsageActivityDay struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens"`
}

type UsageActivity struct {
	Timezone   string               `json:"timezone"`
	AsOf       string               `json:"as_of"`
	Summary    UsageActivitySummary `json:"summary"`
	DailyUsage []UsageActivityDay   `json:"daily_usage"`
}

// GetUserUsageActivity summarizes retained consume logs, including cached input
// when the normalized input count is available. Zero-token requests contribute
// to the longest request duration, but do not make a day active.
func GetUserUsageActivity(ctx context.Context, userID int, location *time.Location, now time.Time) (*UsageActivity, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user identity")
	}
	if location == nil {
		location = time.UTC
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	activity := &UsageActivity{
		Timezone:   location.String(),
		AsOf:       now.In(location).Format(time.RFC3339),
		DailyUsage: make([]UsageActivityDay, 0),
	}

	// Stream only the required columns. Aggregating civil dates in Go keeps IANA
	// time zone and DST behavior identical across all supported log databases,
	// without requiring database time zone tables or retaining individual logs.
	rows, err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select("created_at, prompt_tokens, input_tokens_total, completion_tokens, use_time, other").
		Where("user_id = ? AND type = ? AND created_at <= ?", userID, LogTypeConsume, now.Unix()).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dailyTokens := make(map[string]int64)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var createdAt int64
		var prompt, input, output, useTime sql.NullInt64
		var other sql.NullString
		if err := rows.Scan(&createdAt, &prompt, &input, &output, &useTime, &other); err != nil {
			return nil, err
		}
		inputTokens := prompt.Int64
		if input.Valid {
			inputTokens = input.Int64
		}
		tokens := common.SumTokenCountsForStatistics(inputTokens, output.Int64)
		if useTime.Int64 > activity.Summary.LongestRequestSeconds {
			// Failed billable streams and violation fees also use consume logs.
			// Their tokens remain usage, but their duration is not a successful
			// request. Legacy logs without outcome metadata retain consume semantics.
			var outcome struct {
				ViolationFee bool `json:"violation_fee"`
				StreamStatus struct {
					Status string `json:"status"`
				} `json:"stream_status"`
			}
			_ = common.UnmarshalJsonStr(other.String, &outcome)
			if !outcome.ViolationFee && outcome.StreamStatus.Status != "error" {
				activity.Summary.LongestRequestSeconds = useTime.Int64
			}
		}
		if tokens == 0 {
			continue
		}
		date := time.Unix(createdAt, 0).In(location).Format(time.DateOnly)
		dailyTokens[date] = common.SumTokenCountsForStatistics(dailyTokens[date], tokens)
		activity.Summary.TotalTokens = common.SumTokenCountsForStatistics(activity.Summary.TotalTokens, tokens)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for date, tokens := range dailyTokens {
		activity.DailyUsage = append(activity.DailyUsage, UsageActivityDay{Date: date, Tokens: tokens})
		if tokens > activity.Summary.PeakDailyTokens {
			activity.Summary.PeakDailyTokens = tokens
		}
	}
	sort.Slice(activity.DailyUsage, func(i, j int) bool {
		return activity.DailyUsage[i].Date < activity.DailyUsage[j].Date
	})

	// Compare civil dates in UTC so 23/25-hour DST days remain consecutive.
	var previousDay time.Time
	streak := 0
	for _, day := range activity.DailyUsage {
		date, err := time.Parse(time.DateOnly, day.Date)
		if err != nil {
			return nil, err
		}
		if date.Equal(previousDay.AddDate(0, 0, 1)) {
			streak++
		} else {
			streak = 1
		}
		if streak > activity.Summary.LongestStreakDays {
			activity.Summary.LongestStreakDays = streak
		}
		previousDay = date
	}
	year, month, day := now.In(location).Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if previousDay.Equal(today) || previousDay.Equal(today.AddDate(0, 0, -1)) {
		activity.Summary.CurrentStreakDays = streak
	}
	return activity, nil
}
