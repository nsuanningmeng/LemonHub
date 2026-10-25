package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const rewardDistributionBatchSize = 100

// RewardDistributionState exists only for explicitly continuous campaigns. The
// source cursors cover the initial retained history once; ongoing writes arrive
// through durable source events, not a repeated scan or a maximum-ID watermark.
type RewardDistributionState struct {
	CampaignID           int    `gorm:"primaryKey"`
	Initialized          bool   `gorm:"not null;default:false"`
	BackfillEnd          int64  `gorm:"type:bigint;not null;default:0"`
	TopupUpperID         int64  `gorm:"type:bigint;not null;default:0"`
	TopupCursorID        int64  `gorm:"type:bigint;not null;default:0"`
	TopupDone            bool   `gorm:"not null;default:false"`
	UsageCursorAt        int64  `gorm:"type:bigint;not null;default:0"`
	UsageCursorID        int64  `gorm:"type:bigint;not null;default:0"`
	UsageCursorKey       string `gorm:"type:varchar(64);not null;default:''"`
	UsageCursorUserID    int    `gorm:"not null;default:0"`
	UsageDone            bool   `gorm:"not null;default:false"`
	CandidateCursorID    int64  `gorm:"type:bigint;not null;default:0"`
	NextCandidateCheckAt int64  `gorm:"type:bigint;not null;default:0"`
	UpdatedAt            int64  `gorm:"type:bigint;not null;default:0;index"`
}

// Candidates preserve evidence from the two sources independently, including
// half-complete AND audiences and temporarily disabled accounts. Granting is
// idempotent even if a source event or a backfill page is replayed after a crash.
type RewardDistributionCandidate struct {
	Id              int64 `gorm:"primaryKey"`
	CampaignID      int   `gorm:"not null;uniqueIndex:idx_reward_candidate_user,priority:1;index:idx_reward_candidate_pending,priority:1"`
	UserID          int   `gorm:"not null;uniqueIndex:idx_reward_candidate_user,priority:2"`
	HasTopup        bool  `gorm:"not null;default:false"`
	HasUsage        bool  `gorm:"not null;default:false"`
	TopupOccurredAt int64 `gorm:"type:bigint;not null;default:0"`
	UsageOccurredAt int64 `gorm:"type:bigint;not null;default:0"`
	GrantedAt       int64 `gorm:"type:bigint;not null;default:0;index:idx_reward_candidate_pending,priority:2"`
}

type RewardDistributionResult struct {
	EventsProcessed    int  `json:"events_processed"`
	CampaignsProcessed int  `json:"campaigns_processed"`
	More               bool `json:"more"`
}

func createContinuousRewardCampaign(input RewardCampaignInput, createdBy int) (*RewardCampaign, error) {
	campaign := &RewardCampaign{
		Title: input.Title, Description: input.Description, Type: input.Type,
		MinQuota: input.MinQuota, MaxQuota: input.MaxQuota, Audience: input.Audience,
		StartTime: input.StartTime, EndTime: input.EndTime, RequestID: input.RequestID,
		CreatedBy: createdBy, ExpiresInDays: input.ExpiresInDays,
		Continuous: true, CreatedAt: common.GetTimestamp(),
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(campaign).Error; err != nil {
			return err
		}
		return tx.Create(&RewardDistributionState{CampaignID: campaign.Id}).Error
	})
	if err != nil {
		var existing RewardCampaign
		if lookupErr := DB.Where("request_id = ?", input.RequestID).First(&existing).Error; lookupErr == nil {
			if !existing.matchesRequest(input, createdBy) {
				return nil, ErrRewardRequestConflict
			}
			items := []RewardCampaign{existing}
			if err := populateRewardDistributionStatuses(items); err != nil {
				return nil, err
			}
			return &items[0], nil
		}
		return nil, err
	}
	setRewardDistributionStatus(campaign, nil, common.GetTimestamp())
	return campaign, nil
}

