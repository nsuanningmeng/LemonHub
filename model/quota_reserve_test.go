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

func createReserveTestUser(t *testing.T, quota int) User {
	t.Helper()
	user := User{
		Username:    "reserve-user-" + common.GetRandomString(6),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       quota,
		AffCode:     "reserve-aff-" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func injectPostCommitUpdateError(t *testing.T, table string) error {
	t.Helper()
	forcedErr := errors.New("forced post-commit driver error")
	callbackName := "test:post_commit_error:" + common.GetUUID()
	fired := false
	require.NoError(t, DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register(callbackName, func(tx *gorm.DB) {
		if !fired && tx.Error == nil && tx.Statement != nil && tx.Statement.Table == table {
			fired = true
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })
	return forcedErr
}

func injectPostCommitUpdateHook(t *testing.T, table string, hook func()) {
	t.Helper()
	callbackName := "test:post_commit_hook:" + common.GetUUID()
	fired := false
	require.NoError(t, DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register(callbackName, func(tx *gorm.DB) {
		if !fired && tx.Error == nil && tx.Statement != nil && tx.Statement.Table == table {
			fired = true
			hook()
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })
}

func TestCreditPersistsBeforeCacheAndCacheSyncFailureReturnsSuccess(t *testing.T) {
	t.Run("user credit", func(t *testing.T) {
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 20)
		require.NoError(t, populateUserCache(user))
		injectPostCommitUpdateHook(t, "users", server.Close)

		err := IncreaseUserQuota(user.Id, 7, true)
		require.NoError(t, err, "a committed credit must not be retried because only cache sync failed")
		assert.Equal(t, 27, getUserQuotaFromDB(t, user.Id), "credit is durable before cache synchronization")

		require.NoError(t, server.Restart())
		cached, cacheErr := cacheGetUserBase(user.Id)
		require.NoError(t, cacheErr)
		assert.LessOrEqual(t, cached.Quota, 27, "cache failure must never create credit absent from the DB")
		server.FastForward(24 * time.Hour)
		rehydrated, rehydrateErr := GetUserCache(user.Id)
		require.NoError(t, rehydrateErr)
		assert.Equal(t, 27, rehydrated.Quota)
	})

	t.Run("token credit", func(t *testing.T) {
		server := useUserCacheMiniRedis(t)
		token := createReserveTestToken(t, 20)
		require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("used_quota", 7).Error)
		_, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		injectPostCommitUpdateHook(t, "tokens", server.Close)

		err = IncreaseTokenQuota(token.Id, token.Key, 7)
		require.NoError(t, err, "a committed token credit must not be retried because only cache sync failed")
		persisted := getTokenFromDB(t, token.Id)
		assert.Equal(t, 27, persisted.RemainQuota)
		assert.Zero(t, persisted.UsedQuota)

		require.NoError(t, server.Restart())
		cached, cacheErr := cacheGetTokenByKey(token.Key)
		require.NoError(t, cacheErr)
		assert.LessOrEqual(t, cached.RemainQuota, 27)
		server.FastForward(24 * time.Hour)
		rehydrated, rehydrateErr := GetTokenByKey(token.Key, false)
		require.NoError(t, rehydrateErr)
		assert.Equal(t, 27, rehydrated.RemainQuota)
		assert.Zero(t, rehydrated.UsedQuota)
	})
}

func TestCommittedDebitReportedAsErrorFencesCacheInsteadOfRestoringOldBalance(t *testing.T) {
	t.Run("user delta", func(t *testing.T) {
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 20)
		require.NoError(t, populateUserCache(user))
		forcedErr := injectPostCommitUpdateError(t, "users")

		err := DecreaseUserQuota(user.Id, 7, true)
		assert.ErrorIs(t, err, forcedErr)
		assert.Equal(t, 13, getUserQuotaFromDB(t, user.Id), "the simulated driver error is reported after commit")
		assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
		assert.False(t, server.Exists(getUserCacheKey(user.Id)), "the old higher hash must not be restored")

		reserved, reserveErr := TryReserveUserQuota(user.Id, 14)
		require.NoError(t, reserveErr)
		assert.False(t, reserved)
		cached, cacheErr := cacheGetUserBase(user.Id)
		require.NoError(t, cacheErr)
		assert.Equal(t, 13, cached.Quota)
	})

	t.Run("token delta", func(t *testing.T) {
		server := useUserCacheMiniRedis(t)
		token := createReserveTestToken(t, 20)
		_, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		forcedErr := injectPostCommitUpdateError(t, "tokens")

		err = DecreaseTokenQuota(token.Id, token.Key, 7)
		assert.ErrorIs(t, err, forcedErr)
		persisted := getTokenFromDB(t, token.Id)
		assert.Equal(t, 13, persisted.RemainQuota)
		assert.Equal(t, 7, persisted.UsedQuota)
		assert.True(t, server.Exists(getTokenCacheFenceKey(token.Key)))
		assert.False(t, server.Exists(getTokenCacheKey(token.Key)))

		server.FastForward((tokenCacheFenceSeconds + 1) * time.Second)
		cached, cacheErr := GetTokenByKey(token.Key, false)
		require.NoError(t, cacheErr)
		assert.Equal(t, 13, cached.RemainQuota)
	})

	t.Run("user reserve", func(t *testing.T) {
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 20)
		require.NoError(t, populateUserCache(user))
		forcedErr := injectPostCommitUpdateError(t, "users")

		reserved, err := TryReserveUserQuota(user.Id, 7)
		assert.False(t, reserved)
		assert.ErrorIs(t, err, forcedErr)
		assert.Equal(t, 13, getUserQuotaFromDB(t, user.Id))
		assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
		assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	})

	t.Run("token reserve", func(t *testing.T) {
		server := useUserCacheMiniRedis(t)
		token := createReserveTestToken(t, 20)
		_, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		forcedErr := injectPostCommitUpdateError(t, "tokens")

		reserved, err := TryReserveTokenQuota(token.Id, token.Key, 7, false)
		assert.False(t, reserved)
		assert.ErrorIs(t, err, forcedErr)
		persisted := getTokenFromDB(t, token.Id)
		assert.Equal(t, 13, persisted.RemainQuota)
		assert.Equal(t, 7, persisted.UsedQuota)
		assert.True(t, server.Exists(getTokenCacheFenceKey(token.Key)))
		assert.False(t, server.Exists(getTokenCacheKey(token.Key)))
	})
}

func createReserveTestToken(t *testing.T, remainQuota int) Token {
	t.Helper()
	token := Token{
		UserId:      1,
		Key:         "reserve-token-" + common.GetRandomString(8),
		Name:        "reserve-test",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: remainQuota,
	}
	require.NoError(t, token.Insert())
	return token
}

func getUserQuotaFromDB(t *testing.T, id int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, id).Error)
	return user.Quota
}

