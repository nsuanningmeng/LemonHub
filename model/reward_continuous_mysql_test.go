package model

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Independent deployed v0.5.8 schemas: creating current models before inserting
// these rows would conceal a migration that accidentally activates old awards.
type rewardCampaignV058 struct {
	Id             int    `gorm:"primaryKey"`
	Title          string `gorm:"type:varchar(480);not null"`
	Description    string `gorm:"type:text"`
	Type           string `gorm:"type:varchar(16);not null"`
	MinQuota       int    `gorm:"type:bigint;not null"`
	MaxQuota       int    `gorm:"type:bigint;not null"`
	ExpiresInDays  int    `gorm:"not null;default:0"`
	ExpiresAt      int64  `gorm:"type:bigint;not null;default:0"`
	Audience       string `gorm:"type:varchar(16);not null"`
	StartTime      int64  `gorm:"type:bigint;not null"`
	EndTime        int64  `gorm:"type:bigint;not null"`
	RequestID      string `gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedBy      int    `gorm:"not null"`
	CreatedAt      int64  `gorm:"type:bigint;not null"`
	RecipientCount int    `gorm:"not null"`
}

func (rewardCampaignV058) TableName() string { return "reward_campaigns" }

type userRewardV058 struct {
	Id           int    `gorm:"primaryKey"`
	CampaignID   int    `gorm:"not null;uniqueIndex:idx_reward_campaign_user,priority:1"`
	UserID       int    `gorm:"not null;uniqueIndex:idx_reward_campaign_user,priority:2;index:idx_reward_user_status,priority:1"`
	Title        string `gorm:"type:varchar(480);not null"`
	Description  string `gorm:"type:text"`
	Type         string `gorm:"type:varchar(16);not null"`
	MinQuota     int    `gorm:"type:bigint;not null"`
	MaxQuota     int    `gorm:"type:bigint;not null"`
	QuotaAwarded int    `gorm:"type:bigint;not null"`
	Status       string `gorm:"type:varchar(16);not null;index:idx_reward_user_status,priority:2"`
	CreatedAt    int64  `gorm:"type:bigint;not null"`
	ClaimedAt    int64  `gorm:"type:bigint;not null"`
	ExpiresAt    int64  `gorm:"type:bigint;not null;default:0"`
}

func (userRewardV058) TableName() string { return "user_rewards" }

func migrateRewardContinuousMySQLFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	connection, err := db.DB()
	require.NoError(t, err)
	// Register before migrating so a partial migration is cleaned up as well.
	t.Cleanup(func() {
		rewardActivityStores.Lock()
		delete(rewardActivityStores.ready, connection)
		rewardActivityStores.Unlock()
		require.NoError(t, db.Migrator().DropTable(&RewardActivityEvent{}, &RewardDistributionCandidate{}, &RewardDistributionState{}))
	})
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &RewardCampaign{}, &UserReward{}, &RewardDistributionState{}, &RewardDistributionCandidate{}))
	require.NoError(t, MigrateRewardActivityEvents(db))
	require.NoError(t, InitializeRewardActivityCapture())
}

func continuousMySQLInput(requestID, audience string, start, end int64) RewardCampaignInput {
	input := rewardTestCampaignInput(requestID)
	input.Continuous, input.Audience = true, audience
	input.StartTime, input.EndTime = start, end
	return input
}

// The bound is a failing-test guard, not a sleep or a claim that an active
// campaign can never receive more events. Every pass reloads durable cursors.
func drainRewardContinuousMySQL(t *testing.T, db *gorm.DB, batch int) {
	t.Helper()
	for range 40 {
		_, err := RunRewardDistributionPass(context.Background(), batch)
		require.NoError(t, err)
		var unfinished int64
		require.NoError(t, db.Model(&RewardDistributionState{}).Where("initialized = ? OR topup_done = ? OR usage_done = ?", false, false, false).Count(&unfinished).Error)
		events, err := ListPendingRewardActivityEvents(context.Background(), 1)
		require.NoError(t, err)
		if unfinished == 0 && len(events) == 0 {
			return
		}
	}
	t.Fatal("continuous reward backfill or committed source events did not drain")
}

func TestMySQLRewardContinuousMigrationPreservesIssuedV058Rewards(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &rewardCampaignV058{}, &userRewardV058{}))
	now := common.GetTimestamp()
	claimed := createReserveTestUser(t, 125)
	pending := createReserveTestUser(t, 100)
	expired := createReserveTestUser(t, 100)
	unlimited := createReserveTestUser(t, 100)
	randomClaimed := createReserveTestUser(t, 109)
	newcomer := createReserveTestUser(t, 100)
	campaigns := []rewardCampaignV058{
		{Id: 11, Title: "已发奖励金", Description: "真实旧活动必须保留", Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, ExpiresInDays: 7, ExpiresAt: now + 6*86400, Audience: "usage", StartTime: now - 86400, EndTime: now + 7*86400, RequestID: "v058-fixed", CreatedBy: claimed.Id, CreatedAt: now - 86400, RecipientCount: 2},
		{Id: 12, Title: "已过期奖励", Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, ExpiresInDays: 1, ExpiresAt: now - 86400, Audience: "either", StartTime: now - 3*86400, EndTime: now + 86400, RequestID: "v058-expired", CreatedBy: claimed.Id, CreatedAt: now - 2*86400, RecipientCount: 1},
		{Id: 13, Title: "无到期时间旧奖励", Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Audience: "usage", StartTime: now - 86400, EndTime: now + 86400, RequestID: "v058-unlimited", CreatedBy: claimed.Id, CreatedAt: now - 86400, RecipientCount: 1},
		{Id: 14, Title: "已经拆开的红包", Type: RewardTypeRandom, MinQuota: 7, MaxQuota: 11, ExpiresInDays: 1, ExpiresAt: now - 86400, Audience: "either", StartTime: now - 3*86400, EndTime: now + 86400, RequestID: "v058-random", CreatedBy: claimed.Id, CreatedAt: now - 2*86400, RecipientCount: 1},
	}
	rewards := []userRewardV058{
		{Id: 101, CampaignID: 11, UserID: claimed.Id, Title: campaigns[0].Title, Description: campaigns[0].Description, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, QuotaAwarded: 25, Status: RewardStatusClaimed, CreatedAt: campaigns[0].CreatedAt, ClaimedAt: now - 3600, ExpiresAt: campaigns[0].ExpiresAt},
		{Id: 102, CampaignID: 11, UserID: pending.Id, Title: campaigns[0].Title, Description: campaigns[0].Description, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: RewardStatusPending, CreatedAt: campaigns[0].CreatedAt, ExpiresAt: campaigns[0].ExpiresAt},
		{Id: 103, CampaignID: 12, UserID: expired.Id, Title: campaigns[1].Title, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: RewardStatusPending, CreatedAt: campaigns[1].CreatedAt, ExpiresAt: campaigns[1].ExpiresAt},
		{Id: 104, CampaignID: 13, UserID: unlimited.Id, Title: campaigns[2].Title, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: RewardStatusPending, CreatedAt: campaigns[2].CreatedAt},
		{Id: 105, CampaignID: 14, UserID: randomClaimed.Id, Title: campaigns[3].Title, Type: RewardTypeRandom, MinQuota: 7, MaxQuota: 11, QuotaAwarded: 9, Status: RewardStatusClaimed, CreatedAt: campaigns[3].CreatedAt, ClaimedAt: now - 90000, ExpiresAt: campaigns[3].ExpiresAt},
	}
	require.NoError(t, db.Create(&campaigns).Error)
	require.NoError(t, db.Create(&rewards).Error)
	require.False(t, db.Migrator().HasColumn(&rewardCampaignV058{}, "continuous"))
	// A deployed already-claimed reward is idempotent before the upgrade, too.
	_, fresh, err := ClaimUserReward(claimed.Id, rewards[0].Id)
	require.NoError(t, err)
	require.False(t, fresh)
	migrateRewardContinuousMySQLFixture(t, db)
	// A second startup must neither change old rows nor turn on distribution.
	require.NoError(t, db.AutoMigrate(&RewardCampaign{}, &UserReward{}, &RewardDistributionState{}, &RewardDistributionCandidate{}))
	require.NoError(t, MigrateRewardActivityEvents(db))
	require.NoError(t, createLog(&Log{UserId: newcomer.Id, Type: LogTypeConsume, CreatedAt: now}))
	drainRewardContinuousMySQL(t, db, 2)
	var afterCampaigns []rewardCampaignV058
	var afterRewards []userRewardV058
	require.NoError(t, db.Order("id").Find(&afterCampaigns).Error)
	require.NoError(t, db.Order("id").Find(&afterRewards).Error)
	assert.Equal(t, campaigns, afterCampaigns, "upgrade and worker must preserve every deployed campaign field")
	assert.Equal(t, rewards, afterRewards, "amounts, recipients, deadlines, and claimed ledger remain byte-for-byte equivalent")
	var continuous, states, candidates int64
	require.NoError(t, db.Model(&RewardCampaign{}).Where("continuous = ?", true).Count(&continuous).Error)
	require.NoError(t, db.Model(&RewardDistributionState{}).Count(&states).Error)
	require.NoError(t, db.Model(&RewardDistributionCandidate{}).Count(&candidates).Error)
	assert.Zero(t, continuous)
	assert.Zero(t, states)
	assert.Zero(t, candidates)
	for _, user := range []User{claimed, pending, expired, unlimited, randomClaimed, newcomer} {
		assert.Equal(t, user.Quota, getUserQuotaFromDB(t, user.Id))
	}
	listed, total, err := ListRewardCampaigns(1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 4, total)
	for _, campaign := range listed {
		assert.False(t, campaign.Continuous)
		assert.Equal(t, "legacy", campaign.DistributionStatus)
		if campaign.Id == 11 {
			assert.Equal(t, 2, campaign.RecipientCount)
			assert.EqualValues(t, 1, campaign.ClaimedCount)
			assert.EqualValues(t, 25, campaign.ClaimedQuota)
		}
	}
	_, fresh, err = ClaimUserReward(pending.Id, rewards[1].Id)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.Equal(t, 125, getUserQuotaFromDB(t, pending.Id))
	_, _, err = ClaimUserReward(expired.Id, rewards[2].Id)
	require.ErrorIs(t, err, ErrRewardExpired)
	assert.Equal(t, 100, getUserQuotaFromDB(t, expired.Id))
	var expiredAfter userRewardV058
	require.NoError(t, db.First(&expiredAfter, rewards[2].Id).Error)
	assert.Equal(t, rewards[2], expiredAfter, "expired failure cannot mutate the old ledger")
	_, fresh, err = ClaimUserReward(unlimited.Id, rewards[3].Id)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.Equal(t, 125, getUserQuotaFromDB(t, unlimited.Id))
	// Empty entropy would fail if an old claimed envelope were drawn again.
	previousReader := cryptorand.Reader
	cryptorand.Reader = bytes.NewReader(nil)
	t.Cleanup(func() { cryptorand.Reader = previousReader })
	for _, reward := range []userRewardV058{rewards[0], rewards[4]} {
		retried, isFresh, claimErr := ClaimUserReward(reward.UserID, reward.Id)
		require.NoError(t, claimErr)
		assert.False(t, isFresh)
		assert.Equal(t, reward.QuotaAwarded, retried.QuotaAwarded)
		assert.Equal(t, reward.ClaimedAt, retried.ClaimedAt)
	}
	assert.Equal(t, 125, getUserQuotaFromDB(t, claimed.Id))
	assert.Equal(t, 109, getUserQuotaFromDB(t, randomClaimed.Id))
	input := continuousMySQLInput("new-after-upgrade", "usage", now-60, now+86400)
	newCampaign, err := CreateRewardCampaign(input, claimed.Id)
	require.NoError(t, err)
	drainRewardContinuousMySQL(t, db, 2)
	var newReward UserReward
	require.NoError(t, db.First(&newReward, "campaign_id = ? AND user_id = ?", newCampaign.Id, newcomer.Id).Error)
	assert.Equal(t, input.MinQuota, newReward.MinQuota)
	assert.GreaterOrEqual(t, newReward.CreatedAt, now)
	assert.Equal(t, newReward.CreatedAt+int64(input.ExpiresInDays)*86400, newReward.ExpiresAt)
	require.NoError(t, db.Where("id <= ?", 14).Order("id").Find(&afterCampaigns).Error)
	assert.Equal(t, campaigns, afterCampaigns, "new activities must not rewrite old recipient counts or deadlines")
}

func TestMySQLRewardContinuousAudienceAndLatePendingTopup(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	admin := createReserveTestUser(t, 0)
	both := createReserveTestUser(t, 0)
	usageOnly := createReserveTestUser(t, 0)
	topupOnly := createReserveTestUser(t, 0)
	disabled := createReserveTestUser(t, 0)
	foreign := createReserveTestUser(t, 0)
	require.NoError(t, db.Model(&disabled).Update("status", common.UserStatusDisabled).Error)
	require.NoError(t, db.Model(&foreign).Update("site_id", 7).Error)
	orders := []TopUp{
		{UserId: both.Id, TradeNo: "old-pending-both", Status: common.TopUpStatusPending, Amount: 1},
		{UserId: topupOnly.Id, TradeNo: "old-pending-topup", Status: common.TopUpStatusPending, Amount: 1},
	}
	require.NoError(t, db.Create(&orders).Error)
	campaigns := make(map[string]*RewardCampaign)
	for _, audience := range []string{"topup", "usage", "both", "either"} {
		input := continuousMySQLInput("four-audiences-"+audience, audience, now-60, now+3600)
		// A stale optional preview must not block submitting the actual activity.
		stalePreview := 999
		input.ExpectedRecipientCount = &stalePreview
		campaign, err := CreateRewardCampaign(input, admin.Id)
		require.NoError(t, err)
		require.Zero(t, campaign.RecipientCount)
		campaigns[audience] = campaign
	}
	drainRewardContinuousMySQL(t, db, 2)
	for _, user := range []User{both, usageOnly, disabled, foreign} {
		require.NoError(t, createLog(&Log{UserId: user.Id, Type: LogTypeConsume, CreatedAt: now}))
	}
	drainRewardContinuousMySQL(t, db, 2)
	var bothBefore int64
	require.NoError(t, db.Model(&UserReward{}).Where("campaign_id = ?", campaigns["both"].Id).Count(&bothBefore).Error)
	assert.Zero(t, bothBefore, "AND must wait for independently arriving successful recharge")
	for _, order := range orders {
		require.NoError(t, ManualCompleteTopUp(order.TradeNo, "127.0.0.1"))
	}
	drainRewardContinuousMySQL(t, db, 2)
	want := map[string][]int{
		"topup": {both.Id, topupOnly.Id}, "usage": {both.Id, usageOnly.Id},
		"both": {both.Id}, "either": {both.Id, usageOnly.Id, topupOnly.Id},
	}
	for audience, expected := range want {
		var ids []int
		require.NoError(t, db.Model(&UserReward{}).Where("campaign_id = ?", campaigns[audience].Id).Order("user_id").Pluck("user_id", &ids).Error)
		assert.ElementsMatch(t, expected, ids, audience)
		var stored RewardCampaign
		require.NoError(t, db.First(&stored, campaigns[audience].Id).Error)
		assert.Equal(t, len(expected), stored.RecipientCount, audience)
	}
	// Replaying source facts and settlement cannot duplicate the OR recipient.
	require.NoError(t, ManualCompleteTopUp(orders[0].TradeNo, "127.0.0.1"))
	require.NoError(t, createLog(&Log{UserId: both.Id, Type: LogTypeConsume, CreatedAt: now}))
	drainRewardContinuousMySQL(t, db, 2)
	var total int64
	require.NoError(t, db.Model(&UserReward{}).Count(&total).Error)
	assert.EqualValues(t, 8, total)
	assert.Equal(t, int(common.QuotaPerUnit), getUserQuotaFromDB(t, both.Id), "distribution alone cannot credit the reward balance")
	assert.Zero(t, getUserQuotaFromDB(t, usageOnly.Id))
}

func TestMySQLRewardContinuousBoundariesAndDelayedOutboxRecovery(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	admin := createReserveTestUser(t, 0)
	start, end := now-200, now-100
	campaign, err := CreateRewardCampaign(continuousMySQLInput("delayed-source", "usage", start, end), admin.Id)
	require.NoError(t, err)
	drainRewardContinuousMySQL(t, db, 2)
	cases := []struct {
		name string
		at   int64
		kind int
		site int
		want bool
	}{
		{"before", start - 1, LogTypeConsume, 0, false},
		{"start", start, LogTypeConsume, 0, true},
		{"last-second", end - 1, LogTypeConsume, 0, true},
		{"end", end, LogTypeConsume, 0, false},
		{"failed", start + 1, LogTypeError, 0, false},
		{"foreign-log", start + 1, LogTypeConsume, 3, false},
	}
	var expected []int
	for _, tc := range cases {
		user := createReserveTestUser(t, 0)
		require.NoError(t, createLog(&Log{UserId: user.Id, Type: tc.kind, SiteId: tc.site, CreatedAt: tc.at, RequestId: tc.name}))
		if tc.want {
			expected = append(expected, user.Id)
		}
	}
	events, err := ListPendingRewardActivityEvents(context.Background(), 20)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	// Model a worker loss after durable recipient commit and before ACK. Startup
	// and a subsequent pass reload DB evidence; replay is harmless after the end.
	for _, event := range events {
		require.NoError(t, ProcessRewardActivityEvent(context.Background(), event.Event))
	}
	require.NoError(t, InitializeRewardActivityCapture())
	drainRewardContinuousMySQL(t, db, 2)
	var rewards []UserReward
	require.NoError(t, db.Where("campaign_id = ?", campaign.Id).Find(&rewards).Error)
	var ids []int
	for _, reward := range rewards {
		ids = append(ids, reward.UserID)
		assert.GreaterOrEqual(t, reward.CreatedAt, now)
		assert.Equal(t, reward.CreatedAt+7*86400, reward.ExpiresAt)
	}
	assert.ElementsMatch(t, expected, ids, "late processing must use original successful event time and [start,end)")
	var stored RewardCampaign
	require.NoError(t, db.First(&stored, campaign.Id).Error)
	assert.Equal(t, 2, stored.RecipientCount)
}

func TestMySQLRewardContinuousBackfillResumesBeyondBatch(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	admin := createReserveTestUser(t, 0)
	var users []int
	// Direct inserts represent existing history from before outbox installation.
	// Equal timestamps force the persisted secondary cursor to do real work.
	for i := range 7 {
		user := createReserveTestUser(t, 0)
		users = append(users, user.Id)
		require.NoError(t, db.Create(&Log{UserId: user.Id, Type: LogTypeConsume, CreatedAt: now - 1, RequestId: fmt.Sprintf("history-%d", i)}).Error)
	}
	campaign, err := CreateRewardCampaign(continuousMySQLInput("resume-history", "usage", now-100, now+3600), admin.Id)
	require.NoError(t, err)
	require.NoError(t, ProcessRewardCampaignBatch(context.Background(), campaign.Id, 2))
	var first []UserReward
	require.NoError(t, db.Where("campaign_id = ?", campaign.Id).Find(&first).Error)
	require.Len(t, first, 2, "one backfill page must bound its historical scan")
	var interruptedState RewardDistributionState
	require.NoError(t, db.First(&interruptedState, "campaign_id = ?", campaign.Id).Error)
	assert.False(t, interruptedState.UsageDone)
	require.NoError(t, InitializeRewardActivityCapture())
	drainRewardContinuousMySQL(t, db, 2)
	var ids []int
	require.NoError(t, db.Model(&UserReward{}).Where("campaign_id = ?", campaign.Id).Pluck("user_id", &ids).Error)
	assert.ElementsMatch(t, users, ids, "the page size cannot become a campaign recipient limit")
	var after []UserReward
	require.NoError(t, db.Where("id IN ?", []int{first[0].Id, first[1].Id}).Order("id").Find(&after).Error)
	assert.Equal(t, first, after, "resuming must not refresh deadlines of already issued records")
	var stored RewardCampaign
	require.NoError(t, db.First(&stored, campaign.Id).Error)
	assert.Equal(t, len(users), stored.RecipientCount)
}

func TestMySQLRewardContinuousPersonalDeadlineAndFutureCreation(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	admin := createReserveTestUser(t, 0)
	futureInput := continuousMySQLInput("future-empty", "usage", now+3600, now+7200)
	future, err := CreateRewardCampaign(futureInput, admin.Id)
	require.NoError(t, err)
	assert.Equal(t, "scheduled", future.DistributionStatus)
	assert.Zero(t, future.RecipientCount)
	_, err = RunRewardDistributionPass(context.Background(), 2)
	require.NoError(t, err)
	var futureRewards int64
	require.NoError(t, db.Model(&UserReward{}).Where("campaign_id = ?", future.Id).Count(&futureRewards).Error)
	assert.Zero(t, futureRewards)
	input := continuousMySQLInput("personal-expiry", "usage", now-10*86400, now+86400)
	input.ExpiresInDays = 2
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	// A campaign may outlive many individual claim windows. Its creation time
	// must not be the deadline origin for recipients arriving later.
	require.NoError(t, db.Model(campaign).Update("created_at", now-10*86400).Error)
	for i, grantedAt := range []int64{now, now + 3600} {
		user := createReserveTestUser(t, 0)
		candidate := RewardDistributionCandidate{CampaignID: campaign.Id, UserID: user.Id, HasUsage: true, UsageOccurredAt: now - 1}
		require.NoError(t, db.Create(&candidate).Error)
		require.NoError(t, grantRewardDistributionCandidate(context.Background(), campaign.Id, user.Id, func() int64 { return grantedAt }))
		var reward UserReward
		require.NoError(t, db.First(&reward, "campaign_id = ? AND user_id = ?", campaign.Id, user.Id).Error)
		assert.Equal(t, grantedAt, reward.CreatedAt)
		assert.Equal(t, grantedAt+2*86400, reward.ExpiresAt)
		_, _, claimErr := claimUserReward(user.Id, reward.Id, func() int64 { return reward.ExpiresAt })
		require.ErrorIs(t, claimErr, ErrRewardExpired, "recipient %d exact deadline", i)
		assert.Zero(t, getUserQuotaFromDB(t, user.Id))
		var unchanged UserReward
		require.NoError(t, db.First(&unchanged, reward.Id).Error)
		assert.Equal(t, reward, unchanged)
	}
	listed, _, err := ListRewardCampaigns(1, 10)
	require.NoError(t, err)
	for _, item := range listed {
		assert.Zero(t, item.ClaimedCount)
		assert.Zero(t, item.ClaimedQuota)
	}
}

func TestMySQLRewardContinuousConcurrentGrantAndReplayCreditOnce(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	user := createReserveTestUser(t, 100)
	campaign, err := CreateRewardCampaign(continuousMySQLInput("concurrent-grant", "usage", now-60, now+3600), user.Id)
	require.NoError(t, err)
	candidate := RewardDistributionCandidate{CampaignID: campaign.Id, UserID: user.Id, HasUsage: true, UsageOccurredAt: now}
	require.NoError(t, db.Create(&candidate).Error)
	locked, released := make(chan struct{}), make(chan struct{})
	connections := make(chan uint64, 2)
	var lockOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	const beforeCallback = "test:continuous_grant_connection"
	const afterCallback = "test:continuous_grant_locked"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(beforeCallback, func(tx *gorm.DB) {
		if tx.Statement.Table != "reward_distribution_candidates" {
			return
		}
		if _, locking := tx.Statement.Clauses["FOR"]; !locking {
			return
		}
		var connection uint64
		if queryErr := tx.Statement.ConnPool.QueryRowContext(tx.Statement.Context, "SELECT CONNECTION_ID()").Scan(&connection); queryErr != nil {
			tx.AddError(queryErr)
			return
		}
		connections <- connection
	}))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(afterCallback, func(tx *gorm.DB) {
		if tx.Error != nil || tx.Statement.Table != "reward_distribution_candidates" {
			return
		}
		if _, locking := tx.Statement.Clauses["FOR"]; !locking {
			return
		}
		lockOnce.Do(func() {
			close(locked)
			select {
			case <-released:
			case <-time.After(10 * time.Second):
				tx.AddError(fmt.Errorf("timed out holding candidate lock"))
			}
		})
	}))
	var removeOnce sync.Once
	removeCallbacks := func() {
		removeOnce.Do(func() {
			require.NoError(t, db.Callback().Query().Remove(beforeCallback))
			require.NoError(t, db.Callback().Query().Remove(afterCallback))
		})
	}
	t.Cleanup(func() { release(); removeCallbacks() })
	var workers sync.WaitGroup
	t.Cleanup(func() { release(); workers.Wait() })
	results := make(chan error, 2)
	grant := func() {
		defer workers.Done()
		results <- grantRewardDistributionCandidate(context.Background(), campaign.Id, user.Id, common.GetTimestamp)
	}
	workers.Add(1)
	go grant()
	waitRewardMySQLBarrier(t, locked)
	first := <-connections
	workers.Add(1)
	go grant()
	select {
	case second := <-connections:
		assert.NotEqual(t, first, second, "real competing MySQL transactions need separate connections")
	case <-time.After(10 * time.Second):
		t.Fatal("competing grant did not start")
	}
	release()
	for range 2 {
		require.NoError(t, <-results)
	}
	// Remove callbacks before the recovery worker makes further candidate reads.
	removeCallbacks()
	for range 2 {
		require.NoError(t, ProcessRewardActivityEvent(context.Background(), RewardActivityEvent{Kind: "usage", UserID: user.Id, OccurredAt: now, Confirmed: true}))
	}
	drainRewardContinuousMySQL(t, db, 2)
	var rewards []UserReward
	require.NoError(t, db.Where("campaign_id = ?", campaign.Id).Find(&rewards).Error)
	require.Len(t, rewards, 1)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	_, fresh, err := ClaimUserReward(user.Id, rewards[0].Id)
	require.NoError(t, err)
	assert.True(t, fresh)
	_, fresh, err = ClaimUserReward(user.Id, rewards[0].Id)
	require.NoError(t, err)
	assert.False(t, fresh)
	assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id))
	listed, total, err := ListRewardCampaigns(1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, listed, 1)
	assert.Equal(t, 1, listed[0].RecipientCount)
	assert.EqualValues(t, 1, listed[0].ClaimedCount)
	assert.EqualValues(t, 25, listed[0].ClaimedQuota)
}

func TestMySQLRewardContinuousConcurrentCreationUsesOneActivity(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	admin := createReserveTestUser(t, 0)
	input := continuousMySQLInput("same-continuous-request", "usage", now+3600, now+7200)
	connections := make(chan uint64, 2)
	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	const callback = "test:continuous_create_transactions"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "reward_campaigns" {
			return
		}
		var id uint64
		if err := tx.Statement.ConnPool.QueryRowContext(tx.Statement.Context, "SELECT CONNECTION_ID()").Scan(&id); err != nil {
			tx.AddError(err)
			return
		}
		connections <- id
		select {
		case <-released:
		case <-time.After(10 * time.Second):
			tx.AddError(fmt.Errorf("timed out synchronizing activity creation"))
		}
	}))
	t.Cleanup(func() { release(); require.NoError(t, db.Callback().Create().Remove(callback)) })
	type result struct {
		campaign *RewardCampaign
		err      error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	t.Cleanup(func() { release(); workers.Wait() })
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			campaign, err := CreateRewardCampaign(input, admin.Id)
			results <- result{campaign, err}
		}()
	}
	var ids []uint64
	for range 2 {
		select {
		case id := <-connections:
			ids = append(ids, id)
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent activity creation did not acquire both connections")
		}
	}
	assert.NotEqual(t, ids[0], ids[1])
	release()
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.campaign)
	require.NotNil(t, second.campaign)
	assert.Equal(t, first.campaign.Id, second.campaign.Id)
	var activities, states, rewards int64
	require.NoError(t, db.Model(&RewardCampaign{}).Count(&activities).Error)
	require.NoError(t, db.Model(&RewardDistributionState{}).Count(&states).Error)
	require.NoError(t, db.Model(&UserReward{}).Count(&rewards).Error)
	assert.EqualValues(t, 1, activities)
	assert.EqualValues(t, 1, states)
	assert.Zero(t, rewards)
	input.ExpiresInDays++
	_, err := CreateRewardCampaign(input, admin.Id)
	require.ErrorIs(t, err, ErrRewardRequestConflict, "idempotency must not accept a changed per-user expiry")
}

func TestMySQLRewardContinuousLowerIDEventCommittedLaterIsNotLost(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	first := createReserveTestUser(t, 0)
	second := createReserveTestUser(t, 0)
	campaign, err := CreateRewardCampaign(continuousMySQLInput("late-commit", "usage", now-60, now+3600), first.Id)
	require.NoError(t, err)
	drainRewardContinuousMySQL(t, db, 2)
	reserved := make(chan RewardActivityEvent, 1)
	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	const callback = "test:reward_lower_id_uncommitted"
	require.NoError(t, db.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		event, ok := tx.Statement.Dest.(*RewardActivityEvent)
		if !ok || tx.Error != nil || event.RequestID != "delayed-low-id" {
			return
		}
		reserved <- *event
		select {
		case <-released:
		case <-time.After(10 * time.Second):
			tx.AddError(fmt.Errorf("timed out holding source transaction"))
		}
	}))
	t.Cleanup(func() { release(); require.NoError(t, db.Callback().Create().Remove(callback)) })
	var worker sync.WaitGroup
	t.Cleanup(func() { release(); worker.Wait() })
	result := make(chan error, 1)
	worker.Add(1)
	go func() {
		defer worker.Done()
		result <- createLog(&Log{UserId: first.Id, CreatedAt: now, Type: LogTypeConsume, RequestId: "delayed-low-id"})
	}()
	var earlier RewardActivityEvent
	select {
	case earlier = <-reserved:
	case <-time.After(10 * time.Second):
		t.Fatal("first source did not reserve its outbox ID")
	}
	require.NoError(t, createLog(&Log{UserId: second.Id, CreatedAt: now, Type: LogTypeConsume, RequestId: "committed-high-id"}))
	visible, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, visible, 1, "the uncommitted source must not become eligible")
	assert.Greater(t, visible[0].Event.Id, earlier.Id)
	drainRewardContinuousMySQL(t, db, 2)
	var initially []int
	require.NoError(t, db.Model(&UserReward{}).Where("campaign_id = ?", campaign.Id).Pluck("user_id", &initially).Error)
	assert.Equal(t, []int{second.Id}, initially)
	release()
	require.NoError(t, <-result)
	require.NoError(t, InitializeRewardActivityCapture())
	drainRewardContinuousMySQL(t, db, 2)
	var eventually []int
	require.NoError(t, db.Model(&UserReward{}).Where("campaign_id = ?", campaign.Id).Pluck("user_id", &eventually).Error)
	assert.ElementsMatch(t, []int{first.Id, second.Id}, eventually, "processing a higher ID cannot permanently skip a later commit of a lower ID")
	var stored RewardCampaign
	require.NoError(t, db.First(&stored, campaign.Id).Error)
	assert.Equal(t, 2, stored.RecipientCount)
}

func TestMySQLRewardContinuousClaimsKeepExactUncappedCampaignTotal(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	migrateRewardContinuousMySQLFixture(t, db)
	now := common.GetTimestamp()
	admin := createReserveTestUser(t, 0)
	input := continuousMySQLInput("uncapped-campaign-total", "usage", now-60, now+3600)
	input.MinQuota, input.MaxQuota = common.MaxWalletQuota, common.MaxWalletQuota
	campaign, err := CreateRewardCampaign(input, admin.Id)
	require.NoError(t, err)
	var users []User
	for range 3 {
		user := createReserveTestUser(t, 0)
		users = append(users, user)
		require.NoError(t, createLog(&Log{UserId: user.Id, Type: LogTypeConsume, CreatedAt: now}))
	}
	drainRewardContinuousMySQL(t, db, 2)
	for _, user := range users {
		var reward UserReward
		require.NoError(t, db.First(&reward, "campaign_id = ? AND user_id = ?", campaign.Id, user.Id).Error)
		_, fresh, claimErr := ClaimUserReward(user.Id, reward.Id)
		require.NoError(t, claimErr)
		assert.True(t, fresh)
		assert.Equal(t, common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))
	}
	listed, total, err := ListRewardCampaigns(1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, listed, 1)
	expected := int64(common.MaxWalletQuota) * 3
	assert.Equal(t, 3, listed[0].RecipientCount)
	assert.EqualValues(t, 3, listed[0].ClaimedCount)
	assert.Equal(t, expected, listed[0].ClaimedQuota)
	assert.Equal(t, strconv.FormatInt(expected, 10), listed[0].ClaimedQuotaExact,
		"SQL decimal SUM must not round campaign totals through a float or impose a single-wallet ceiling")
}
