package model

import (
	"bytes"
	cryptorand "crypto/rand"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Only an explicitly selected empty test schema may be modified. Cleanup is
// restricted to the five tables this fixture owns, never the database or volume.
func setupRewardMySQLTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_REWARD_TEST_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_REWARD_TEST_DSN to an empty lemonhub_rewards_test_* database")
	}
	config, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^lemonhub_rewards_test_[a-z0-9_]+$`), config.DBName)
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	sqlDB.SetMaxOpenConns(6)
	sqlDB.SetMaxIdleConns(4)
	var selected, version string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selected).Error)
	require.Equal(t, config.DBName, selected)
	require.NoError(t, db.Raw("SELECT VERSION()").Scan(&version).Error)
	t.Logf("reward integration database: MySQL %s", version)
	var tables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error)
	require.Zero(t, tables, "refusing to modify a nonempty database")

	oldDB, oldLogDB := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch
		require.NoError(t, db.Migrator().DropTable(&UserReward{}, &RewardCampaign{}, &TopUp{}, &Log{}, &User{}))
	})
	return db
}

// These are the deployed reward schemas before expiry fields were introduced.
// Keeping them independent of the current models prevents a migration test from
// silently creating the new columns before the old rows are inserted.
type rewardCampaignBeforeExpiry struct {
	Id             int    `gorm:"primaryKey"`
	Title          string `gorm:"type:varchar(480);not null"`
	Description    string `gorm:"type:text"`
	Type           string `gorm:"type:varchar(16);not null"`
	MinQuota       int    `gorm:"type:bigint;not null"`
	MaxQuota       int    `gorm:"type:bigint;not null"`
	Audience       string `gorm:"type:varchar(16);not null"`
	StartTime      int64  `gorm:"type:bigint;not null"`
	EndTime        int64  `gorm:"type:bigint;not null"`
	RequestID      string `gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedBy      int    `gorm:"not null"`
	CreatedAt      int64  `gorm:"type:bigint;not null"`
	RecipientCount int    `gorm:"not null"`
}

func (rewardCampaignBeforeExpiry) TableName() string { return "reward_campaigns" }

type userRewardBeforeExpiry struct {
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
}

func (userRewardBeforeExpiry) TableName() string { return "user_rewards" }

