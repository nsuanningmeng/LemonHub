package model

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type rewardRestartMySQLSnapshot struct {
	ParentPID            int
	RequestID            string
	Rewards              []UserReward
	PendingRewardIDs     []int
	AlreadyClaimedID     int
	ExpiredID            int
	ContinuousCampaignID int
	RecoveryUserID       int
}

// Two genuinely separate executable processes start against the same retained
// test database. This exercises reward persistence across worker/service process
// loss; it deliberately does not claim to test a full deployment container.
func TestMySQLRewardContinuousIndependentProcessRestartsPreserveRewards(t *testing.T) {
	db := setupRewardMySQLTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &rewardCampaignV058{}, &userRewardV058{}))
	now := common.GetTimestamp()
	oldPending := createReserveTestUser(t, 0)
	oldClaimed := createReserveTestUser(t, 25)
	oldExpired := createReserveTestUser(t, 0)
	newPending := createReserveTestUser(t, 0)
	recovery := createReserveTestUser(t, 0)
	requestID := "restart-fixture-" + common.GetUUID()
	oldCampaigns := []rewardCampaignV058{
		{Id: 11, Title: "升级前已经发放", Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, ExpiresInDays: 7, ExpiresAt: now + 6*86400, Audience: "usage", StartTime: now - 86400, EndTime: now + 7*86400, RequestID: requestID, CreatedBy: oldPending.Id, CreatedAt: now - 86400, RecipientCount: 2},
		{Id: 12, Title: "重启不能复活过期奖励", Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, ExpiresInDays: 1, ExpiresAt: now - 86400, Audience: "usage", StartTime: now - 3*86400, EndTime: now + 86400, RequestID: "restart-expired", CreatedBy: oldPending.Id, CreatedAt: now - 2*86400, RecipientCount: 1},
	}
	oldRewards := []userRewardV058{
		{Id: 101, CampaignID: 11, UserID: oldPending.Id, Title: oldCampaigns[0].Title, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: RewardStatusPending, CreatedAt: oldCampaigns[0].CreatedAt, ExpiresAt: oldCampaigns[0].ExpiresAt},
		{Id: 102, CampaignID: 11, UserID: oldClaimed.Id, Title: oldCampaigns[0].Title, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: RewardStatusClaimed, QuotaAwarded: 25, CreatedAt: oldCampaigns[0].CreatedAt, ClaimedAt: now - 3600, ExpiresAt: oldCampaigns[0].ExpiresAt},
		{Id: 103, CampaignID: 12, UserID: oldExpired.Id, Title: oldCampaigns[1].Title, Type: RewardTypeFixed, MinQuota: 25, MaxQuota: 25, Status: RewardStatusPending, CreatedAt: oldCampaigns[1].CreatedAt, ExpiresAt: oldCampaigns[1].ExpiresAt},
	}
	require.NoError(t, db.Create(&oldCampaigns).Error)
	require.NoError(t, db.Create(&oldRewards).Error)
	migrateRewardContinuousMySQLFixture(t, db)
	campaign, err := CreateRewardCampaign(continuousMySQLInput("restart-continuous", "usage", now-60, now+3600), oldPending.Id)
	require.NoError(t, err)
	drainRewardContinuousMySQL(t, db, 2)
	require.NoError(t, createLog(&Log{UserId: newPending.Id, Type: LogTypeConsume, CreatedAt: now, RequestId: "restart-issued-not-acked"}))
	events, err := ListPendingRewardActivityEvents(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, events, 1)
	// Recipient committed, source not acknowledged: a restart must neither
	// regenerate the grant timestamp nor issue a second copy.
	require.NoError(t, ProcessRewardActivityEvent(context.Background(), events[0].Event))
	var newReward UserReward
	require.NoError(t, db.First(&newReward, "campaign_id = ? AND user_id = ?", campaign.Id, newPending.Id).Error)
	// This event is committed but has not been distributed at all yet.
	require.NoError(t, createLog(&Log{UserId: recovery.Id, Type: LogTypeConsume, CreatedAt: now, RequestId: "restart-undelivered"}))
	snapshot := rewardRestartMySQLSnapshot{
		ParentPID: os.Getpid(), RequestID: requestID,
		PendingRewardIDs: []int{101, newReward.Id}, AlreadyClaimedID: 102, ExpiredID: 103,
		ContinuousCampaignID: campaign.Id, RecoveryUserID: recovery.Id,
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	var previousPID int
	for _, phase := range []string{"first-start", "after-claims"} {
		require.NoError(t, db.Order("id").Find(&snapshot.Rewards).Error)
		encoded, marshalErr := common.Marshal(snapshot)
		require.NoError(t, marshalErr)
		command := exec.Command(executable, "-test.run=^TestMySQLRewardContinuousRestartChild$", "-test.v", "-test.timeout=45s")
		command.Env = append(os.Environ(), "LEMONHUB_REWARD_RESTART_PHASE="+phase, "LEMONHUB_REWARD_RESTART_SNAPSHOT="+string(encoded))
		output, runErr := command.CombinedOutput()
		require.NoError(t, runErr, "independent process %s:\n%s", phase, output)
		require.NotNil(t, command.ProcessState)
		pid := command.ProcessState.Pid()
		assert.NotEqual(t, os.Getpid(), pid)
		assert.NotEqual(t, previousPID, pid)
		previousPID = pid
		t.Logf("independent reward process %s completed (PID %d)", phase, pid)
	}
	var afterCampaigns []rewardCampaignV058
	require.NoError(t, db.Where("id IN ?", []int{11, 12}).Order("id").Find(&afterCampaigns).Error)
	assert.Equal(t, oldCampaigns, afterCampaigns, "neither process may expand or rewrite a deployed legacy activity")
	for _, user := range []User{oldPending, oldClaimed, newPending, recovery} {
		assert.Equal(t, 25, getUserQuotaFromDB(t, user.Id), "each eligible wallet remains credited exactly once across both processes")
	}
	assert.Zero(t, getUserQuotaFromDB(t, oldExpired.Id))
	var allRewards []UserReward
	require.NoError(t, db.Order("id").Find(&allRewards).Error)
	assert.Len(t, allRewards, 5, "only the one undistributed recipient may be added during recovery")
}

