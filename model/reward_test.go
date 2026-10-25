package model

import (
	"bytes"
	cryptorand "crypto/rand"
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Keep the log database separate so audience selection cannot accidentally depend
// on a cross-database SQL join or on the main database containing usage records.
func setupRewardTestDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlLogDB, err := logDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlLogDB.SetMaxOpenConns(1)
	DB, LOG_DB = db, logDB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch
		require.NoError(t, sqlDB.Close())
		require.NoError(t, sqlLogDB.Close())
	})
	require.NoError(t, DB.AutoMigrate(&User{}, &TopUp{}, &RewardCampaign{}, &UserReward{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
}

func rewardTestCampaignInput(requestID string) RewardCampaignInput {
	return RewardCampaignInput{
		Title: "Loyal customer reward", Description: "Thank you for creating with us.",
		Type: "fixed", MinQuota: 25, MaxQuota: 25, ExpiresInDays: 7,
		Audience: "usage", StartTime: 100, EndTime: 200, RequestID: requestID,
	}
}

func createRewardForTestUser(t *testing.T, user User, input RewardCampaignInput) UserReward {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, SiteId: 0, CreatedAt: 100, Type: LogTypeConsume}).Error)
	campaign, err := CreateRewardCampaign(input, 1)
	require.NoError(t, err)
	var reward UserReward
	require.NoError(t, DB.Where("campaign_id = ? AND user_id = ?", campaign.Id, user.Id).First(&reward).Error)
	return reward
}

func TestRewardAudienceFiltersPaymentStateTimeAndSiteAcrossSeparateDatabases(t *testing.T) {
	setupRewardTestDB(t)
	topUpOnly := createReserveTestUser(t, 0)
	usageOnly := createReserveTestUser(t, 0)
	both := createReserveTestUser(t, 0)
	endBoundary := createReserveTestUser(t, 0)
	beforeStart := createReserveTestUser(t, 0)
	pending := createReserveTestUser(t, 0)
	refunded := createReserveTestUser(t, 0)
	partiallyRefunded := createReserveTestUser(t, 0)
	disabled := createReserveTestUser(t, 0)
	deleted := createReserveTestUser(t, 0)
	foreign := createReserveTestUser(t, 0)
	foreignRecords := createReserveTestUser(t, 0)
	errorOnly := createReserveTestUser(t, 0)
	require.NoError(t, DB.Model(&disabled).Update("status", common.UserStatusDisabled).Error)
	require.NoError(t, DB.Delete(&deleted).Error)
	require.NoError(t, DB.Model(&foreign).Update("site_id", 8).Error)
	topUps := []TopUp{
		{UserId: topUpOnly.Id, TradeNo: "start", CompleteTime: 100, CreateTime: 99, Status: common.TopUpStatusSuccess},
		{UserId: usageOnly.Id, TradeNo: "usage-user-outside-window", CompleteTime: 99, Status: common.TopUpStatusSuccess},
		{UserId: both.Id, TradeNo: "both", CompleteTime: 199, Status: common.TopUpStatusSuccess},
		{UserId: both.Id, TradeNo: "both-again", CompleteTime: 150, Status: common.TopUpStatusSuccess},
		{UserId: endBoundary.Id, TradeNo: "end", CompleteTime: 200, Status: common.TopUpStatusSuccess},
		{UserId: beforeStart.Id, TradeNo: "before", CompleteTime: 99, Status: common.TopUpStatusSuccess},
		{UserId: pending.Id, TradeNo: "pending", CompleteTime: 150, Status: common.TopUpStatusPending},
		{UserId: refunded.Id, TradeNo: "refunded", CompleteTime: 150, Status: common.TopUpStatusRefunded},
		{UserId: partiallyRefunded.Id, TradeNo: "part-refund", CompleteTime: 150, Status: common.TopUpStatusSuccess, ClawedBackQuota: 1},
		{UserId: disabled.Id, TradeNo: "disabled", CompleteTime: 150, Status: common.TopUpStatusSuccess},
		{UserId: deleted.Id, TradeNo: "deleted", CompleteTime: 150, Status: common.TopUpStatusSuccess},
		{UserId: foreign.Id, TradeNo: "foreign-user", CompleteTime: 150, Status: common.TopUpStatusSuccess},
		{UserId: foreignRecords.Id, SiteId: 8, TradeNo: "foreign-record", CompleteTime: 150, Status: common.TopUpStatusSuccess},
	}
	require.NoError(t, DB.Create(&topUps).Error)
	logs := []Log{
		{UserId: usageOnly.Id, CreatedAt: 100, Type: LogTypeConsume},
		{UserId: topUpOnly.Id, CreatedAt: 200, Type: LogTypeConsume},
		{UserId: both.Id, CreatedAt: 199, Type: LogTypeConsume},
		{UserId: both.Id, CreatedAt: 150, Type: LogTypeConsume},
		{UserId: endBoundary.Id, CreatedAt: 200, Type: LogTypeConsume},
		{UserId: beforeStart.Id, CreatedAt: 99, Type: LogTypeConsume},
		{UserId: disabled.Id, CreatedAt: 150, Type: LogTypeConsume},
		{UserId: deleted.Id, CreatedAt: 150, Type: LogTypeConsume},
		{UserId: foreign.Id, CreatedAt: 150, Type: LogTypeConsume},
		{UserId: foreignRecords.Id, SiteId: 8, CreatedAt: 150, Type: LogTypeConsume},
		{UserId: errorOnly.Id, CreatedAt: 150, Type: LogTypeError},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	for _, tc := range []struct {
		audience   string
		recipients []int
	}{
		{"topup", []int{topUpOnly.Id, both.Id}},
		{"usage", []int{usageOnly.Id, both.Id}},
		{"both", []int{both.Id}},
		{"either", []int{topUpOnly.Id, usageOnly.Id, both.Id}},
	} {
		t.Run(tc.audience, func(t *testing.T) {
			preview, err := PreviewRewardRecipients(RewardAudienceFilter{Audience: tc.audience, StartTime: 100, EndTime: 200})
			require.NoError(t, err)
			assert.Equal(t, len(tc.recipients), preview.RecipientCount)
			input := rewardTestCampaignInput("audience-" + tc.audience)
			input.Audience = tc.audience
			campaign, err := CreateRewardCampaign(input, 1)
			require.NoError(t, err)
			assert.Equal(t, len(tc.recipients), campaign.RecipientCount)
			var recipients []int
			require.NoError(t, DB.Model(&UserReward{}).Where("campaign_id = ?", campaign.Id).Pluck("user_id", &recipients).Error)
			assert.ElementsMatch(t, tc.recipients, recipients)
		})
	}
}

func TestRewardCampaignRetryKeepsOriginalRecipientsAndRejectsConflictingRequest(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 0)
	input := rewardTestCampaignInput("campaign-retry")
	reward := createRewardForTestUser(t, user, input)
	laterUser := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: laterUser.Id, CreatedAt: 150, Type: LogTypeConsume}).Error)

	retried, err := CreateRewardCampaign(input, 1)
	require.NoError(t, err)
	assert.Equal(t, reward.CampaignID, retried.Id)
	assert.EqualValues(t, 1, retried.RecipientCount)
	var recipientCount int64
	require.NoError(t, DB.Model(&UserReward{}).Where("campaign_id = ?", retried.Id).Count(&recipientCount).Error)
	assert.EqualValues(t, 1, recipientCount)
	input.Title = "A different campaign"
	_, err = CreateRewardCampaign(input, 1)
	assert.ErrorIs(t, err, ErrRewardRequestConflict)
	input.Title = "Loyal customer reward"
	_, err = CreateRewardCampaign(input, 2)
	assert.ErrorIs(t, err, ErrRewardRequestConflict)
	campaigns, total, err := ListRewardCampaigns(1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, campaigns, 1)
	assert.Equal(t, reward.CampaignID, campaigns[0].Id)
}

