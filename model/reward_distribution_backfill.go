package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

// Top-ups have no complete_time index in old installations. Traverse the
// existing primary key once with a persisted upper bound, selecting narrow rows
// before filtering in Go. This bounds every batch without an automatic large
// table index build. Subsequent completion of an old order is a durable event.
func backfillRewardTopups(ctx context.Context, campaign RewardCampaign, state RewardDistributionState, limit int) error {
	var rows []TopUp
	if err := DB.WithContext(ctx).Select("id, user_id, site_id, status, clawed_back_quota, complete_time").
		Where("id > ? AND id <= ?", state.TopupCursorID, state.TopupUpperID).
		Order("id").Limit(limit).Find(&rows).Error; err != nil {
		return err
	}
	cursor := state.TopupCursorID
	seen := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if row.SiteId == 0 && row.UserId > 0 && row.Status == common.TopUpStatusSuccess && row.ClawedBackQuota == 0 && row.CompleteTime >= campaign.StartTime && row.CompleteTime < state.BackfillEnd {
			if _, exists := seen[row.UserId]; !exists {
				if err := recordRewardDistributionCandidate(ctx, campaign.Id, row.UserId, "topup", row.CompleteTime); err != nil {
					return err
				}
				seen[row.UserId] = struct{}{}
			}
		}
		cursor = int64(row.Id)
	}
	return DB.WithContext(ctx).Model(&RewardDistributionState{}).
		Where("campaign_id = ? AND topup_cursor_id = ? AND topup_done = ?", campaign.Id, state.TopupCursorID, false).
		Updates(map[string]interface{}{"topup_cursor_id": cursor, "topup_done": len(rows) < limit || cursor >= state.TopupUpperID}).Error
}

func backfillRewardUsage(ctx context.Context, campaign RewardCampaign, state RewardDistributionState, limit int) error {
	var rows []Log
	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select("id, user_id, site_id, type, created_at, request_id").
		Where("created_at >= ? AND created_at < ?", campaign.StartTime, state.BackfillEnd)
	clickHouse := common.UsingLogDatabase(common.DatabaseTypeClickHouse)
	if clickHouse {
		// ClickHouse stores logs in (created_at, request_id) order, without a
		// durable numeric ID. The user tie-breaker avoids dropping distinct
		// recipients with imported equal request IDs; duplicate evidence for
		// the same user is intentionally harmless.
		query = query.Where("site_id = ? AND type = ?", 0, LogTypeConsume).
			Where("created_at > ? OR (created_at = ? AND request_id > ?) OR (created_at = ? AND request_id = ? AND user_id > ?)",
				state.UsageCursorAt, state.UsageCursorAt, state.UsageCursorKey, state.UsageCursorAt, state.UsageCursorKey, state.UsageCursorUserID).
			Order("created_at, request_id, user_id")
	} else {
		// Do not filter errors/sites before LIMIT: each indexed page has a
		// bounded row count even in a period dominated by other log types.
		query = query.Where("created_at > ? OR (created_at = ? AND id > ?)", state.UsageCursorAt, state.UsageCursorAt, state.UsageCursorID).
			Order("created_at, id")
	}
	if err := query.Limit(limit).Find(&rows).Error; err != nil {
		return err
	}
	seen := make(map[int]struct{}, len(rows))
	at, id, key, userID := state.UsageCursorAt, state.UsageCursorID, state.UsageCursorKey, state.UsageCursorUserID
	for _, row := range rows {
		if row.SiteId == 0 && row.Type == LogTypeConsume && row.UserId > 0 {
			if _, exists := seen[row.UserId]; !exists {
				if err := recordRewardDistributionCandidate(ctx, campaign.Id, row.UserId, "usage", row.CreatedAt); err != nil {
					return err
				}
				seen[row.UserId] = struct{}{}
			}
		}
		at, id, key, userID = row.CreatedAt, int64(row.Id), row.RequestId, row.UserId
	}
	return DB.WithContext(ctx).Model(&RewardDistributionState{}).
		Where("campaign_id = ? AND usage_cursor_at = ? AND usage_cursor_id = ? AND usage_cursor_key = ? AND usage_cursor_user_id = ? AND usage_done = ?", campaign.Id, state.UsageCursorAt, state.UsageCursorID, state.UsageCursorKey, state.UsageCursorUserID, false).
		Updates(map[string]interface{}{"usage_cursor_at": at, "usage_cursor_id": id, "usage_cursor_key": key, "usage_cursor_user_id": userID, "usage_done": len(rows) < limit}).Error
}

func retryRewardDistributionCandidates(ctx context.Context, state RewardDistributionState, limit int) error {
	var candidates []RewardDistributionCandidate
	if err := DB.WithContext(ctx).Where("campaign_id = ? AND granted_at = ? AND id > ?", state.CampaignID, 0, state.CandidateCursorID).
		Order("id").Limit(limit).Find(&candidates).Error; err != nil {
		return err
	}
	cursor := int64(0)
	for _, candidate := range candidates {
		if err := grantRewardDistributionCandidate(ctx, state.CampaignID, candidate.UserID, common.GetTimestamp); err != nil {
			return err
		}
		cursor = candidate.Id
	}
	nextCheck := int64(0)
	if len(candidates) < limit {
		cursor = 0
		// Missing AND evidence and disabled accounts are durable but cannot
		// improve by tight-looping while unrelated source events are drained.
		// New source evidence still attempts a grant immediately.
		nextCheck = common.GetTimestamp() + 15
	}
	return DB.WithContext(ctx).Model(&RewardDistributionState{}).
		Where("campaign_id = ? AND candidate_cursor_id = ?", state.CampaignID, state.CandidateCursorID).
		Updates(map[string]interface{}{"candidate_cursor_id": cursor, "next_candidate_check_at": nextCheck}).Error
}
