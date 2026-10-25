package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupContinuousRewardTestDB(t *testing.T) {
	t.Helper()
	setupRewardTestDB(t)
	require.NoError(t, DB.AutoMigrate(&RewardDistributionState{}, &RewardDistributionCandidate{}))
	require.NoError(t, MigrateRewardActivityEvents(DB))
	require.NoError(t, MigrateRewardActivityEvents(LOG_DB))
}

func drainRewardDistributionForTest(t *testing.T, batchSize int) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		result, err := RunRewardDistributionPass(context.Background(), batchSize)
		require.NoError(t, err)
		if !result.More {
			return
		}
	}
	t.Fatal("reward backfill or confirmed events did not drain within bounded fixture work")
}

func TestContinuousRewardRegistrationAllowsEmptyAudienceWithoutPreview(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	now := common.GetTimestamp()
	input := rewardTestCampaignInput("empty-continuous")
	input.Continuous, input.StartTime, input.EndTime = true, now+3600, now+7200
	stalePreview := 100001
	input.ExpectedRecipientCount = &stalePreview
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	assert.True(t, campaign.Continuous)
	assert.Zero(t, campaign.RecipientCount)
	assert.Zero(t, campaign.ExpiresAt)
	assert.Equal(t, "scheduled", campaign.DistributionStatus)
	drainRewardDistributionForTest(t, 2)
	var state RewardDistributionState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.Id).Error)
	assert.False(t, state.Initialized, "a future campaign must wait instead of scanning history early")
	retried, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	assert.Equal(t, campaign.Id, retried.Id)
	assert.Equal(t, campaign.CreatedAt, retried.CreatedAt)
}

func TestContinuousRewardBackfillResumesAllBatchesAndPreservesWalletUntilClaim(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	for i := 0; i < 7; i++ {
		user := createReserveTestUser(t, 150)
		require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, Type: LogTypeConsume, CreatedAt: 150}).Error)
	}
	input := rewardTestCampaignInput("batched-backfill")
	input.Continuous = true
	input.ExpectedRecipientCount = nil
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	require.NoError(t, ProcessRewardCampaignBatch(context.Background(), campaign.Id, 2))
	var state RewardDistributionState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.Id).Error)
	assert.False(t, state.UsageDone)
	assert.Positive(t, state.UsageCursorID)
	drainRewardDistributionForTest(t, 2)
	var rewards []UserReward
	require.NoError(t, DB.Where("campaign_id = ?", campaign.Id).Find(&rewards).Error)
	require.Len(t, rewards, 7)
	for _, reward := range rewards {
		assert.Equal(t, reward.CreatedAt+7*86400, reward.ExpiresAt)
		assert.Equal(t, RewardStatusPending, reward.Status)
		var user User
		require.NoError(t, DB.First(&user, reward.UserID).Error)
		assert.Equal(t, 150, user.Quota)
	}
	campaigns, _, err := ListRewardCampaigns(1, 20)
	require.NoError(t, err)
	require.Len(t, campaigns, 1)
	assert.Equal(t, 7, campaigns[0].RecipientCount)
	assert.Equal(t, "completed", campaigns[0].DistributionStatus)
	retried, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	assert.Equal(t, "completed", retried.DistributionStatus)
	drainRewardDistributionForTest(t, 2)
	var count int64
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 7, count)
}

func TestContinuousRewardAndAudienceWaitsForSecondActivityAndRechecksRefunds(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	input := rewardTestCampaignInput("and-events")
	input.Continuous, input.Audience = true, "both"
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	order := TopUp{UserId: user.Id, TradeNo: "first", Status: common.TopUpStatusSuccess, CompleteTime: 120}
	require.NoError(t, DB.Create(&order).Error)
	topup := RewardActivityEvent{Kind: "topup", UserID: user.Id, OccurredAt: 120, Confirmed: true}
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), topup))
	var count int64
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&order).Update("clawed_back_quota", 1).Error)
	usage := RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: 150, Confirmed: true}
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), usage))
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.Zero(t, count, "a refunded order cannot satisfy AND when the second event arrives")
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, TradeNo: "second", Status: common.TopUpStatusSuccess, CompleteTime: 170}).Error)
	topup.OccurredAt = 170
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), topup))
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), usage))
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), topup))
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, 1, campaign.RecipientCount)
}