func TestRewardCampaignRecipientInsertFailureRollsBackIssuanceAndAllowsRetry(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 100)
	require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, CreatedAt: 100, Type: LogTypeConsume}).Error)
	forcedError := errors.New("reward recipient insert unavailable")
	const callback = "test:reward_recipient_insert_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_rewards" {
			tx.AddError(forcedError)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	input := rewardTestCampaignInput("failed-issuance")
	_, err := CreateRewardCampaign(input, 1)
	assert.ErrorIs(t, err, forcedError)
	var campaigns, rewards int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaigns).Error)
	require.NoError(t, DB.Model(&UserReward{}).Count(&rewards).Error)
	assert.Zero(t, campaigns)
	assert.Zero(t, rewards)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))

	require.NoError(t, DB.Callback().Create().Remove(callback))
	campaign, err := CreateRewardCampaign(input, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, campaign.RecipientCount)
	require.NoError(t, DB.Model(&UserReward{}).Count(&rewards).Error)
	assert.EqualValues(t, 1, rewards)
}

func TestRewardConcurrentIssuanceRetriesCreateOneCampaignAndOneReward(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 100)
	require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, CreatedAt: 100, Type: LogTypeConsume}).Error)
	input := rewardTestCampaignInput("concurrent-issuance")
	var workers sync.WaitGroup
	start := make(chan struct{})
	type campaignResult struct {
		campaign *RewardCampaign
		err      error
	}
	results := make(chan campaignResult, 2)
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			<-start
			campaign, err := CreateRewardCampaign(input, 1)
			results <- campaignResult{campaign: campaign, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	var campaignIDs []int
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.campaign)
		campaignIDs = append(campaignIDs, result.campaign.Id)
	}
	require.Len(t, campaignIDs, 2)
	assert.Equal(t, campaignIDs[0], campaignIDs[1])
	var campaigns, rewards int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaigns).Error)
	require.NoError(t, DB.Model(&UserReward{}).Count(&rewards).Error)
	assert.EqualValues(t, 1, campaigns)
	assert.EqualValues(t, 1, rewards)
}

