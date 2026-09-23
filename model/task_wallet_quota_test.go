package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskWalletFundingSupportsLargeBalances(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance int
		delta   int
	}{
		{name: "large debit", balance: 5_000_000_000, delta: 50},
		{name: "large refund", balance: 5_000_000_000, delta: -50},
		{name: "large debt refund", balance: -5_000_000_000, delta: -50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			useTaskLedgerRedis(t)
			user := createReserveTestUser(t, tc.balance)
			task := &Task{TaskID: "large-wallet-task", UserId: user.Id, Quota: 100}
			insertTask(t, task)
			_, err := GetUserCache(user.Id)
			require.NoError(t, err)

			stage := TaskBillingStageParams{
				TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
				Operation: fmt.Sprintf("settle:%d", 100+tc.delta), Stage: TaskBillingStageFunding,
				Delta: tc.delta, TargetQuota: 100 + tc.delta, UserId: user.Id, BillingSource: "wallet",
			}
			applied, err := ApplyTaskBillingStage(stage)
			require.NoError(t, err)
			require.True(t, applied)
			assert.Equal(t, tc.balance-tc.delta, getUserQuotaFromDB(t, user.Id))
			cached, err := GetUserCache(user.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.balance-tc.delta, cached.Quota)

			applied, err = ApplyTaskBillingStage(stage)
			require.NoError(t, err)
			assert.False(t, applied)
			undone, err := UndoTaskBillingStage(stage)
			require.NoError(t, err)
			assert.True(t, undone)
			assert.Equal(t, tc.balance, getUserQuotaFromDB(t, user.Id))
			cached, err = GetUserCache(user.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.balance, cached.Quota)
		})
	}
}

func TestTaskWalletFundingRejectsOutOfRangeWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance int
		delta   int
	}{
		{name: "wallet upper bound", balance: common.MaxWalletQuota, delta: -50},
		{name: "wallet lower bound", balance: -common.MaxWalletQuota, delta: 50},
		{name: "individual debit bound", balance: 5_000_000_000, delta: common.MaxQuota + 1},
		{name: "individual refund bound", balance: 5_000_000_000, delta: common.MinQuota - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			useTaskLedgerRedis(t)
			user := createReserveTestUser(t, tc.balance)
			task := &Task{TaskID: "wallet-task-boundary", UserId: user.Id, Quota: 100}
			insertTask(t, task)
			_, err := GetUserCache(user.Id)
			require.NoError(t, err)
			stage := TaskBillingStageParams{
				TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
				Operation: "settle:150", Stage: TaskBillingStageFunding,
				Delta: tc.delta, TargetQuota: 150, UserId: user.Id, BillingSource: "wallet",
			}
			applied, err := ApplyTaskBillingStage(stage)
			require.Error(t, err)
			assert.False(t, applied)
			assert.Equal(t, tc.balance, getUserQuotaFromDB(t, user.Id))
			cached, err := GetUserCache(user.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.balance, cached.Quota)
			ledger, err := GetTaskBillingStageRecord(stage.TaskType, task.ID, stage.Operation, stage.Stage)
			require.NoError(t, err)
			assert.Nil(t, ledger)
		})
	}
}

func TestTaskWalletAggregateUsageSupportsLargeTotals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		baseline bool
		delta    int
	}{
		{name: "submission baseline", baseline: true, delta: 100},
		{name: "settlement debit", delta: 50},
		{name: "settlement refund", delta: -50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			user := createReserveTestUser(t, 1_000)
			const initialUsed = 5_000_000_000
			require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
				"used_quota": initialUsed, "request_count": 10,
			}).Error)
			task := &Task{TaskID: "large-wallet-usage", UserId: user.Id, Quota: 100}
			if !tc.baseline {
				task.PrivateData.AggregateUsageState = TaskAggregateUsageAccounted
			}
			insertTask(t, task)
			stage := TaskBillingStageParams{
				TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
				Operation: fmt.Sprintf("settle:%d", 100+tc.delta), Stage: TaskBillingStageFunding,
				Delta: tc.delta, TargetQuota: 100 + tc.delta, UserId: user.Id, BillingSource: "wallet",
			}
			if tc.baseline {
				stage.Operation = "submit"
				stage.Stage = TaskBillingStageAggregateBaseline
				stage.TargetQuota = 100
				stage.RequestCountDelta = 1
			} else {
				applied, err := ApplyTaskBillingStage(stage)
				require.NoError(t, err)
				require.True(t, applied)
				stage.Stage = TaskBillingStageFinalize
			}
			applied, err := ApplyTaskBillingStage(stage)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = ApplyTaskBillingStage(stage)
			require.NoError(t, err)
			assert.False(t, applied)
			var reloaded User
			require.NoError(t, DB.First(&reloaded, user.Id).Error)
			assert.Equal(t, initialUsed+tc.delta, reloaded.UsedQuota)
			assert.Equal(t, 10+stage.RequestCountDelta, reloaded.RequestCount)
		})
	}
}