func getTokenFromDB(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.First(&token, id).Error)
	return token
}

func resetBatchUpdateTestState(t *testing.T) {
	t.Helper()
	oldBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchEnabled
		for i := 0; i < BatchUpdateTypeCount; i++ {
			batchUpdateLocks[i].Lock()
			batchUpdateStores[i] = make(map[int]int)
			batchUpdateLocks[i].Unlock()
		}
	})
}

func TestTryReserveQuotaWithoutRedis(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createReserveTestUser(t, 100)
	reserved, err := TryReserveUserQuota(user.Id, 60)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 41)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 25, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 55, reloaded.RemainQuota)
	assert.Equal(t, 25, reloaded.UsedQuota)

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 56, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 55, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestRedisReservePersistsBalanceEvenWhenBatchMetricsAreEnabled(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id), "spendable balance must be durable before return")

	reserved, err = TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.False(t, reserved, "stale DB balance must not authorize a second spend")
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, cachedUser.Quota)

	token := createReserveTestToken(t, 9)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 3, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 2, getTokenFromDB(t, token.Id).RemainQuota)

	batchUpdate()
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.Equal(t, 2, reloadedToken.RemainQuota)
	assert.Equal(t, 7, reloadedToken.UsedQuota)
}

func TestReserveFailsClosedAndPreservesBalanceAcrossRedisRecovery(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 20)
	require.NoError(t, populateUserCache(user))
	token := createReserveTestToken(t, 20)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	server.Close()

	reserved, err := TryReserveUserQuota(user.Id, 5)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 5, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	assert.Equal(t, 20, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 20, getTokenFromDB(t, token.Id).RemainQuota)

	require.NoError(t, server.Restart())
	reserved, err = TryReserveUserQuota(user.Id, 16)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 16, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 4, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 4, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestReserveFailsClosedWhenRedisAuthorityIsLost(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	require.True(t, reserved)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))

	server.Close()
	reserved, err = TryReserveUserQuota(user.Id, 3)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
}