func TestRewardCampaignRejectsChangedAudienceUntilAdministratorPreviewsAgain(t *testing.T) {
	setupRewardTestDB(t)
	first := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: first.Id, CreatedAt: 100, Type: LogTypeConsume}).Error)
	input := rewardTestCampaignInput("preview-recipients")
	preview, err := PreviewRewardRecipients(RewardAudienceFilter{Audience: input.Audience, StartTime: input.StartTime, EndTime: input.EndTime})
	require.NoError(t, err)
	input.ExpectedRecipientCount = &preview.RecipientCount
	second := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: second.Id, CreatedAt: 150, Type: LogTypeConsume}).Error)
	_, err = CreateRewardCampaign(input, 1)
	assert.ErrorIs(t, err, ErrRewardAudienceChanged)
	var count int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&count).Error)
	assert.Zero(t, count)

	preview, err = PreviewRewardRecipients(RewardAudienceFilter{Audience: input.Audience, StartTime: input.StartTime, EndTime: input.EndTime})
	require.NoError(t, err)
	input.ExpectedRecipientCount = &preview.RecipientCount
	campaign, err := CreateRewardCampaign(input, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, campaign.RecipientCount)
	third := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: third.Id, CreatedAt: 150, Type: LogTypeConsume}).Error)
	retried, err := CreateRewardCampaign(input, 1)
	require.NoError(t, err)
	assert.Equal(t, campaign.Id, retried.Id)
	assert.EqualValues(t, 2, retried.RecipientCount)
}

func TestRewardCampaignRejectsAudienceChangedByDisablingRecipientAfterPreview(t *testing.T) {
	setupRewardTestDB(t)
	issuer := createReserveTestUser(t, 0)
	recipient := createReserveTestUser(t, 0)
	logs := []Log{
		{UserId: issuer.Id, CreatedAt: 100, Type: LogTypeConsume},
		{UserId: recipient.Id, CreatedAt: 150, Type: LogTypeConsume},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	input := rewardTestCampaignInput("disabled-after-preview")
	preview, err := PreviewRewardRecipients(RewardAudienceFilter{Audience: input.Audience, StartTime: input.StartTime, EndTime: input.EndTime})
	require.NoError(t, err)
	require.Equal(t, 2, preview.RecipientCount)
	input.ExpectedRecipientCount = &preview.RecipientCount
	require.NoError(t, DB.Model(&recipient).Update("status", common.UserStatusDisabled).Error)
	_, err = CreateRewardCampaign(input, issuer.Id)
	assert.ErrorIs(t, err, ErrRewardAudienceChanged)
	var campaignCount, rewardCount int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaignCount).Error)
	require.NoError(t, DB.Model(&UserReward{}).Count(&rewardCount).Error)
	assert.Zero(t, campaignCount)
	assert.Zero(t, rewardCount)
	updated, err := PreviewRewardRecipients(RewardAudienceFilter{Audience: input.Audience, StartTime: input.StartTime, EndTime: input.EndTime})
	require.NoError(t, err)
	assert.Equal(t, 1, updated.RecipientCount)
}

func TestRewardCampaignRechecksRecipientsDisabledAfterSelectionBeforeIssuance(t *testing.T) {
	setupRewardTestDB(t)
	issuer := createReserveTestUser(t, 0)
	recipient := createReserveTestUser(t, 0)
	logs := []Log{
		{UserId: issuer.Id, CreatedAt: 100, Type: LogTypeConsume},
		{UserId: recipient.Id, CreatedAt: 150, Type: LogTypeConsume},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	const callback = "test:disable_reward_recipient_after_selection"
	disabledAfterSelection := false
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if disabledAfterSelection || tx.Error != nil || tx.Statement.Table != "users" {
			return
		}
		// The source query has finished and returned its recipient IDs. The
		// transaction must not trust those IDs after this account change.
		selectedIDs, ok := tx.Statement.Dest.(*[]int)
		if !ok {
			return
		}
		for _, id := range *selectedIDs {
			if id == recipient.Id {
				disabledAfterSelection = true
				tx.AddError(DB.Model(&User{}).Where("id = ?", recipient.Id).Update("status", common.UserStatusDisabled).Error)
				return
			}
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callback) })

	_, err := CreateRewardCampaign(rewardTestCampaignInput("disabled-after-selection"), issuer.Id)
	require.True(t, disabledAfterSelection, "the account change must occur after its ID was selected")
	assert.ErrorIs(t, err, ErrRewardAudienceChanged)
	var campaignCount, rewardCount int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaignCount).Error)
	require.NoError(t, DB.Model(&UserReward{}).Count(&rewardCount).Error)
	assert.Zero(t, campaignCount)
	assert.Zero(t, rewardCount)
}

