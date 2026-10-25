package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRewardDistributionServiceTest(t *testing.T) *model.RewardCampaign {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.RewardCampaign{}, &model.UserReward{},
		&model.RewardDistributionState{}, &model.RewardDistributionCandidate{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	require.NoError(t, model.MigrateRewardActivityEvents(db))
	user := model.User{Username: "reward-service-test", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	campaign, err := model.CreateRewardCampaign(model.RewardCampaignInput{
		Title: "Recoverable activity", Type: "fixed", MinQuota: 20, MaxQuota: 20, ExpiresInDays: 7,
		Audience: "usage", StartTime: 100, EndTime: 200, Continuous: true, RequestID: "service-campaign",
	}, user.Id)
	require.NoError(t, err)
	return campaign
}

func TestRewardDistributionHandlerDrainsSeveralBatchesUnderOneTask(t *testing.T) {
	campaign := setupRewardDistributionServiceTest(t)
	logs := make([]model.Log, 101)
	for i := range logs {
		logs[i] = model.Log{UserId: campaign.CreatedBy, CreatedAt: 150, Type: model.LogTypeConsume}
	}
	require.NoError(t, model.LOG_DB.CreateInBatches(&logs, 20).Error)
	task, err := model.CreateSystemTask(rewardDistributionTaskType, nil, nil)
	require.NoError(t, err)
	claimed, acquired, err := model.ClaimSystemTask(task.ID, rewardDistributionTaskType, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, acquired)
	rewardDistributionHandler{}.Run(context.Background(), claimed, "worker")
	require.NoError(t, model.DB.First(task, task.ID).Error)
	assert.Equal(t, model.SystemTaskStatusSucceeded, task.Status)
	var result model.RewardDistributionResult
	require.NoError(t, common.UnmarshalJsonStr(task.Result, &result))
	assert.GreaterOrEqual(t, result.CampaignsProcessed, 2, "the scheduler must drain beyond one 100-row batch")
	var state model.RewardDistributionState
	require.NoError(t, model.DB.First(&state, "campaign_id = ?", campaign.Id).Error)
	assert.True(t, state.UsageDone)
	var count int64
	require.NoError(t, model.DB.Model(&model.UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestRewardDistributionHandlerCancellationLeavesDurableWorkForNextTask(t *testing.T) {
	campaign := setupRewardDistributionServiceTest(t)
	event := model.RewardActivityEvent{Kind: "usage", UserID: campaign.CreatedBy, OccurredAt: 150, Confirmed: true}
	require.NoError(t, model.DB.Create(&event).Error)
	task, err := model.CreateSystemTask(rewardDistributionTaskType, nil, nil)
	require.NoError(t, err)
	claimed, acquired, err := model.ClaimSystemTask(task.ID, rewardDistributionTaskType, "worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, acquired)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rewardDistributionHandler{}.Run(ctx, claimed, "worker")
	require.NoError(t, model.DB.First(task, task.ID).Error)
	assert.Equal(t, model.SystemTaskStatusFailed, task.Status)
	var count int64
	require.NoError(t, model.DB.Model(&model.RewardActivityEvent{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, model.DB.Model(&model.UserReward{}).Count(&count).Error)
	assert.Zero(t, count)
	_, err = model.RunRewardDistributionPass(context.Background(), 100)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.UserReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