func setRewardDistributionStatus(campaign *RewardCampaign, state *RewardDistributionState, now int64) {
	if !campaign.Continuous {
		campaign.DistributionStatus = "legacy"
		return
	}
	if now < campaign.StartTime {
		campaign.DistributionStatus = "scheduled"
		return
	}
	if state == nil || !state.Initialized || !state.TopupDone || !state.UsageDone {
		campaign.DistributionStatus = "backfilling"
		return
	}
	campaign.DistributionStatus = "active"
	if now >= campaign.EndTime {
		campaign.DistributionStatus = "completed"
	}
}

func populateRewardDistributionStatuses(campaigns []RewardCampaign) error {
	ids := make([]int, 0, len(campaigns))
	for _, campaign := range campaigns {
		if campaign.Continuous {
			ids = append(ids, campaign.Id)
		}
	}
	states := make([]RewardDistributionState, 0, len(ids))
	if len(ids) > 0 {
		if err := DB.Where("campaign_id IN ?", ids).Find(&states).Error; err != nil {
			return err
		}
	}
	byID := make(map[int]*RewardDistributionState, len(states))
	for i := range states {
		byID[states[i].CampaignID] = &states[i]
	}
	now := common.GetTimestamp()
	for i := range campaigns {
		setRewardDistributionStatus(&campaigns[i], byID[campaigns[i].Id], now)
	}
	return nil
}

func rewardDistributionLimit(limit int) int {
	if limit < 1 || limit > 1000 {
		return rewardDistributionBatchSize
	}
	return limit
}

// ProcessRewardActivityEvent records committed source evidence before attempting
// a grant. ACK belongs to the caller and must follow this durable main-DB work.
func ProcessRewardActivityEvent(ctx context.Context, event RewardActivityEvent) error {
	if !event.Confirmed || event.UserID <= 0 || (event.Kind != "topup" && event.Kind != "usage") {
		return nil
	}
	var afterID int
	for {
		var campaigns []RewardCampaign
		if err := DB.WithContext(ctx).Where("continuous = ? AND id > ? AND start_time <= ? AND end_time > ?", true, afterID, event.OccurredAt, event.OccurredAt).
			Where("audience IN ?", []string{event.Kind, "both", "either"}).
			Where("NOT EXISTS (SELECT 1 FROM user_rewards WHERE user_rewards.campaign_id = reward_campaigns.id AND user_rewards.user_id = ?)", event.UserID).Order("id").Limit(rewardDistributionBatchSize).Find(&campaigns).Error; err != nil {
			return err
		}
		for _, campaign := range campaigns {
			if err := recordRewardDistributionCandidate(ctx, campaign.Id, event.UserID, event.Kind, event.OccurredAt); err != nil {
				return err
			}
			afterID = campaign.Id
		}
		if len(campaigns) < rewardDistributionBatchSize {
			return nil
		}
	}
}

func recordRewardDistributionCandidate(ctx context.Context, campaignID, userID int, kind string, occurredAt int64) error {
	candidate := RewardDistributionCandidate{CampaignID: campaignID, UserID: userID}
	field := "has_usage"
	timeField := "usage_occurred_at"
	candidate.UsageOccurredAt = occurredAt
	candidate.HasUsage = kind == "usage"
	candidate.HasTopup = kind == "topup"
	if candidate.HasTopup {
		field = "has_topup"
		timeField = "topup_occurred_at"
		candidate.TopupOccurredAt = occurredAt
		candidate.UsageOccurredAt = 0
	}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "campaign_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			field:     true,
			timeField: gorm.Expr("CASE WHEN reward_distribution_candidates."+timeField+" = 0 OR reward_distribution_candidates."+timeField+" > ? THEN ? ELSE reward_distribution_candidates."+timeField+" END", occurredAt, occurredAt),
		}),
	}).Create(&candidate).Error; err != nil {
		return err
	}
	return grantRewardDistributionCandidate(ctx, campaignID, userID, common.GetTimestamp)
}