// Read the serialized public contract, including zero values. A missing field
// must not be mistaken for an activity with no claims.
func rewardCampaignClaimStats(t *testing.T, campaign RewardCampaign) (int64, int64) {
	t.Helper()
	encoded, err := common.Marshal(campaign)
	require.NoError(t, err)
	var statistics struct {
		ClaimedCount *int64 `json:"claimed_count"`
		ClaimedQuota *int64 `json:"claimed_quota"`
	}
	require.NoError(t, common.Unmarshal(encoded, &statistics))
	require.NotNil(t, statistics.ClaimedCount, "campaign responses must include claimed_count, including zero")
	require.NotNil(t, statistics.ClaimedQuota, "campaign responses must include claimed_quota, including zero")
	return *statistics.ClaimedCount, *statistics.ClaimedQuota
}

func TestRewardCampaignClaimStatisticsPreserveActualAmountsAndHistoricalRecipients(t *testing.T) {
	for _, rewardType := range []string{"fixed", "random"} {
		t.Run(rewardType, func(t *testing.T) {
			setupRewardTestDB(t)
			issuer := createReserveTestUser(t, 0)
			first := createReserveTestUser(t, 100)
			second := createReserveTestUser(t, 200)
			unclaimed := createReserveTestUser(t, 300)
			logs := []Log{
				{UserId: first.Id, CreatedAt: 100, Type: LogTypeConsume},
				{UserId: second.Id, CreatedAt: 150, Type: LogTypeConsume},
				{UserId: unclaimed.Id, CreatedAt: 199, Type: LogTypeConsume},
			}
			require.NoError(t, LOG_DB.Create(&logs).Error)
			olderInput := rewardTestCampaignInput("older-statistics-campaign")
			olderInput.MinQuota, olderInput.MaxQuota = 3, 3
			older, err := CreateRewardCampaign(olderInput, issuer.Id)
			require.NoError(t, err)
			input := rewardTestCampaignInput("claimed-statistics-campaign")
			input.Type, input.MinQuota, input.MaxQuota = rewardType, 7, 7
			if rewardType == "random" {
				input.MaxQuota = 11
			}
			campaign, err := CreateRewardCampaign(input, issuer.Id)
			require.NoError(t, err)
			require.Equal(t, 3, campaign.RecipientCount)

			items, total, err := ListRewardCampaigns(1, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 2, total)
			require.Len(t, items, 1)
			assert.Equal(t, campaign.Id, items[0].Id)
			count, quota := rewardCampaignClaimStats(t, items[0])
			assert.Zero(t, count)
			assert.Zero(t, quota)

			// Zero entropy fixes random draws at the lower bound, making a
			// configured-maximum sum observably different from the credited sum.
			previousRandomReader := cryptorand.Reader
			cryptorand.Reader = bytes.NewReader(make([]byte, 64))
			t.Cleanup(func() { cryptorand.Reader = previousRandomReader })
			var firstReward, secondReward, olderReward UserReward
			require.NoError(t, DB.Where("campaign_id = ? AND user_id = ?", campaign.Id, first.Id).First(&firstReward).Error)
			require.NoError(t, DB.Where("campaign_id = ? AND user_id = ?", campaign.Id, second.Id).First(&secondReward).Error)
			require.NoError(t, DB.Where("campaign_id = ? AND user_id = ?", older.Id, first.Id).First(&olderReward).Error)
			firstClaim, fresh, err := ClaimUserReward(first.Id, firstReward.Id)
			require.NoError(t, err)
			assert.True(t, fresh)
			assert.Equal(t, 7, firstClaim.QuotaAwarded)
			secondClaim, fresh, err := ClaimUserReward(second.Id, secondReward.Id)
			require.NoError(t, err)
			assert.True(t, fresh)
			assert.Equal(t, 7, secondClaim.QuotaAwarded)
			_, fresh, err = ClaimUserReward(first.Id, firstReward.Id)
			require.NoError(t, err)
			assert.False(t, fresh)
			_, fresh, err = ClaimUserReward(first.Id, olderReward.Id)
			require.NoError(t, err)
			assert.True(t, fresh)
			assert.Equal(t, 110, getUserQuotaFromDB(t, first.Id))
			assert.Equal(t, 207, getUserQuotaFromDB(t, second.Id))

			require.NoError(t, DB.Model(&first).Update("status", common.UserStatusDisabled).Error)
			require.NoError(t, DB.Unscoped().Delete(&second).Error)
			items, total, err = ListRewardCampaigns(1, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 2, total)
			require.Len(t, items, 1)
			assert.Equal(t, campaign.Id, items[0].Id)
			assert.Equal(t, 3, items[0].RecipientCount)
			count, quota = rewardCampaignClaimStats(t, items[0])
			assert.EqualValues(t, 2, count)
			assert.EqualValues(t, 14, quota)
			items, total, err = ListRewardCampaigns(2, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 2, total)
			require.Len(t, items, 1)
			assert.Equal(t, older.Id, items[0].Id)
			count, quota = rewardCampaignClaimStats(t, items[0])
			assert.EqualValues(t, 1, count)
			assert.EqualValues(t, 3, quota)
		})
	}
}