func TestMySQLRewardExpiryMigrationPreservesLegacyRows(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &rewardCampaignBeforeExpiry{}, &userRewardBeforeExpiry{}))
	first := createReserveTestUser(t, 100)
	second := createReserveTestUser(t, 6_000_000_003)
	third := createReserveTestUser(t, 200)
	campaigns := []rewardCampaignBeforeExpiry{
		{Id: 7, Title: "历史奖励金", Description: "保留原活动说明", Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25,
			Audience: "topup", StartTime: 100, EndTime: 200, RequestID: "legacy-fixed", CreatedBy: first.Id, CreatedAt: 300, RecipientCount: 1},
		{Id: 8, Title: "历史红包", Description: "保留大额账本", Type: RewardTypeRandom, MinQuota: 4_000_000_000, MaxQuota: 4_000_000_010,
			Audience: "both", StartTime: 100, EndTime: 200, RequestID: "legacy-random", CreatedBy: first.Id, CreatedAt: 301, RecipientCount: 2},
	}
	rewards := []userRewardBeforeExpiry{
		{Id: 101, CampaignID: 7, UserID: first.Id, Title: campaigns[0].Title, Description: "旧待领取记录", Type: RewardTypeFixed,
			MinQuota: 25, MaxQuota: 25, Status: RewardStatusPending, CreatedAt: 300},
		{Id: 102, CampaignID: 8, UserID: second.Id, Title: campaigns[1].Title, Description: "旧已领取记录", Type: RewardTypeRandom,
			MinQuota: 4_000_000_000, MaxQuota: 4_000_000_010, QuotaAwarded: 4_000_000_003, Status: RewardStatusClaimed, CreatedAt: 301, ClaimedAt: 400},
		{Id: 103, CampaignID: 8, UserID: third.Id, Title: campaigns[1].Title, Description: "旧待领取红包", Type: RewardTypeRandom,
			MinQuota: 4_000_000_000, MaxQuota: 4_000_000_010, Status: RewardStatusPending, CreatedAt: 301},
	}
	require.NoError(t, db.Create(&campaigns).Error)
	require.NoError(t, db.Create(&rewards).Error)
	for _, field := range []string{"expires_in_days", "expires_at"} {
		require.False(t, db.Migrator().HasColumn(&rewardCampaignBeforeExpiry{}, field))
	}
	require.False(t, db.Migrator().HasColumn(&userRewardBeforeExpiry{}, "expires_at"))
	for range 2 {
		require.NoError(t, db.AutoMigrate(&RewardCampaign{}, &UserReward{}))
		var oldCampaigns []rewardCampaignBeforeExpiry
		var oldRewards []userRewardBeforeExpiry
		require.NoError(t, db.Order("id").Find(&oldCampaigns).Error)
		require.NoError(t, db.Order("id").Find(&oldRewards).Error)
		assert.Equal(t, campaigns, oldCampaigns, "every pre-upgrade campaign field must survive")
		assert.Equal(t, rewards, oldRewards, "every pre-upgrade credit ledger field must survive")
		var currentCampaigns []RewardCampaign
		var currentRewards []UserReward
		require.NoError(t, db.Find(&currentCampaigns).Error)
		require.NoError(t, db.Find(&currentRewards).Error)
		for _, campaign := range currentCampaigns {
			assert.Zero(t, campaign.ExpiresInDays)
			assert.Zero(t, campaign.ExpiresAt)
		}
		for _, reward := range currentRewards {
			assert.Zero(t, reward.ExpiresAt)
		}
		assert.Equal(t, 100, getUserQuotaFromDB(t, first.Id))
		assert.Equal(t, 6_000_000_003, getUserQuotaFromDB(t, second.Id))
		assert.Equal(t, 200, getUserQuotaFromDB(t, third.Id))
	}
	claimed, fresh, err := ClaimUserReward(first.Id, 101)
	require.NoError(t, err)
	assert.True(t, fresh, "a migrated zero deadline must remain claimable")
	assert.Equal(t, 25, claimed.QuotaAwarded)
	assert.Equal(t, 125, getUserQuotaFromDB(t, first.Id))
	retried, fresh, err := ClaimUserReward(second.Id, 102)
	require.NoError(t, err)
	assert.False(t, fresh)
	assert.Equal(t, 4_000_000_003, retried.QuotaAwarded)
	assert.EqualValues(t, 400, retried.ClaimedAt)
	assert.Equal(t, 6_000_000_003, getUserQuotaFromDB(t, second.Id))
	listed, total, err := ListRewardCampaigns(1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, listed, 2)
	assert.Equal(t, 8, listed[0].Id)
	assert.Equal(t, 2, listed[0].RecipientCount)
	assert.EqualValues(t, 1, listed[0].ClaimedCount)
	assert.EqualValues(t, 4_000_000_003, listed[0].ClaimedQuota)
	assert.Equal(t, 7, listed[1].Id)
	assert.EqualValues(t, 1, listed[1].ClaimedCount)
	assert.EqualValues(t, 25, listed[1].ClaimedQuota)
}

