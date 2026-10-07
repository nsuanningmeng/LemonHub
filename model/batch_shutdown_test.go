package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFinalBatchFlushRollsBackWholeSnapshotBeforeRetry(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 1000)
	token := createReserveTestToken(t, 1000)
	channel := Channel{Name: "shutdown-channel", Key: "test"}
	require.NoError(t, DB.Create(&channel).Error)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -5)
	addNewRecord(BatchUpdateTypeTokenQuota, token.Id, -7)
	UpdateUserUsedQuotaAndRequestCount(user.Id, 5)
	UpdateChannelUsedQuota(channel.Id, 5)
	forced := errors.New("channel update rejected before SQL")
	const callback = "test:shutdown-channel-failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(forced)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	require.ErrorIs(t, FlushBatchUpdates(context.Background()), forced)
	assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeChannelUsedQuota, channel.Id))
	assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeUserQuota, user.Id))
	assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, token.Id))
	UpdateChannelUsedQuota(channel.Id, 2)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	require.NoError(t, FlushBatchUpdates(context.Background()))
	require.NoError(t, FlushBatchUpdates(context.Background()))
	var persistedUser User
	require.NoError(t, DB.First(&persistedUser, user.Id).Error)
	assert.Equal(t, 995, persistedUser.Quota)
	assert.Equal(t, 5, persistedUser.UsedQuota)
	assert.Equal(t, 1, persistedUser.RequestCount)
	persistedToken := getTokenFromDB(t, token.Id)
	assert.Equal(t, 993, persistedToken.RemainQuota)
	assert.Equal(t, 7, persistedToken.UsedQuota)
	require.NoError(t, DB.First(&channel, channel.Id).Error)
	assert.Equal(t, int64(7), channel.UsedQuota)
	assert.False(t, hasPendingBatchUpdate(BatchUpdateTypeChannelUsedQuota, channel.Id))
}

func TestFinalBatchFlushWaitIsCancellableAndPreservesConcurrentTail(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	channel := Channel{Name: "shutdown-serial", Key: "test"}
	require.NoError(t, DB.Create(&channel).Error)
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	const callback = "test:shutdown-block-flush"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			close(entered)
			<-release
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	UpdateChannelUsedQuota(channel.Id, 5)
	first := make(chan error, 1)
	go func() { first <- FlushBatchUpdates(context.Background()) }()
	<-entered
	UpdateChannelUsedQuota(channel.Id, 3)
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() { second <- FlushBatchUpdates(ctx) }()
	cancel()
	require.ErrorIs(t, <-second, context.Canceled)
	assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeChannelUsedQuota, channel.Id))
	close(release)
	released = true
	require.NoError(t, <-first)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	require.NoError(t, FlushBatchUpdates(context.Background()))
	require.NoError(t, DB.First(&channel, channel.Id).Error)
	assert.Equal(t, int64(8), channel.UsedQuota)
}

func TestStopBatchUpdaterLeavesTailForFinalFlush(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true
	oldInterval := common.BatchUpdateInterval
	common.BatchUpdateInterval = 3600
	t.Cleanup(func() {
		common.BatchUpdateInterval = oldInterval
		batchUpdaterMu.Lock()
		batchUpdaterCancel = nil
		batchUpdaterDone = nil
		batchUpdaterMu.Unlock()
	})
	user := createReserveTestUser(t, 1000)
	InitBatchUpdater()
	UpdateUserUsedQuotaAndRequestCount(user.Id, 23)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, StopBatchUpdater(ctx))
	require.NoError(t, StopBatchUpdater(ctx))
	require.NoError(t, FlushBatchUpdates(ctx))
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 23, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
}