func TestRewardSummaryRejectsCombinedAmountsBeyondSafeWalletInteger(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 0)
	input := rewardTestCampaignInput("large-campaign-first")
	input.MinQuota, input.MaxQuota = common.MaxWalletQuota/2+1, common.MaxWalletQuota/2+1
	createRewardForTestUser(t, user, input)
	input.RequestID = "large-campaign-second"
	createRewardForTestUser(t, user, input)
	summary, err := GetUserRewardSummary(user.Id)
	assert.ErrorIs(t, err, ErrRewardRecipientsLimit)
	assert.Nil(t, summary)
}

func TestRewardFixedClaimCreditsBalanceAndCacheOnlyOnceAndPreventsTheft(t *testing.T) {
	setupRewardTestDB(t)
	useUserCacheMiniRedis(t)
	owner := createReserveTestUser(t, 100)
	attacker := createReserveTestUser(t, 200)
	require.NoError(t, populateUserCache(owner))
	reward := createRewardForTestUser(t, owner, rewardTestCampaignInput("fixed-claim"))
	summary, err := GetUserRewardSummary(owner.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, summary.PendingCount)
	assert.EqualValues(t, 1, summary.FixedCount)
	assert.EqualValues(t, 25, summary.FixedQuota)

	_, newlyClaimed, err := ClaimUserReward(attacker.Id, reward.Id)
	assert.ErrorIs(t, err, ErrRewardNotFound)
	assert.False(t, newlyClaimed)
	assert.Equal(t, 200, getUserQuotaFromDB(t, attacker.Id))
	claimed, newlyClaimed, err := ClaimUserReward(owner.Id, reward.Id)
	require.NoError(t, err)
	assert.True(t, newlyClaimed)
	assert.Equal(t, "claimed", claimed.Status)
	assert.Equal(t, 25, claimed.QuotaAwarded)
	assert.Positive(t, claimed.ClaimedAt)
	assert.Equal(t, 125, getUserQuotaFromDB(t, owner.Id))
	cached, err := cacheGetUserBase(owner.Id)
	require.NoError(t, err)
	assert.Equal(t, 125, cached.Quota)

	repeated, newlyClaimed, err := ClaimUserReward(owner.Id, reward.Id)
	require.NoError(t, err)
	assert.False(t, newlyClaimed)
	assert.Equal(t, claimed.QuotaAwarded, repeated.QuotaAwarded)
	assert.Equal(t, claimed.ClaimedAt, repeated.ClaimedAt)
	assert.Equal(t, 125, getUserQuotaFromDB(t, owner.Id))
	summary, err = GetUserRewardSummary(owner.Id)
	require.NoError(t, err)
	assert.Zero(t, summary.PendingCount)
	assert.Zero(t, summary.FixedQuota)
	pending, total, err := ListUserRewards(owner.Id, "pending", "", 1, 10)
	require.NoError(t, err)
	assert.Empty(t, pending)
	assert.Zero(t, total)
	history, total, err := ListUserRewards(owner.Id, "claimed", "fixed", 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, history, 1)
	assert.Equal(t, reward.Id, history[0].Id)
	_, total, err = ListUserRewards(attacker.Id, "", "", 1, 10)
	require.NoError(t, err)
	assert.Zero(t, total)
}

func TestRewardRandomClaimRevealsAmountWithinConfiguredRangeAndRetainsItOnRetry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		min, max int
	}{{"single amount", 25, 25}, {"amount range", 10, 30}} {
		t.Run(tc.name, func(t *testing.T) {
			setupRewardTestDB(t)
			user := createReserveTestUser(t, 100)
			input := rewardTestCampaignInput("random-claim")
			input.Type, input.MinQuota, input.MaxQuota = "random", tc.min, tc.max
			reward := createRewardForTestUser(t, user, input)
			pending, total, err := ListUserRewards(user.Id, "pending", "random", 1, 10)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, pending, 1)
			assert.Zero(t, pending[0].QuotaAwarded)
			summary, err := GetUserRewardSummary(user.Id)
			require.NoError(t, err)
			assert.EqualValues(t, 1, summary.RandomCount)
			assert.Zero(t, summary.FixedQuota)
			claimed, fresh, err := ClaimUserReward(user.Id, reward.Id)
			require.NoError(t, err)
			assert.True(t, fresh)
			assert.GreaterOrEqual(t, claimed.QuotaAwarded, tc.min)
			assert.LessOrEqual(t, claimed.QuotaAwarded, tc.max)
			assert.Equal(t, 100+claimed.QuotaAwarded, getUserQuotaFromDB(t, user.Id))
			repeated, fresh, err := ClaimUserReward(user.Id, reward.Id)
			require.NoError(t, err)
			assert.False(t, fresh)
			assert.Equal(t, claimed.QuotaAwarded, repeated.QuotaAwarded)
			assert.Equal(t, 100+claimed.QuotaAwarded, getUserQuotaFromDB(t, user.Id))
		})
	}
}