func TestBalanceUsesDatabaseAuthorityWhenRedisIsDisabled(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 1)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 9, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 10)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 1, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 9, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestCacheMissHydratesFromDurableBalance(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, common.RDB.Del(context.Background(), getUserCacheKey(user.Id)).Err())

	reserved, err = TryReserveUserQuota(user.Id, 3)
	assert.False(t, reserved)
	require.NoError(t, err)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 10)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 8, false)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, common.RDB.Del(context.Background(), getTokenCacheKey(token.Key)).Err())

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 3, false)
	assert.False(t, reserved)
	require.NoError(t, err)
	assert.Equal(t, 2, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestSubscriptionBalancePurchaseUsesCacheAuthoritativeQuota(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	require.True(t, reserved)

	allowBalance := true
	plan := SubscriptionPlan{
		Title:            "cache-authoritative-plan",
		PriceAmount:      3,
		Currency:         "USD",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		Enabled:          true,
		AllowBalancePay:  &allowBalance,
		QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	err = PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorContains(t, err, "余额不足")
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionBalancePurchaseReleasesReservationOnBusinessFailure(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 2)
	require.NoError(t, err)
	require.True(t, reserved)

	allowBalance := true
	plan := SubscriptionPlan{
		Title:              "reservation-compensation-plan",
		PriceAmount:        3,
		Currency:           "USD",
		DurationUnit:       SubscriptionDurationMonth,
		DurationValue:      1,
		Enabled:            true,
		AllowBalancePay:    &allowBalance,
		QuotaResetPeriod:   SubscriptionResetNever,
		MaxPurchasePerUser: 1,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId: user.Id,
		PlanId: plan.Id,
		Status: "active",
	}).Error)

	err = PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.Error(t, err)
	cached, cacheErr := GetUserCache(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 8, cached.Quota, "the failed purchase must release only its own reservation")
	assert.Equal(t, 8, getUserQuotaFromDB(t, user.Id), "the failed purchase releases only its own reservation")

	batchUpdate()
	assert.Equal(t, 8, getUserQuotaFromDB(t, user.Id))
}

func TestSynchronousReserveFencesCacheWhenPersistenceFails(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DB.Delete(&user).Error)

	reserved, err := TryReserveUserQuota(user.Id, 6)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, cacheErr := cacheGetUserBase(user.Id)
	assert.Error(t, cacheErr, "an uncertain debit must remove the spendable user hash")
	userFence, fenceErr := common.RDB.Exists(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Result()
	require.NoError(t, fenceErr)
	assert.EqualValues(t, 1, userFence)

	token := createReserveTestToken(t, 12)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Delete(&token).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, cacheErr = cacheGetTokenByKey(token.Key)
	assert.Error(t, cacheErr, "an uncertain debit must remove the spendable token hash")
	tokenFence, fenceErr := common.RDB.Exists(context.Background(), getTokenCacheFenceKey(token.Key)).Result()
	require.NoError(t, fenceErr)
	assert.EqualValues(t, 1, tokenFence)
}

func TestTokenCacheInitPreservesLiveQuotaAndFenceBlocksStaleSnapshot(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	token := createReserveTestToken(t, 100)
	loaded, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	stale := *loaded

	result, err := cacheApplyTokenQuotaDelta(token.Id, token.Key, -70)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)

	// 已存在的哈希只刷新 TTL：数据库快照不得覆盖已被原子预扣的余额。
	code, err := cacheInitToken(stale)
	require.NoError(t, err)
	assert.Equal(t, 2, code)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 30, cached.RemainQuota)

	// 变更期间：fence 删除缓存并拦截并发读者手中的过期快照。
	require.NoError(t, invalidateTokenCacheForMutation(token.Key))
	code, err = cacheInitToken(stale)
	require.NoError(t, err)
	assert.Zero(t, code, "the pre-mutation snapshot must not be published while fenced")
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err)

	// fence 过期后可重新从数据库水合。
	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	fresh, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 100, fresh.RemainQuota)
	cached, err = cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 100, cached.RemainQuota)
}

