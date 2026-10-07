package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStopSystemTaskRunnerWaitsForFinalWritesAndLeavesNewTasksPending(t *testing.T) {
	truncate(t)
	seedUser(t, 15001, 100)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	var releaseOnce sync.Once
	handler := &stubScheduledHandler{
		taskType: "test_shutdown_active",
		onRun: func(ctx context.Context, task *model.SystemTask, runnerID string) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release
			// Cancellation can interrupt an upstream operation while its final
			// accounting and task-state writes still need the open database.
			model.UpdateUserUsedQuotaAndRequestCount(15001, 7)
			finished <- model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, nil, ctx.Err().Error())
		},
	}
	laterHandler := &stubScheduledHandler{taskType: "test_shutdown_pending"}
	withSystemTaskRegistry(t, handler, laterHandler)

	systemTaskRunnerMu.Lock()
	savedRunner := systemTaskRunner
	systemTaskRunner = nil
	systemTaskRunnerMu.Unlock()
	savedMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		assert.NoError(t, StopSystemTaskRunner(ctx))
		systemTaskRunnerMu.Lock()
		systemTaskRunner = savedRunner
		systemTaskRunnerMu.Unlock()
		common.IsMasterNode = savedMaster
	})

	active, err := model.CreateSystemTask(handler.taskType, nil, nil)
	require.NoError(t, err)
	StartSystemTaskRunner()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("system task handler did not start")
	}

	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	require.ErrorIs(t, StopSystemTaskRunner(waitCtx), context.Canceled,
		"shutdown must not report completion while a cancelled handler is still writing")
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel the active handler")
	}

	later, created, err := EnqueueSystemTask(laterHandler.taskType, nil)
	require.NoError(t, err)
	require.True(t, created)
	releaseOnce.Do(func() { close(release) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, StopSystemTaskRunner(ctx))
	require.NoError(t, <-finished)
	require.NoError(t, StopSystemTaskRunner(waitCtx), "completed shutdown is idempotent")

	var user model.User
	require.NoError(t, model.DB.First(&user, 15001).Error)
	assert.Equal(t, 7, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	var activeRow model.SystemTask
	require.NoError(t, model.DB.First(&activeRow, active.ID).Error)
	assert.Equal(t, model.SystemTaskStatusFailed, activeRow.Status)
	var locks int64
	require.NoError(t, model.DB.Model(&model.SystemTaskLock{}).Where("task_id = ?", active.TaskID).Count(&locks).Error)
	assert.Zero(t, locks, "handler must release its lease before shutdown returns")
	var laterRow model.SystemTask
	require.NoError(t, model.DB.First(&laterRow, later.ID).Error)
	assert.Equal(t, model.SystemTaskStatusPending, laterRow.Status)
}

func TestCancelledSystemTaskClaimPassPreservesPendingTask(t *testing.T) {
	truncate(t)
	handler := &stubScheduledHandler{taskType: "test_cancelled_claim"}
	withSystemTaskRegistry(t, handler)
	task, err := model.CreateSystemTask(handler.taskType, nil, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &systemTaskRunnerState{ctx: ctx}
	runner.claimPass("stopped-runner")
	var saved model.SystemTask
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	assert.Equal(t, model.SystemTaskStatusPending, saved.Status)
	var locks int64
	require.NoError(t, model.DB.Model(&model.SystemTaskLock{}).Count(&locks).Error)
	assert.Zero(t, locks)
}