func TestRewardClaimWalletLimitRollsBackRewardAndBalance(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, common.MaxWalletQuota-10)
	reward := createRewardForTestUser(t, user, rewardTestCampaignInput("overflow-claim"))
	_, fresh, err := ClaimUserReward(user.Id, reward.Id)
	assert.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	assert.False(t, fresh)
	assert.Equal(t, common.MaxWalletQuota-10, getUserQuotaFromDB(t, user.Id))
	var persisted UserReward
	require.NoError(t, DB.First(&persisted, reward.Id).Error)
	assert.Equal(t, "pending", persisted.Status)
	assert.Zero(t, persisted.ClaimedAt)
	assert.Zero(t, persisted.QuotaAwarded)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 100).Error)
	_, fresh, err = ClaimUserReward(user.Id, reward.Id)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id))
}

func TestRewardConcurrentClaimsCreditOnlyOnce(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 100)
	reward := createRewardForTestUser(t, user, rewardTestCampaignInput("concurrent-claim"))
	var workers sync.WaitGroup
	start := make(chan struct{})
	type claimResult struct {
		reward *UserReward
		fresh  bool
		err    error
	}
	results := make(chan claimResult, 2)
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			<-start
			reward, fresh, err := ClaimUserReward(user.Id, reward.Id)
			results <- claimResult{reward: reward, fresh: fresh, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	newClaims := 0
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.reward)
		assert.Equal(t, 25, result.reward.QuotaAwarded)
		if result.fresh {
			newClaims++
		}
	}
	assert.Equal(t, 1, newClaims)
	assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id))
}

func TestRewardClaimRejectsUnavailableOrForeignUsers(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       int
	}{{"disabled", "status", common.UserStatusDisabled}, {"foreign site", "site_id", 8}} {
		t.Run(tc.name, func(t *testing.T) {
			setupRewardTestDB(t)
			user := createReserveTestUser(t, 100)
			reward := createRewardForTestUser(t, user, rewardTestCampaignInput("unavailable-user"))
			require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update(tc.field, tc.value).Error)
			_, fresh, err := ClaimUserReward(user.Id, reward.Id)
			assert.ErrorIs(t, err, ErrRewardUserUnavailable)
			assert.False(t, fresh)
			assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
		})
	}
}

func TestRewardCampaignRejectsUnavailableOrForeignIssuers(t *testing.T) {
	setupRewardTestDB(t)
	recipient := createReserveTestUser(t, 100)
	disabledIssuer := createReserveTestUser(t, 0)
	foreignIssuer := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: recipient.Id, CreatedAt: 100, Type: LogTypeConsume}).Error)
	require.NoError(t, DB.Model(&disabledIssuer).Update("status", common.UserStatusDisabled).Error)
	require.NoError(t, DB.Model(&foreignIssuer).Update("site_id", 8).Error)
	for _, actorID := range []int{disabledIssuer.Id, foreignIssuer.Id, 999999} {
		_, err := CreateRewardCampaign(rewardTestCampaignInput("unauthorized-issuer"), actorID)
		assert.ErrorIs(t, err, ErrRewardUserUnavailable)
	}
	var campaigns int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaigns).Error)
	assert.Zero(t, campaigns)
}

func TestRewardCampaignRejectsInvalidAmountsAndEmptyRecipientsWithoutIssuingRewards(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, CreatedAt: 100, Type: LogTypeConsume}).Error)
	for _, tc := range []struct {
		name       string
		rewardType string
		minimum    int
		maximum    int
	}{{"zero", "fixed", 0, 0}, {"negative", "fixed", -1, -1}, {"reversed", "random", 20, 10}, {"fixed range", "fixed", 10, 20}, {"unsupported type", "other", 10, 10}, {"wallet bound", "fixed", common.MaxWalletQuota + 1, common.MaxWalletQuota + 1}} {
		t.Run(tc.name, func(t *testing.T) {
			input := rewardTestCampaignInput("invalid-amount")
			input.Type, input.MinQuota, input.MaxQuota = tc.rewardType, tc.minimum, tc.maximum
			_, err := CreateRewardCampaign(input, 1)
			assert.ErrorIs(t, err, ErrInvalidRewardCampaign)
		})
	}
	input := rewardTestCampaignInput("empty-recipients")
	input.StartTime, input.EndTime = 300, 400
	_, err := CreateRewardCampaign(input, 1)
	assert.ErrorIs(t, err, ErrRewardRecipientsEmpty)
	var campaigns, rewards int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaigns).Error)
	require.NoError(t, DB.Model(&UserReward{}).Count(&rewards).Error)
	assert.Zero(t, campaigns)
	assert.Zero(t, rewards)
}