func TestQuotaDeltasAreVisibleAndDurableBeforeReturn(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	require.NoError(t, DecreaseUserQuota(user.Id, 7, false))
	reserved, err := TryReserveUserQuota(user.Id, 4)
	require.NoError(t, err)
	assert.False(t, reserved, "a settlement debit must be visible to the next reserve immediately")
	userCache, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 3, userCache.Quota)
	assert.Equal(t, 3, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 10)
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 7))
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 4, false)
	require.NoError(t, err)
	assert.False(t, reserved, "a token settlement debit must be visible to the next reserve immediately")
	tokenCache, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 3, tokenCache.RemainQuota)
	assert.Equal(t, 7, tokenCache.UsedQuota)

	batchUpdate()
	assert.Equal(t, 3, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 3, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestBatchQuotaDeltaHandlesRedisUnavailableSafely(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	token := createReserveTestToken(t, 10)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("used_quota", 2).Error)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	server.Close()

	err = DecreaseUserQuota(user.Id, 3, false)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	err = IncreaseTokenQuota(token.Id, token.Key, 2)
	require.NoError(t, err, "DB-first credit remains successful when only cache sync is unavailable")
	assert.False(t, hasPendingBatchUpdate(BatchUpdateTypeUserQuota, user.Id))
	assert.False(t, hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, token.Id))
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 12, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestForcedUserQuotaDeltaWritesDBAndCacheSynchronously(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, IncreaseUserQuota(user.Id, 5, true))

	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))
	assert.False(t, hasPendingBatchUpdate(BatchUpdateTypeUserQuota, user.Id))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 15, cached.Quota)
}

func TestBatchQuotaFlushFailureRequeuesWithoutLosingConcurrentDelta(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 20)
	token := createReserveTestToken(t, 20)
	require.NoError(t, populateUserCache(user))
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	result, err := cacheApplyUserQuotaDelta(user.Id, -3)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	result, err = cacheApplyTokenQuotaDelta(token.Id, token.Key, -4)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -3)
	addNewRecord(BatchUpdateTypeTokenQuota, token.Id, -4)

	require.NoError(t, DB.Exec(`CREATE TRIGGER fail_user_quota_flush
BEFORE UPDATE OF quota ON users BEGIN SELECT RAISE(ABORT, 'forced user flush failure'); END`).Error)
	require.NoError(t, DB.Exec(`CREATE TRIGGER fail_token_quota_flush
BEFORE UPDATE OF remain_quota ON tokens BEGIN SELECT RAISE(ABORT, 'forced token flush failure'); END`).Error)
	batchUpdate()

	assert.Equal(t, 20, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 20, getTokenFromDB(t, token.Id).RemainQuota)
	assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeUserQuota, user.Id))
	assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, token.Id))

	// A delta arriving after the failed flush is merged with the requeued delta,
	// rather than being overwritten by the old store snapshot.
	result, err = cacheApplyUserQuotaDelta(user.Id, -2)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	result, err = cacheApplyTokenQuotaDelta(token.Id, token.Key, 1)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -2)
	addNewRecord(BatchUpdateTypeTokenQuota, token.Id, 1)
	require.NoError(t, DB.Exec("DROP TRIGGER fail_user_quota_flush").Error)
	require.NoError(t, DB.Exec("DROP TRIGGER fail_token_quota_flush").Error)
	batchUpdate()

	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.Equal(t, 17, reloadedToken.RemainQuota)
	assert.Equal(t, 3, reloadedToken.UsedQuota)
	assert.False(t, hasPendingBatchUpdate(BatchUpdateTypeUserQuota, user.Id))
	assert.False(t, hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, token.Id))

	batchUpdate()
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id), "a completed retry must not be applied twice")
	assert.Equal(t, 17, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestProcessLocalBatchStateLossCannotResurrectBalance(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 20)
	token := createReserveTestToken(t, 20)
	require.NoError(t, DecreaseUserQuota(user.Id, 7, false))
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 8))

	// Simulate a process restart losing every in-memory batch map, followed by
	// expiry/eviction of the Redis hashes. Spendable balances must rehydrate
	// from already-durable database rows, not from pre-debit values.
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	require.NoError(t, common.RDB.Del(context.Background(), getUserCacheKey(user.Id), getTokenCacheKey(token.Key)).Err())

	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 13, cachedUser.Quota)
	cachedToken, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	assert.Equal(t, 12, cachedToken.RemainQuota)
	assert.Equal(t, 8, cachedToken.UsedQuota)
}