func TestMySQLRewardConcurrentClaimsCreditOnlyOnce(t *testing.T) {
	for _, rewardType := range []string{RewardTypeFixed, RewardTypeRandom} {
		t.Run(rewardType, func(t *testing.T) {
			db := setupRewardMySQLTestDB(t)
			require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &RewardCampaign{}, &UserReward{}))
			user := createReserveTestUser(t, 100)
			input := rewardTestCampaignInput("mysql-concurrent-claim")
			input.Type = rewardType
			amount := 25
			if rewardType == RewardTypeRandom {
				input.MinQuota, input.MaxQuota, amount = 7, 11, 9
				previousReader := cryptorand.Reader
				cryptorand.Reader = bytes.NewReader([]byte{2})
				t.Cleanup(func() { cryptorand.Reader = previousReader })
			}
			reward := createRewardForTestUser(t, user, input)
			locked, release, connections := holdRewardMySQLUserLock(t, db)
			type claimResult struct {
				reward *UserReward
				fresh  bool
				err    error
			}
			done := make(chan claimResult, 2)
			var workers sync.WaitGroup
			// Registered after the barrier callbacks so workers finish before
			// callback removal, global DB restoration, and fixture teardown.
			t.Cleanup(func() {
				release()
				workers.Wait()
			})
			claim := func() {
				defer workers.Done()
				item, fresh, err := ClaimUserReward(user.Id, reward.Id)
				done <- claimResult{item, fresh, err}
			}
			workers.Add(1)
			go claim()
			waitRewardMySQLBarrier(t, locked)
			firstConnection := <-connections
			workers.Add(1)
			go claim()
			var secondConnection uint64
			select {
			case secondConnection = <-connections:
			case <-time.After(10 * time.Second):
				t.Fatal("the competing claim did not acquire its own connection")
			}
			assert.NotEqual(t, firstConnection, secondConnection, "competing claims must run on independent MySQL connections")
			release()
			freshClaims := 0
			for range 2 {
				result := <-done
				require.NoError(t, result.err)
				require.NotNil(t, result.reward)
				assert.Equal(t, amount, result.reward.QuotaAwarded)
				if result.fresh {
					freshClaims++
				}
			}
			assert.Equal(t, 1, freshClaims)
			assert.Equal(t, 100+amount, getUserQuotaFromDB(t, user.Id))
			campaigns, total, err := ListRewardCampaigns(1, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, campaigns, 1)
			assert.Equal(t, 1, campaigns[0].RecipientCount)
			assert.EqualValues(t, 1, campaigns[0].ClaimedCount)
			assert.EqualValues(t, amount, campaigns[0].ClaimedQuota)
			// An already committed claim must remain idempotent after expiry.
			require.NoError(t, db.Model(&reward).Update("expires_at", common.GetTimestamp()-1).Error)
			retried, fresh, err := ClaimUserReward(user.Id, reward.Id)
			require.NoError(t, err)
			assert.False(t, fresh)
			assert.Equal(t, amount, retried.QuotaAwarded)
			assert.Equal(t, 100+amount, getUserQuotaFromDB(t, user.Id))
		})
	}
}

// Hold the first transaction after it owns the user's actual row lock, and
// expose the independent connection used by the competing transaction.
func holdRewardMySQLUserLock(t *testing.T, db *gorm.DB) (<-chan struct{}, func(), <-chan uint64) {
	t.Helper()
	locked, released := make(chan struct{}), make(chan struct{})
	connections := make(chan uint64, 4)
	var first, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	const before = "test:reward_mysql_connection"
	const after = "test:reward_mysql_locked_user"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(before, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" || tx.Statement.Clauses["FOR"].Name == "" {
			return
		}
		var id uint64
		if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT CONNECTION_ID()").Scan(&id).Error; err != nil {
			tx.AddError(err)
			return
		}
		connections <- id
	}))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(after, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" || tx.Statement.Clauses["FOR"].Name == "" || tx.Error != nil {
			return
		}
		first.Do(func() {
			close(locked)
			select {
			case <-released:
			case <-time.After(10 * time.Second):
				tx.AddError(errors.New("reward transaction barrier timed out"))
			}
		})
	}))
	t.Cleanup(func() {
		release()
		require.NoError(t, db.Callback().Query().Remove(before))
		require.NoError(t, db.Callback().Query().Remove(after))
	})
	return locked, release, connections
}

func waitRewardMySQLBarrier(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("reward transaction did not reach its barrier")
	}
}