func TestRewardAudienceRejectsInvalidTimeWindowsAndUnknownSelection(t *testing.T) {
	setupRewardTestDB(t)
	for _, filter := range []RewardAudienceFilter{
		{Audience: "either", StartTime: -1, EndTime: 100},
		{Audience: "either", StartTime: 100, EndTime: 100},
		{Audience: "either", StartTime: 200, EndTime: 100},
		{Audience: "unknown", StartTime: 100, EndTime: 200},
	} {
		_, err := PreviewRewardRecipients(filter)
		assert.ErrorIs(t, err, ErrInvalidRewardAudience)
	}
}

func TestRewardCampaignExpiryDaysRejectMissingAndOutOfRangeValues(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 0)
	require.NoError(t, LOG_DB.Create(&Log{UserId: user.Id, CreatedAt: 100, Type: LogTypeConsume}).Error)
	for _, tc := range []struct {
		name  string
		days  int
		valid bool
	}{
		{"missing", 0, false},
		{"negative", -1, false},
		{"too-long", 3651, false},
		{"minimum", 1, true},
		{"maximum", 3650, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := rewardTestCampaignInput("expiry-days-" + tc.name)
			input.ExpiresInDays = tc.days
			campaign, err := CreateRewardCampaign(input, user.Id)
			if !tc.valid {
				assert.ErrorIs(t, err, ErrInvalidRewardCampaign)
				assert.Nil(t, campaign)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.days, campaign.ExpiresInDays)
			assert.Equal(t, campaign.CreatedAt+int64(tc.days)*86400, campaign.ExpiresAt)
		})
	}
	var campaignCount int64
	require.NoError(t, DB.Model(&RewardCampaign{}).Count(&campaignCount).Error)
	assert.EqualValues(t, 2, campaignCount)
}

func TestRewardCampaignExpiryDeadlineIsSharedAndNeverExtendedByRetry(t *testing.T) {
	setupRewardTestDB(t)
	first := createReserveTestUser(t, 0)
	second := createReserveTestUser(t, 0)
	logs := []Log{
		{UserId: first.Id, CreatedAt: 100, Type: LogTypeConsume},
		{UserId: second.Id, CreatedAt: 150, Type: LogTypeConsume},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	input := rewardTestCampaignInput("shared-expiry-deadline")
	campaign, err := CreateRewardCampaign(input, first.Id)
	require.NoError(t, err)
	assert.Equal(t, 7, campaign.ExpiresInDays)
	assert.Equal(t, campaign.CreatedAt+7*86400, campaign.ExpiresAt)
	var rewards []UserReward
	require.NoError(t, DB.Where("campaign_id = ?", campaign.Id).Find(&rewards).Error)
	require.Len(t, rewards, 2)
	for _, reward := range rewards {
		assert.Equal(t, campaign.ExpiresAt, reward.ExpiresAt)
	}

	// Persist a deadline in the past to model a retry after the activity ended.
	pastDeadline := common.GetTimestamp() - 1
	require.NoError(t, DB.Model(&RewardCampaign{}).Where("id = ?", campaign.Id).Update("expires_at", pastDeadline).Error)
	require.NoError(t, DB.Model(&UserReward{}).Where("campaign_id = ?", campaign.Id).Update("expires_at", pastDeadline).Error)
	retried, err := CreateRewardCampaign(input, first.Id)
	require.NoError(t, err)
	assert.Equal(t, campaign.Id, retried.Id)
	assert.Equal(t, pastDeadline, retried.ExpiresAt)
	require.NoError(t, DB.Where("campaign_id = ?", campaign.Id).Find(&rewards).Error)
	require.Len(t, rewards, 2)
	for _, reward := range rewards {
		assert.Equal(t, pastDeadline, reward.ExpiresAt)
	}
	input.ExpiresInDays = 8
	_, err = CreateRewardCampaign(input, first.Id)
	assert.ErrorIs(t, err, ErrRewardRequestConflict)
}

func TestRewardExpiredFixedAndRandomClaimsNeverCreditBalanceOrChangeLedger(t *testing.T) {
	for _, rewardType := range []string{"fixed", "random"} {
		t.Run(rewardType, func(t *testing.T) {
			setupRewardTestDB(t)
			useUserCacheMiniRedis(t)
			user := createReserveTestUser(t, 100)
			require.NoError(t, populateUserCache(user))
			input := rewardTestCampaignInput("expired-claim")
			input.Type = rewardType
			if rewardType == "random" {
				input.MinQuota, input.MaxQuota = 7, 11
			}
			reward := createRewardForTestUser(t, user, input)
			require.NoError(t, DB.Model(&reward).Update("expires_at", common.GetTimestamp()).Error)
			_, fresh, err := ClaimUserReward(user.Id, reward.Id)
			assert.ErrorIs(t, err, ErrRewardExpired)
			assert.False(t, fresh)
			assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
			cached, err := cacheGetUserBase(user.Id)
			require.NoError(t, err)
			assert.Equal(t, 100, cached.Quota)
			var persisted UserReward
			require.NoError(t, DB.First(&persisted, reward.Id).Error)
			assert.Equal(t, RewardStatusPending, persisted.Status, "expiration is derived without rewriting the credit ledger")
			assert.Zero(t, persisted.QuotaAwarded)
			assert.Zero(t, persisted.ClaimedAt)
			campaigns, total, err := ListRewardCampaigns(1, 10)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, campaigns, 1)
			assert.Zero(t, campaigns[0].ClaimedCount)
			assert.Zero(t, campaigns[0].ClaimedQuota)
		})
	}
}

