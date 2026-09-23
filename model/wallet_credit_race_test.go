package model

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenCreditDoesNotDoubleCountConcurrentCacheHydration(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	const initialQuota = 1<<54 + 3
	const initialUsed = 5_000_000_000
	token := createReserveTestToken(t, initialQuota)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("used_quota", initialUsed).Error)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	injectPostCommitUpdateHook(t, "tokens", func() {
		require.NoError(t, common.RDB.Del(context.Background(), getTokenCacheKey(token.Key)).Err())
		cached, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		assert.Equal(t, initialQuota+50, cached.RemainQuota)
		assert.Equal(t, initialUsed-50, cached.UsedQuota)
	})
	require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 50))
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, initialQuota+50, cached.RemainQuota)
	assert.Equal(t, initialUsed-50, cached.UsedQuota)
}

func TestWalletCreditDoesNotDoubleCountConcurrentCacheHydration(t *testing.T) {
	const initialQuota = 5_000_000_000
	const credit = 2_500_000_000
	for _, tc := range []struct {
		name          string
		warmCache     bool
		replaceCache  bool
		concurrentFee int
	}{
		{name: "first hydration"},
		{name: "expired cache rehydration", warmCache: true, replaceCache: true},
		{name: "preserve concurrent debit", warmCache: true, concurrentFee: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			useUserCacheMiniRedis(t)
			user := createReserveTestUser(t, initialQuota)
			if tc.warmCache {
				_, err := GetUserCache(user.Id)
				require.NoError(t, err)
			}
			// Interleave a request after the durable write, before credit cache sync.
			injectPostCommitUpdateHook(t, "users", func() {
				if tc.replaceCache {
					require.NoError(t, invalidateUserCache(user.Id))
				}
				if tc.concurrentFee > 0 {
					require.NoError(t, DecreaseUserQuota(user.Id, tc.concurrentFee, true))
				} else {
					cached, err := GetUserCache(user.Id)
					require.NoError(t, err)
					assert.Equal(t, initialQuota+credit, cached.Quota)
				}
			})
			require.NoError(t, IncreaseUserQuota(user.Id, credit, true))
			assert.Equal(t, initialQuota+credit-tc.concurrentFee, getUserQuotaFromDB(t, user.Id))
			cached, err := GetUserCache(user.Id)
			require.NoError(t, err)
			assert.Equal(t, initialQuota+credit-tc.concurrentFee, cached.Quota)
		})
	}
}

func TestTaskWalletRefundDoesNotDoubleCountConcurrentCacheHydration(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	const initialQuota = 5_000_000_000
	user := createReserveTestUser(t, initialQuota)
	task := &Task{TaskID: "refund-cache-hydration", UserId: user.Id, Quota: 100}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	oldRun := runTaskBillingTransaction
	t.Cleanup(func() { runTaskBillingTransaction = oldRun })
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		require.NoError(t, invalidateUserCache(user.Id))
		cached, err := GetUserCache(user.Id)
		require.NoError(t, err)
		assert.Equal(t, initialQuota+50, cached.Quota)
		return nil
	}
	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:50", Stage: TaskBillingStageFunding,
		Delta: -50, TargetQuota: 50, UserId: user.Id, BillingSource: "wallet",
	}
	applied, err := ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, initialQuota+50, getUserQuotaFromDB(t, user.Id))
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, initialQuota+50, cached.Quota)
}

func TestTaskTokenRefundDoesNotDoubleCountConcurrentCacheHydration(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 1_000)
	const initialQuota = 1<<54 + 3
	token := createReserveTestToken(t, initialQuota)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"user_id": user.Id, "used_quota": 100,
	}).Error)
	task := &Task{
		TaskID: "token-refund-cache-hydration", UserId: user.Id, Quota: 100,
		PrivateData: TaskPrivateData{AggregateUsageState: TaskAggregateUsageAccounted},
	}
	insertTask(t, task)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:50", Stage: TaskBillingStageFunding,
		Delta: -50, TargetQuota: 50, UserId: user.Id, BillingSource: "wallet",
		TokenId: token.Id, TokenKey: token.Key,
	}
	applied, err := ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	require.True(t, applied)
	stage.Stage = TaskBillingStageToken
	oldRun := runTaskBillingTransaction
	t.Cleanup(func() { runTaskBillingTransaction = oldRun })
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		require.NoError(t, common.RDB.Del(context.Background(), getTokenCacheKey(token.Key)).Err())
		_, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		return nil
	}
	applied, err = ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	require.True(t, applied)
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, initialQuota+50, cached.RemainQuota)
	assert.Equal(t, 50, cached.UsedQuota)
}

func TestStripeClawbackRollbackDoesNotCreditRehydratedCache(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	user, topUp := seedCachedStripeClawback(t, 6_000_000_000, 5_000_000_000)
	forcedErr := errors.New("force clawback rollback after cache eviction")
	callbackName := "test:clawback_cache_eviction:" + common.GetUUID()
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" || tx.Error != nil {
			return
		}
		// A reader publishes the last committed snapshot while this transaction
		// is still uncommitted. Compensation must not credit that fresh hash.
		tx.AddError(forcedErr)
		require.NoError(t, invalidateUserCache(user.Id))
		require.NoError(t, populateUserCache(user))
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })
	require.ErrorIs(t, ReverseStripeTopUp(topUp.PaymentIntent, 70, 100, false, "test"), forcedErr)
	assert.Equal(t, user.Quota, getUserQuotaFromDB(t, user.Id))
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, user.Quota, cached.Quota)
	var persisted TopUp
	require.NoError(t, DB.First(&persisted, topUp.Id).Error)
	assert.Zero(t, persisted.ClawedBackQuota)
}