func TestContinuousRewardEventsRespectHalfOpenRangeAndLegacyIsolation(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	input := rewardTestCampaignInput("boundary-events")
	input.Continuous = true
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	legacy := RewardCampaign{Title: "Real old reward", Type: RewardTypeFixed, MinQuota: 10, MaxQuota: 10, Audience: "usage", StartTime: 100, EndTime: common.GetTimestamp() + 86400, RequestID: "legacy"}
	require.NoError(t, DB.Create(&legacy).Error)
	for _, timestamp := range []int64{99, 100, 199, 200} {
		user := createReserveTestUser(t, 0)
		event := RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: timestamp, Confirmed: true}
		require.NoError(t, ProcessRewardActivityEvent(context.Background(), event))
	}
	var rewards []UserReward
	require.NoError(t, DB.Find(&rewards).Error)
	require.Len(t, rewards, 2)
	for _, reward := range rewards {
		assert.Equal(t, campaign.Id, reward.CampaignID)
	}
	var original RewardCampaign
	require.NoError(t, DB.First(&original, legacy.Id).Error)
	assert.Equal(t, legacy, original, "an old campaign with a future end must never be reactivated")
}

func TestContinuousRewardFutureEvidenceWaitsAndUsesPersonalGrantTime(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	now := common.GetTimestamp()
	input := rewardTestCampaignInput("personal-deadline")
	input.Continuous, input.StartTime, input.EndTime = true, now-100, now+7200
	input.ExpiresInDays = 3
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	event := RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: now + 3600, Confirmed: true}
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), event))
	var count int64
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.Zero(t, count, "an anomalous future source timestamp cannot award before it occurs")
	grantTime := now + 3600
	require.NoError(t, grantRewardDistributionCandidate(context.Background(), campaign.Id, user.Id, func() int64 { return grantTime }))
	var reward UserReward
	require.NoError(t, DB.First(&reward).Error)
	assert.Equal(t, grantTime, reward.CreatedAt)
	assert.Equal(t, grantTime+3*86400, reward.ExpiresAt)
	assert.NotEqual(t, campaign.CreatedAt+3*86400, reward.ExpiresAt)
	grantTime += 86400
	require.NoError(t, grantRewardDistributionCandidate(context.Background(), campaign.Id, user.Id, func() int64 { return grantTime }))
	var retried UserReward
	require.NoError(t, DB.First(&retried, reward.Id).Error)
	assert.Equal(t, reward, retried, "replaying delivery cannot extend a personal deadline")
}

func TestContinuousRewardDisabledCandidateIsRetriedWithoutDuplicatingAward(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	require.NoError(t, DB.Model(&user).Update("status", common.UserStatusDisabled).Error)
	input := rewardTestCampaignInput("reenabled-candidate")
	input.Continuous = true
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	event := RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: 150, Confirmed: true}
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), event))
	var count int64
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&user).Update("status", common.UserStatusEnabled).Error)
	drainRewardDistributionForTest(t, 2)
	require.NoError(t, DB.Model(&UserReward{}).Where("campaign_id = ?", campaign.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestContinuousRewardBackfillScansOldTopupIDsBeforeApplyingActivityFilter(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	for i := 0; i < 5; i++ {
		require.NoError(t, DB.Create(&TopUp{UserId: user.Id, TradeNo: fmt.Sprint("old-", i), Status: common.TopUpStatusPending, CompleteTime: 0}).Error)
	}
	input := rewardTestCampaignInput("topup-backfill")
	input.Continuous, input.Audience = true, "topup"
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	require.NoError(t, ProcessRewardCampaignBatch(context.Background(), campaign.Id, 2))
	var state RewardDistributionState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.Id).Error)
	assert.EqualValues(t, 2, state.TopupCursorID)
	assert.False(t, state.TopupDone)
	// An already-passed primary key can complete later: the durable event, not
	// a maximum-ID cursor or an arbitrary lookback window, delivers that user.
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", 1).Updates(map[string]interface{}{"status": common.TopUpStatusSuccess, "complete_time": 150}).Error)
	require.NoError(t, DB.Create(&RewardActivityEvent{Kind: "topup", UserID: user.Id, OccurredAt: 150, SourceID: 1, Confirmed: true}).Error)
	drainRewardDistributionForTest(t, 2)
	var reward UserReward
	require.NoError(t, DB.First(&reward).Error)
	assert.Equal(t, user.Id, reward.UserID)
}