func TestRewardExpiryListsAndSummaryPreserveLegacyAndClaimedRewards(t *testing.T) {
	setupRewardTestDB(t)
	user := createReserveTestUser(t, 100)
	future := createRewardForTestUser(t, user, rewardTestCampaignInput("future-fixed"))
	expired := createRewardForTestUser(t, user, rewardTestCampaignInput("expired-fixed"))
	legacy := createRewardForTestUser(t, user, rewardTestCampaignInput("legacy-fixed"))
	claimed := createRewardForTestUser(t, user, rewardTestCampaignInput("claimed-before-expiry"))
	randomInput := rewardTestCampaignInput("future-random")
	randomInput.Type, randomInput.MinQuota, randomInput.MaxQuota = "random", 7, 11
	randomFuture := createRewardForTestUser(t, user, randomInput)
	randomInput.RequestID = "expired-random"
	randomExpired := createRewardForTestUser(t, user, randomInput)
	claimedReward, fresh, err := ClaimUserReward(user.Id, claimed.Id)
	require.NoError(t, err)
	assert.True(t, fresh)
	deadline := common.GetTimestamp()
	require.NoError(t, DB.Model(&UserReward{}).Where("id IN ?", []int{future.Id, randomFuture.Id}).Update("expires_at", deadline+3600).Error)
	require.NoError(t, DB.Model(&expired).Update("expires_at", deadline).Error)
	require.NoError(t, DB.Model(&randomExpired).Update("expires_at", deadline-1).Error)
	require.NoError(t, DB.Model(&legacy).Update("expires_at", 0).Error)
	require.NoError(t, DB.Model(&claimed).Update("expires_at", deadline-1).Error)

	summary, err := GetUserRewardSummary(user.Id)
	require.NoError(t, err)
	assert.Equal(t, RewardSummary{FixedCount: 2, RandomCount: 1, PendingCount: 3, FixedQuota: 50}, *summary)
	for _, tc := range []struct {
		status string
		ids    []int
	}{
		{"pending", []int{future.Id, legacy.Id, randomFuture.Id}},
		{"expired", []int{expired.Id, randomExpired.Id}},
		{"claimed", []int{claimed.Id}},
	} {
		items, total, listErr := ListUserRewards(user.Id, tc.status, "", 1, 10)
		require.NoError(t, listErr)
		assert.EqualValues(t, len(tc.ids), total)
		ids := make([]int, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.Id)
			assert.Equal(t, tc.status, item.Status)
		}
		assert.ElementsMatch(t, tc.ids, ids)
	}
	all, total, err := ListUserRewards(user.Id, "", "", 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 6, total)
	statuses := make(map[int]string, len(all))
	for _, item := range all {
		statuses[item.Id] = item.Status
	}
	assert.Equal(t, RewardStatusExpired, statuses[expired.Id])
	assert.Equal(t, RewardStatusExpired, statuses[randomExpired.Id])
	assert.Equal(t, RewardStatusClaimed, statuses[claimed.Id])
	assert.Equal(t, RewardStatusPending, statuses[legacy.Id])

	retried, fresh, err := ClaimUserReward(user.Id, claimed.Id)
	require.NoError(t, err)
	assert.False(t, fresh)
	assert.Equal(t, claimedReward.QuotaAwarded, retried.QuotaAwarded)
	assert.Equal(t, claimedReward.ClaimedAt, retried.ClaimedAt)
	assert.Equal(t, RewardStatusClaimed, retried.Status)
	assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id))
	_, fresh, err = ClaimUserReward(user.Id, legacy.Id)
	require.NoError(t, err)
	assert.True(t, fresh, "legacy zero deadlines remain claimable")
	assert.Equal(t, 150, getUserQuotaFromDB(t, user.Id))
}