func TestMySQLRewardConcurrentIssuanceKeepsOneRecipientSnapshot(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &RewardCampaign{}, &UserReward{}))
	user := createReserveTestUser(t, 100)
	require.NoError(t, db.Create(&Log{UserId: user.Id, Type: LogTypeConsume, CreatedAt: 100}).Error)
	input := rewardTestCampaignInput("mysql-concurrent-issue")
	ready, released := make(chan struct{}, 2), make(chan struct{})
	var entered atomic.Int32
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	const callback = "test:reward_mysql_missing_request"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "reward_campaigns" || !errors.Is(tx.Error, gorm.ErrRecordNotFound) || entered.Add(1) > 2 {
			return
		}
		ready <- struct{}{}
		select {
		case <-released:
		case <-time.After(10 * time.Second):
			tx.AddError(errors.New("reward issuance barrier timed out"))
		}
	}))
	t.Cleanup(func() {
		release()
		require.NoError(t, db.Callback().Query().Remove(callback))
	})
	type issueResult struct {
		campaign *RewardCampaign
		err      error
	}
	done := make(chan issueResult, 2)
	var workers sync.WaitGroup
	t.Cleanup(func() {
		release()
		workers.Wait()
	})
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			campaign, err := CreateRewardCampaign(input, user.Id)
			done <- issueResult{campaign, err}
		}()
	}
	for range 2 {
		waitRewardMySQLBarrier(t, ready)
	}
	release()
	first, second := <-done, <-done
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.campaign)
	require.NotNil(t, second.campaign)
	assert.Equal(t, first.campaign.Id, second.campaign.Id)
	assert.Equal(t, first.campaign.ExpiresAt, second.campaign.ExpiresAt)
	var campaigns, rewards int64
	require.NoError(t, db.Model(&RewardCampaign{}).Count(&campaigns).Error)
	require.NoError(t, db.Model(&UserReward{}).Count(&rewards).Error)
	assert.EqualValues(t, 1, campaigns)
	assert.EqualValues(t, 1, rewards)
	assert.Equal(t, 1, first.campaign.RecipientCount)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
}

func TestMySQLRewardExpiredClaimsPreserveWalletAndStatistics(t *testing.T) {
	for _, rewardType := range []string{RewardTypeFixed, RewardTypeRandom} {
		t.Run(rewardType, func(t *testing.T) {
			db := setupRewardMySQLTestDB(t)
			require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &RewardCampaign{}, &UserReward{}))
			user := createReserveTestUser(t, 100)
			input := rewardTestCampaignInput("mysql-expired-claim")
			input.Type = rewardType
			if rewardType == RewardTypeRandom {
				input.MinQuota, input.MaxQuota = 7, 11
			}
			reward := createRewardForTestUser(t, user, input)
			require.NoError(t, db.Model(&reward).Update("expires_at", common.GetTimestamp()-1).Error)
			for range 2 {
				_, fresh, err := ClaimUserReward(user.Id, reward.Id)
				assert.ErrorIs(t, err, ErrRewardExpired)
				assert.False(t, fresh)
			}
			assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
			var ledger UserReward
			require.NoError(t, db.First(&ledger, reward.Id).Error)
			assert.Equal(t, RewardStatusPending, ledger.Status)
			assert.Zero(t, ledger.QuotaAwarded)
			assert.Zero(t, ledger.ClaimedAt)
			campaigns, total, err := ListRewardCampaigns(1, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, campaigns, 1)
			assert.Equal(t, 1, campaigns[0].RecipientCount)
			assert.Zero(t, campaigns[0].ClaimedCount)
			assert.Zero(t, campaigns[0].ClaimedQuota)
			summary, err := GetUserRewardSummary(user.Id)
			require.NoError(t, err)
			assert.Zero(t, summary.PendingCount)
			items, count, err := ListUserRewards(user.Id, RewardStatusExpired, "", 1, 10)
			require.NoError(t, err)
			assert.EqualValues(t, 1, count)
			require.Len(t, items, 1)
			assert.Equal(t, RewardStatusExpired, items[0].Status)
		})
	}
}