// This helper is executed only by the lifecycle test above. It does not own or
// drop the schema; the parent verifies the empty dedicated schema and controls
// its entire lifetime. An exact fixture identity gates access to existing rows.
func TestMySQLRewardContinuousRestartChild(t *testing.T) {
	phase := os.Getenv("LEMONHUB_REWARD_RESTART_PHASE")
	if phase == "" {
		t.Skip("independent-process helper, invoked by the lifecycle test")
	}
	require.Contains(t, []string{"first-start", "after-claims"}, phase)
	var snapshot rewardRestartMySQLSnapshot
	require.NoError(t, common.UnmarshalJsonStr(os.Getenv("LEMONHUB_REWARD_RESTART_SNAPSHOT"), &snapshot))
	require.NotEqual(t, os.Getpid(), snapshot.ParentPID)
	require.NotEmpty(t, snapshot.RequestID)
	config, err := mysqldriver.ParseDSN(os.Getenv("MYSQL_REWARD_TEST_DSN"))
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^lemonhub_rewards_test_[a-z0-9_]+$`), config.DBName)
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(false))
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	var selected string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selected).Error)
	require.Equal(t, config.DBName, selected)
	var identity RewardCampaign
	require.NoError(t, db.First(&identity, "request_id = ?", snapshot.RequestID).Error)
	require.False(t, identity.Continuous)
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	// These are the migrations/startup registration used for reward storage;
	// existing large source tables are not rebuilt in a child process.
	require.NoError(t, db.AutoMigrate(&RewardCampaign{}, &UserReward{}, &RewardDistributionState{}, &RewardDistributionCandidate{}))
	require.NoError(t, MigrateRewardActivityEvents(db))
	require.NoError(t, InitializeRewardActivityCapture())
	for _, before := range snapshot.Rewards {
		var stored UserReward
		require.NoError(t, db.First(&stored, before.Id).Error)
		assert.Equal(t, before.CreatedAt, stored.CreatedAt)
		assert.Equal(t, before.ExpiresAt, stored.ExpiresAt)
		assert.Equal(t, before.MinQuota, stored.MinQuota)
		assert.Equal(t, before.MaxQuota, stored.MaxQuota)
		assert.Equal(t, before, stored, "restart preserves every existing pending, expired, and claimed ledger field")
	}
	drainRewardContinuousMySQL(t, db, 2)
	for _, id := range snapshot.PendingRewardIDs {
		var stored UserReward
		require.NoError(t, db.First(&stored, id).Error)
		claimed, fresh, claimErr := ClaimUserReward(stored.UserID, id)
		require.NoError(t, claimErr)
		assert.Equal(t, phase == "first-start", fresh)
		assert.Equal(t, 25, claimed.QuotaAwarded)
		assert.Equal(t, stored.CreatedAt, claimed.CreatedAt)
		assert.Equal(t, stored.ExpiresAt, claimed.ExpiresAt)
		assert.Equal(t, 25, getUserQuotaFromDB(t, stored.UserID))
	}
	var expired UserReward
	require.NoError(t, db.First(&expired, snapshot.ExpiredID).Error)
	_, fresh, err := ClaimUserReward(expired.UserID, expired.Id)
	require.ErrorIs(t, err, ErrRewardExpired)
	assert.False(t, fresh)
	assert.Zero(t, getUserQuotaFromDB(t, expired.UserID))
	var already UserReward
	require.NoError(t, db.First(&already, snapshot.AlreadyClaimedID).Error)
	retried, fresh, err := ClaimUserReward(already.UserID, already.Id)
	require.NoError(t, err)
	assert.False(t, fresh)
	assert.Equal(t, already, *retried)
	assert.Equal(t, 25, getUserQuotaFromDB(t, already.UserID))
	var recovered UserReward
	require.NoError(t, db.First(&recovered, "campaign_id = ? AND user_id = ?", snapshot.ContinuousCampaignID, snapshot.RecoveryUserID).Error)
	_, fresh, err = ClaimUserReward(recovered.UserID, recovered.Id)
	require.NoError(t, err)
	assert.Equal(t, phase == "first-start", fresh)
	assert.Equal(t, 25, getUserQuotaFromDB(t, recovered.UserID))
	listed, total, err := ListRewardCampaigns(1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	for _, campaign := range listed {
		if campaign.Id == snapshot.ContinuousCampaignID || campaign.Id == 11 {
			assert.Equal(t, 2, campaign.RecipientCount)
			assert.EqualValues(t, 2, campaign.ClaimedCount)
			assert.EqualValues(t, 50, campaign.ClaimedQuota)
		} else {
			assert.Equal(t, 1, campaign.RecipientCount)
			assert.Zero(t, campaign.ClaimedCount)
			assert.Zero(t, campaign.ClaimedQuota)
		}
	}
}