func grantRewardDistributionCandidate(ctx context.Context, campaignID, userID int, now func() int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidate RewardDistributionCandidate
		if err := lockForUpdate(tx).Where("campaign_id = ? AND user_id = ?", campaignID, userID).First(&candidate).Error; err != nil {
			return err
		}
		if candidate.GrantedAt != 0 {
			return nil
		}
		var campaign RewardCampaign
		if err := lockForUpdate(tx).Where("id = ?", campaignID).First(&campaign).Error; err != nil {
			return err
		}
		if !campaign.Continuous || now() < campaign.StartTime {
			return nil
		}
		if err := requireMainRewardUser(lockForUpdate(tx), userID); err != nil {
			if errors.Is(err, ErrRewardUserUnavailable) {
				return nil
			}
			return err
		}
		checkedAt := now()
		hasUsage := candidate.HasUsage && candidate.UsageOccurredAt <= checkedAt
		eligible := hasUsage
		if campaign.Audience == "topup" || campaign.Audience == "both" || campaign.Audience == "either" {
			hasTopup := false
			if candidate.HasTopup && candidate.TopupOccurredAt <= checkedAt {
				// Refunds can invalidate an earlier source event. Recheck the
				// authoritative order state immediately before awarding.
				var id int
				if err := tx.Model(&TopUp{}).Select("id").Where("user_id = ? AND site_id = ? AND status = ? AND clawed_back_quota = ? AND complete_time >= ? AND complete_time < ?", userID, 0, common.TopUpStatusSuccess, 0, campaign.StartTime, campaign.EndTime).Where("complete_time <= ?", checkedAt).
					Limit(1).Scan(&id).Error; err != nil {
					return err
				}
				hasTopup = id > 0
			}
			switch campaign.Audience {
			case "topup":
				eligible = hasTopup
			case "both":
				eligible = hasTopup && hasUsage
			case "either":
				eligible = hasTopup || hasUsage
			}
		}
		if !eligible {
			return nil
		}
		var existing UserReward
		lookup := tx.Where("campaign_id = ? AND user_id = ?", campaignID, userID).First(&existing)
		if lookup.Error == nil {
			return tx.Model(&candidate).Update("granted_at", existing.CreatedAt).Error
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		grantedAt := now()
		reward := UserReward{
			CampaignID: campaignID, UserID: userID, Title: campaign.Title, Description: campaign.Description,
			Type: campaign.Type, MinQuota: campaign.MinQuota, MaxQuota: campaign.MaxQuota,
			Status: RewardStatusPending, CreatedAt: grantedAt,
			ExpiresAt: grantedAt + int64(campaign.ExpiresInDays)*86400,
		}
		if err := tx.Create(&reward).Error; err != nil {
			return err
		}
		if err := tx.Model(&campaign).UpdateColumn("recipient_count", gorm.Expr("recipient_count + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&candidate).Update("granted_at", grantedAt).Error
	})
}

// RunRewardDistributionPass bounds work per source and campaign. All cursors and
// candidates survive process loss; the scheduler lease is only an optimization.
func RunRewardDistributionPass(ctx context.Context, batchSize int) (*RewardDistributionResult, error) {
	batchSize = rewardDistributionLimit(batchSize)
	result := &RewardDistributionResult{}
	var states []RewardDistributionState
	if err := DB.WithContext(ctx).
		Where("topup_done = ? OR usage_done = ? OR (next_candidate_check_at <= ? AND EXISTS (SELECT 1 FROM reward_distribution_candidates WHERE reward_distribution_candidates.campaign_id = reward_distribution_states.campaign_id AND reward_distribution_candidates.granted_at = 0))", false, false, common.GetTimestamp()).
		Order("updated_at, campaign_id").Limit(20).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if err := ProcessRewardCampaignBatch(ctx, state.CampaignID, batchSize); err != nil {
			return nil, err
		}
		result.CampaignsProcessed++
	}
	events, err := ListPendingRewardActivityEvents(ctx, batchSize)
	if err != nil {
		return nil, err
	}
	for _, item := range events {
		if err := ProcessRewardActivityEvent(ctx, item.Event); err != nil {
			return nil, err
		}
		if err := AckRewardActivityEvent(ctx, item); err != nil {
			return nil, err
		}
		result.EventsProcessed++
	}
	result.More = len(events) > 0
	if !result.More {
		var pending struct{ CampaignID int }
		err := DB.WithContext(ctx).Model(&RewardDistributionState{}).Select("reward_distribution_states.campaign_id").
			Joins("JOIN reward_campaigns ON reward_campaigns.id = reward_distribution_states.campaign_id").
			Where("reward_campaigns.continuous = ? AND reward_campaigns.start_time <= ? AND (topup_done = ? OR usage_done = ? OR (next_candidate_check_at <= ? AND candidate_cursor_id > 0 AND EXISTS (SELECT 1 FROM reward_distribution_candidates WHERE reward_distribution_candidates.campaign_id = reward_distribution_states.campaign_id AND reward_distribution_candidates.granted_at = 0 AND reward_distribution_candidates.id > reward_distribution_states.candidate_cursor_id)))", true, common.GetTimestamp(), false, false, common.GetTimestamp()).Limit(1).Scan(&pending).Error
		if err != nil {
			return nil, err
		}
		result.More = pending.CampaignID > 0
	}
	return result, nil
}