func TestContinuousRewardClaimTotalsRemainExactBeyondWalletAndInt64Limits(t *testing.T) {
	setupContinuousRewardTestDB(t)
	campaign := RewardCampaign{Title: "Uncapped activity", Type: RewardTypeFixed, MinQuota: 1, MaxQuota: 1, RequestID: "large-total", Continuous: true}
	require.NoError(t, DB.Create(&campaign).Error)
	// These are ledger fixtures, not wallet credits. Each award is within the
	// existing wallet limit; their legitimate campaign total exceeds int64.
	rewards := make([]UserReward, 1100)
	for i := range rewards {
		rewards[i] = UserReward{CampaignID: campaign.Id, UserID: i + 1, Status: RewardStatusClaimed, QuotaAwarded: common.MaxWalletQuota}
	}
	require.NoError(t, DB.CreateInBatches(&rewards, 50).Error)
	items, _, err := ListRewardCampaigns(1, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.EqualValues(t, 1100, items[0].ClaimedCount)
	assert.Equal(t, "9907919180215090100", items[0].ClaimedQuotaExact)
	encoded, err := common.Marshal(items[0])
	require.NoError(t, err)
	var wire struct {
		ClaimedQuota      *int64 `json:"claimed_quota"`
		ClaimedQuotaExact string `json:"claimed_quota_exact"`
	}
	require.NoError(t, common.Unmarshal(encoded, &wire))
	assert.Nil(t, wire.ClaimedQuota, "unsafe numeric totals must be explicitly absent, never rounded or capped")
	assert.Equal(t, "9907919180215090100", wire.ClaimedQuotaExact)
}

func TestContinuousRewardFailedGrantPreservesBackfillCheckpointForRecovery(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, Type: LogTypeConsume, CreatedAt: 150}).Error)
	input := rewardTestCampaignInput("failed-grant-recovery")
	input.Continuous = true
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	callback := "test:continuous_grant_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_rewards" {
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	require.ErrorIs(t, ProcessRewardCampaignBatch(context.Background(), campaign.Id, 2), assert.AnError)
	var state RewardDistributionState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.Id).Error)
	assert.Zero(t, state.UsageCursorID)
	assert.False(t, state.UsageDone)
	var count int64
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Zero(t, campaign.RecipientCount)
	require.NoError(t, DB.Callback().Create().Remove(callback))
	drainRewardDistributionForTest(t, 2)
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestContinuousRewardAcknowledgementFailureReplaysWithoutResettingAward(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	input := rewardTestCampaignInput("ack-failure-recovery")
	input.Continuous = true
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	drainRewardDistributionForTest(t, 2)
	event := RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: 150, Confirmed: true}
	require.NoError(t, LOG_DB.Create(&event).Error)
	callback := "test:continuous_ack_failure"
	require.NoError(t, LOG_DB.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "reward_activity_events" {
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { _ = LOG_DB.Callback().Delete().Remove(callback) })
	_, err = RunRewardDistributionPass(context.Background(), 2)
	require.ErrorIs(t, err, assert.AnError)
	var granted UserReward
	require.NoError(t, DB.First(&granted).Error)
	var count int64
	require.NoError(t, LOG_DB.Model(&RewardActivityEvent{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, LOG_DB.Callback().Delete().Remove(callback))
	drainRewardDistributionForTest(t, 2)
	var after UserReward
	require.NoError(t, DB.First(&after, granted.Id).Error)
	assert.Equal(t, granted, after)
	require.NoError(t, DB.First(campaign, campaign.Id).Error)
	assert.Equal(t, 1, campaign.RecipientCount)
	require.NoError(t, LOG_DB.Model(&RewardActivityEvent{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestContinuousRewardCandidateCooldownDoesNotDelayNewQualifyingEvidence(t *testing.T) {
	setupContinuousRewardTestDB(t)
	admin := createReserveTestUser(t, 0)
	user := createReserveTestUser(t, 0)
	input := rewardTestCampaignInput("candidate-cooldown")
	input.Continuous, input.Audience = true, "both"
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: 150, Confirmed: true}))
	drainRewardDistributionForTest(t, 2)
	var state RewardDistributionState
	require.NoError(t, DB.First(&state, "campaign_id = ?", campaign.Id).Error)
	assert.Greater(t, state.NextCandidateCheckAt, common.GetTimestamp(), "an exhausted pending-candidate pass must back off instead of tight-looping")
	var count int64
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, TradeNo: "new-proof", Status: common.TopUpStatusSuccess, CompleteTime: 160}).Error)
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), RewardActivityEvent{Kind: "topup", UserID: user.Id, OccurredAt: 160, Confirmed: true}))
	require.NoError(t, DB.Model(&UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "fresh evidence must attempt delivery immediately despite the background cooldown")
}