func ProcessRewardCampaignBatch(ctx context.Context, campaignID, batchSize int) error {
	batchSize = rewardDistributionLimit(batchSize)
	var campaign RewardCampaign
	if err := DB.WithContext(ctx).Where("id = ? AND continuous = ?", campaignID, true).First(&campaign).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var state RewardDistributionState
	if err := DB.WithContext(ctx).Where("campaign_id = ?", campaignID).First(&state).Error; err != nil {
		return err
	}
	if common.GetTimestamp() < campaign.StartTime {
		return DB.WithContext(ctx).Model(&state).Update("updated_at", time.Now().UnixNano()).Error
	}
	if !state.Initialized {
		var maximum struct{ ID int64 }
		if err := DB.WithContext(ctx).Model(&TopUp{}).Select("COALESCE(MAX(id), 0) AS id").Scan(&maximum).Error; err != nil {
			return err
		}
		end := min(campaign.EndTime, common.GetTimestamp()+1)
		updates := map[string]interface{}{
			"initialized": true, "backfill_end": end, "topup_upper_id": maximum.ID,
			"topup_done":      campaign.Audience == "usage" || maximum.ID == 0,
			"usage_done":      campaign.Audience == "topup" || end <= campaign.StartTime,
			"usage_cursor_at": campaign.StartTime,
		}
		if err := DB.WithContext(ctx).Model(&state).Where("initialized = ?", false).Updates(updates).Error; err != nil {
			return err
		}
		if err := DB.WithContext(ctx).First(&state, "campaign_id = ?", campaignID).Error; err != nil {
			return err
		}
	}
	if !state.TopupDone {
		if err := backfillRewardTopups(ctx, campaign, state, batchSize); err != nil {
			return err
		}
	}
	if !state.UsageDone {
		if err := backfillRewardUsage(ctx, campaign, state, batchSize); err != nil {
			return err
		}
	}
	if state.NextCandidateCheckAt <= common.GetTimestamp() {
		if err := retryRewardDistributionCandidates(ctx, state, batchSize); err != nil {
			return err
		}
	}
	return DB.WithContext(ctx).Model(&state).Update("updated_at", time.Now().UnixNano()).Error
}

// RewardDistributionReady is enabled only after all production source stores
// have been registered during startup; isolated legacy fixtures remain inert.
func RewardDistributionReady() bool {
	return DB != nil && rewardActivityCaptureEnabled(DB)
}
