package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type failAfterMatchingRedisCommand struct {
	needle string
	err    error
	fired  atomic.Bool
}

func (h *failAfterMatchingRedisCommand) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h *failAfterMatchingRedisCommand) AfterProcess(_ context.Context, cmd redis.Cmder) error {
	if h.fired.Load() {
		return nil
	}
	for _, arg := range cmd.Args() {
		if strings.Contains(fmt.Sprint(arg), h.needle) && h.fired.CompareAndSwap(false, true) {
			// The real command has completed; only its reply is made unavailable.
			return h.err
		}
	}
	return nil
}

func (h *failAfterMatchingRedisCommand) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h *failAfterMatchingRedisCommand) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func createBalancePurchasePlan(t *testing.T, price float64) SubscriptionPlan {
	t.Helper()
	allowBalance := true
	plan := SubscriptionPlan{
		Title:            "atomic-balance-plan-" + common.GetRandomString(6),
		PriceAmount:      price,
		Currency:         "USD",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		Enabled:          true,
		AllowBalancePay:  &allowBalance,
		QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	return plan
}

func useUnitSubscriptionBalancePrice(t *testing.T) {
	t.Helper()
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = previous })
}

func countBalancePurchaseRows(t *testing.T, userId int) (orders int64, subscriptions int64) {
	t.Helper()
	require.NoError(t, DB.Model(&SubscriptionOrder{}).
		Where("user_id = ? AND payment_method = ?", userId, PaymentMethodBalance).
		Count(&orders).Error)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userId).Count(&subscriptions).Error)
	return orders, subscriptions
}

func TestUserCacheHydrationFailsClosedWhenInFlightFenceAppearsAfterDatabaseRead(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 10)
	require.NoError(t, common.RDB.Del(context.Background(), getUserCacheKey(user.Id)).Err())

	databaseRead := make(chan struct{})
	continueHydration := make(chan struct{})
	var releaseOnce sync.Once
	releaseHydration := func() { releaseOnce.Do(func() { close(continueHydration) }) }
	t.Cleanup(releaseHydration)
	var blocked atomic.Bool
	callbackName := "test:block_user_cache_hydration:" + common.GetUUID()
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callbackName, func(*gorm.DB) {
		if blocked.CompareAndSwap(false, true) {
			close(databaseRead)
			<-continueHydration
		}
	}))
	t.Cleanup(func() { DB.Callback().Query().Remove(callbackName) })

	type cacheResult struct {
		user *UserBase
		err  error
	}
	resultCh := make(chan cacheResult, 1)
	go func() {
		cached, err := GetUserCache(user.Id)
		resultCh <- cacheResult{user: cached, err: err}
	}()

	select {
	case <-databaseRead:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "GetUserCache did not reach the database hydration boundary")
	}
	require.NoError(t, populateUserCache(user))
	tradeNo := "hydrate-race-" + common.GetRandomString(8)
	reserved, err := reserveSubscriptionBalanceCacheQuota(user.Id, 4, tradeNo)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, reserved)
	releaseHydration()

	var result cacheResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "GetUserCache did not return after hydration was released")
	}
	assert.Nil(t, result.user)
	assert.ErrorIs(t, result.err, ErrQuotaCacheUnavailable,
		"a database snapshot read before the reservation must not escape after its in-flight fence appears")
	require.NoError(t, compensateSubscriptionBalanceCacheDebit(user.Id, 4, tradeNo))
}

func TestPersistentSubscriptionCommitFenceCannotBeDowngradedByCreditSync(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	tradeNo := "persistent-unknown-" + common.GetRandomString(8)
	fenceValue := "subscription_commit_unknown:" + tradeNo
	require.NoError(t, fenceUserQuotaCacheUntilReconciled(user.Id, fenceValue, ""))

	require.NoError(t, applyUserQuotaDelta(user.Id, 5, true),
		"the durable credit succeeds even though its cache sync is fenced")
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))
	actualFence, err := common.RDB.Get(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, fenceValue, actualFence)
	ttl, err := common.RDB.TTL(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), ttl, "a weaker cache-sync fence must not add an expiry to commit uncertainty")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
}

func TestQuotaFencePreservationNeverShortensForeignTTL(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 10)
	key := getUserQuotaUncertaintyKey(user.Id)
	foreignFence := "inflight:foreign-long-operation"
	originalTTL := 2 * time.Hour
	require.NoError(t, common.RDB.Set(context.Background(), key, foreignFence, originalTTL).Err())

	require.NoError(t, fenceUserQuotaCacheUncertainty(user.Id, "weaker-generic-fence"))
	assert.Equal(t, foreignFence, common.RDB.Get(context.Background(), key).Val())
	ttl, err := common.RDB.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, originalTTL, ttl)
}

func TestSubscriptionBalancePurchaseRollsBackDebitOrderAndSubscriptionTogether(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	forcedErr := errors.New("forced subscription transaction rollback")
	previousBeforeCommit := beforeSubscriptionBalanceTransactionCommit
	beforeSubscriptionBalanceTransactionCommit = func() error { return forcedErr }
	t.Cleanup(func() { beforeSubscriptionBalanceTransactionCommit = previousBeforeCommit })

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, forcedErr)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id), "a rolled-back purchase cannot durably charge the wallet")
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 10, cached.Quota, "the tentative Redis debit must be compensated")
	assert.False(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.Zero(t, orders)
	assert.Zero(t, subscriptions)
}

func TestSubscriptionBalancePurchaseReconcilesCommittedDriverError(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	commitTransportErr := errors.New("subscription commit result unavailable")
	previousRunner := runSubscriptionBalanceTransaction
	runSubscriptionBalanceTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	t.Cleanup(func() { runSubscriptionBalanceTransaction = previousRunner })

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id),
		"the durable order must reconcile a commit that only looked unsuccessful to the client")
	assert.Equal(t, 6, getUserQuotaFromDB(t, user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 6, cached.Quota)
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.EqualValues(t, 1, orders)
	assert.EqualValues(t, 1, subscriptions)
}

func TestSubscriptionBalanceInFlightFenceBlocksEvictionHydrationAndReserveUntilCommit(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	var hydrateErr error
	var reserveErr error
	var directPopulateErr error
	previousBeforeCommit := beforeSubscriptionBalanceTransactionCommit
	beforeSubscriptionBalanceTransactionCommit = func() error {
		if err := common.RDB.Del(context.Background(), getUserCacheKey(user.Id)).Err(); err != nil {
			return err
		}
		_, hydrateErr = GetUserCache(user.Id)
		_, reserveErr = TryReserveUserQuota(user.Id, 1)
		directPopulateErr = populateUserCache(user)
		return nil
	}
	t.Cleanup(func() { beforeSubscriptionBalanceTransactionCommit = previousBeforeCommit })

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
	assert.ErrorIs(t, hydrateErr, ErrQuotaCacheUnavailable)
	assert.ErrorIs(t, reserveErr, ErrQuotaCacheUnavailable)
	assert.ErrorIs(t, directPopulateErr, ErrQuotaCacheUnavailable)
	assert.Equal(t, 6, getUserQuotaFromDB(t, user.Id))
	assert.False(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 6, cached.Quota, "commit finalization must rebuild from the durable debited balance")

	operationKeys, keysErr := common.RDB.Keys(context.Background(), "subscription_balance_quota:*").Result()
	require.NoError(t, keysErr)
	require.Len(t, operationKeys, 1)
	state, stateErr := common.RDB.HGet(context.Background(), operationKeys[0], "state").Result()
	require.NoError(t, stateErr)
	assert.Equal(t, "committed", state)
	assert.Positive(t, common.RDB.TTL(context.Background(), operationKeys[0]).Val())
}

func TestSubscriptionBalanceRollbackAfterCacheEvictionRebuildsDurableBalance(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	forcedErr := errors.New("forced rollback after cache eviction")
	var hydrateErr error
	previousBeforeCommit := beforeSubscriptionBalanceTransactionCommit
	beforeSubscriptionBalanceTransactionCommit = func() error {
		if err := common.RDB.Del(context.Background(), getUserCacheKey(user.Id)).Err(); err != nil {
			return err
		}
		_, hydrateErr = GetUserCache(user.Id)
		return forcedErr
	}
	t.Cleanup(func() { beforeSubscriptionBalanceTransactionCommit = previousBeforeCommit })

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, forcedErr)
	assert.ErrorIs(t, hydrateErr, ErrQuotaCacheUnavailable)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.Zero(t, orders)
	assert.Zero(t, subscriptions)

	operationKeys, keysErr := common.RDB.Keys(context.Background(), "subscription_balance_quota:*").Result()
	require.NoError(t, keysErr)
	require.Len(t, operationKeys, 1)
	state, stateErr := common.RDB.HGet(context.Background(), operationKeys[0], "state").Result()
	require.NoError(t, stateErr)
	assert.Equal(t, "compensated", state)

	require.NoError(t, ensureUserQuotaCacheAvailable(user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 10, cached.Quota)
	assert.False(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
}

func TestSubscriptionBalancePurchaseFencesUnreconciledCommit(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	commitTransportErr := errors.New("subscription commit result unavailable")
	recheckErr := errors.New("subscription order recheck unavailable")
	previousRunner := runSubscriptionBalanceTransaction
	previousResolver := resolveSubscriptionBalancePurchaseCommitFn
	runSubscriptionBalanceTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveSubscriptionBalancePurchaseCommitFn = func(string, int, int) (bool, error) {
		return false, recheckErr
	}
	restoreHooks := func() {
		runSubscriptionBalanceTransaction = previousRunner
		resolveSubscriptionBalancePurchaseCommitFn = previousResolver
	}
	t.Cleanup(restoreHooks)

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, commitTransportErr)
	assert.ErrorIs(t, err, recheckErr)
	assert.ErrorIs(t, err, ErrSubscriptionBalanceCommitUncertain)
	assert.Equal(t, 6, getUserQuotaFromDB(t, user.Id), "an unknown commit must never restore a possibly committed debit")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	operationKeys, keysErr := common.RDB.Keys(context.Background(), "subscription_balance_quota:*").Result()
	require.NoError(t, keysErr)
	require.Len(t, operationKeys, 1)
	state, stateErr := common.RDB.HGet(context.Background(), operationKeys[0], "state").Result()
	require.NoError(t, stateErr)
	assert.Equal(t, "unknown", state)
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.EqualValues(t, 1, orders)
	assert.EqualValues(t, 1, subscriptions)

	// Once durable storage is readable again, the shared quota resolver hydrates
	// the committed balance and removes the fail-closed fence.
	restoreHooks()
	require.NoError(t, ensureUserQuotaCacheAvailable(user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 6, cached.Quota)
	assert.False(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
}

func TestSubscriptionBalancePurchaseTreatsInitiallyInvisibleOrderAsCommitUnknown(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	commitTransportErr := errors.New("subscription commit acknowledgement lost")
	previousRunner := runSubscriptionBalanceTransaction
	previousResolver := resolveSubscriptionBalancePurchaseCommitFn
	previousBeforeCommit := beforeSubscriptionBalanceTransactionCommit
	var hydrateErr error
	var reserveErr error
	beforeSubscriptionBalanceTransactionCommit = func() error {
		if err := common.RDB.Del(context.Background(), getUserCacheKey(user.Id)).Err(); err != nil {
			return err
		}
		_, hydrateErr = GetUserCache(user.Id)
		_, reserveErr = TryReserveUserQuota(user.Id, 1)
		return nil
	}
	runSubscriptionBalanceTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	// Simulate a server-side COMMIT that is not visible to the first read on a
	// different connection. A single absent-order result cannot prove rollback.
	resolveSubscriptionBalancePurchaseCommitFn = func(string, int, int) (bool, error) {
		return false, nil
	}
	restoreHooks := func() {
		runSubscriptionBalanceTransaction = previousRunner
		resolveSubscriptionBalancePurchaseCommitFn = previousResolver
		beforeSubscriptionBalanceTransactionCommit = previousBeforeCommit
	}
	t.Cleanup(restoreHooks)

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, commitTransportErr)
	assert.ErrorIs(t, err, ErrSubscriptionBalanceCommitUncertain)
	assert.ErrorIs(t, hydrateErr, ErrQuotaCacheUnavailable)
	assert.ErrorIs(t, reserveErr, ErrQuotaCacheUnavailable)
	assert.Equal(t, 6, getUserQuotaFromDB(t, user.Id), "the actual delayed-visible commit must not be compensated")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.EqualValues(t, 1, orders)
	assert.EqualValues(t, 1, subscriptions)

	operationKeys, keysErr := common.RDB.Keys(context.Background(), "subscription_balance_quota:*").Result()
	require.NoError(t, keysErr)
	require.Len(t, operationKeys, 1)
	state, stateErr := common.RDB.HGet(context.Background(), operationKeys[0], "state").Result()
	require.NoError(t, stateErr)
	assert.Equal(t, "unknown", state)

	restoreHooks()
	require.NoError(t, ensureUserQuotaCacheAvailable(user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 6, cached.Quota)
}

func TestSubscriptionBalanceUnknownRollbackStaysFencedWithoutDurableOrder(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)

	commitTransportErr := errors.New("subscription commit outcome unavailable after callback")
	previousRunner := runSubscriptionBalanceTransaction
	previousResolver := resolveSubscriptionBalancePurchaseCommitFn
	runSubscriptionBalanceTransaction = func(fn func(*gorm.DB) error) error {
		return DB.Transaction(func(tx *gorm.DB) error {
			if err := fn(tx); err != nil {
				return err
			}
			// The fixture rolls back, but the purchase layer only knows that its
			// callback completed before a transaction-level error was returned.
			return commitTransportErr
		})
	}
	resolveSubscriptionBalancePurchaseCommitFn = func(string, int, int) (bool, error) {
		return false, nil
	}
	restoreHooks := func() {
		runSubscriptionBalanceTransaction = previousRunner
		resolveSubscriptionBalancePurchaseCommitFn = previousResolver
	}
	t.Cleanup(restoreHooks)

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, commitTransportErr)
	assert.ErrorIs(t, err, ErrSubscriptionBalanceCommitUncertain)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.Zero(t, orders)
	assert.Zero(t, subscriptions)

	restoreHooks()
	assert.ErrorIs(t, ensureUserQuotaCacheAvailable(user.Id), ErrQuotaCacheUnavailable,
		"absence of the order on one connection must never be treated as proof of rollback")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
}

func TestSubscriptionBalancePurchaseRejectsPriceChangeBeforeDebit(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 3.1)

	previousRunner := runSubscriptionBalanceTransaction
	runSubscriptionBalanceTransaction = func(fn func(*gorm.DB) error) error {
		// Both prices ceil to four quota units. The purchase must still reject the
		// changed commercial price instead of accepting it merely because the
		// derived integer charge happens to match.
		if err := DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Update("price_amount", 3.2).Error; err != nil {
			return err
		}
		return DB.Transaction(fn)
	}
	t.Cleanup(func() { runSubscriptionBalanceTransaction = previousRunner })

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorContains(t, err, "套餐价格已变更")
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 10, cached.Quota)
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.Zero(t, orders)
	assert.Zero(t, subscriptions)
}

func TestConcurrentSubscriptionBalancePurchasesCannotOverspend(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 6)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			errs <- PurchaseSubscriptionWithBalance(user.Id, plan.Id)
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	blocked := 0
	for range 2 {
		err := <-errs
		if err == nil {
			successes++
		} else if errors.Is(err, ErrQuotaCacheUnavailable) || strings.Contains(err.Error(), "余额不足") {
			blocked++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, blocked)
	assert.Equal(t, 4, getUserQuotaFromDB(t, user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 4, cached.Quota)
	assert.ErrorContains(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id), "余额不足",
		"once the in-flight purchase is finalized, a retry must observe the durable insufficient balance")
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.EqualValues(t, 1, orders)
	assert.EqualValues(t, 1, subscriptions)
}

func TestSubscriptionBalanceRedisJournalMakesMutationsIdempotent(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	tradeNo := "subscription-idempotent-" + common.GetRandomString(8)

	result, err := reserveSubscriptionBalanceCacheQuota(user.Id, 4, tradeNo)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	result, err = reserveSubscriptionBalanceCacheQuota(user.Id, 4, tradeNo)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	cachedQuota, err := common.RDB.HGet(context.Background(), getUserCacheKey(user.Id), "Quota").Int()
	require.NoError(t, err)
	assert.Equal(t, 6, cachedQuota, "replaying the same reservation must not debit twice")
	assert.True(t, common.RDB.Exists(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Val() == 1)

	require.NoError(t, compensateSubscriptionBalanceCacheDebit(user.Id, 4, tradeNo))
	require.NoError(t, compensateSubscriptionBalanceCacheDebit(user.Id, 4, tradeNo))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 10, cached.Quota, "replaying the same compensation must not credit twice")
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))

	state, err := common.RDB.HGet(context.Background(), subscriptionBalanceQuotaOperationKey(tradeNo), "state").Result()
	require.NoError(t, err)
	assert.Equal(t, "compensated", state)
}

func TestSubscriptionBalanceFinalizersPreserveForeignFenceOwnership(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		truncateTables(t)
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 10)
		require.NoError(t, populateUserCache(user))
		tradeNo := "foreign-commit-" + common.GetRandomString(8)
		result, err := reserveSubscriptionBalanceCacheQuota(user.Id, 4, tradeNo)
		require.NoError(t, err)
		require.Equal(t, cacheQuotaOK, result)
		foreignFence := "inflight:foreign-operation"
		require.NoError(t, common.RDB.Set(context.Background(), getUserQuotaUncertaintyKey(user.Id), foreignFence, 0).Err())

		finalizeCommittedSubscriptionBalanceCacheDebit(user.Id, 4, tradeNo)
		state, err := common.RDB.HGet(context.Background(), subscriptionBalanceQuotaOperationKey(tradeNo), "state").Result()
		require.NoError(t, err)
		assert.Equal(t, "reserved", state, "foreign ownership must prevent a committed journal transition")
		assert.Equal(t, foreignFence, common.RDB.Get(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Val())
		assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	})

	t.Run("unknown", func(t *testing.T) {
		truncateTables(t)
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 10)
		require.NoError(t, populateUserCache(user))
		tradeNo := "foreign-unknown-" + common.GetRandomString(8)
		result, err := reserveSubscriptionBalanceCacheQuota(user.Id, 4, tradeNo)
		require.NoError(t, err)
		require.Equal(t, cacheQuotaOK, result)
		foreignFence := "inflight:foreign-operation"
		require.NoError(t, common.RDB.Set(context.Background(), getUserQuotaUncertaintyKey(user.Id), foreignFence, 0).Err())

		err = markSubscriptionBalanceCacheCommitUnknown(user.Id, 4, tradeNo)
		assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
		state, stateErr := common.RDB.HGet(context.Background(), subscriptionBalanceQuotaOperationKey(tradeNo), "state").Result()
		require.NoError(t, stateErr)
		assert.Equal(t, "reserved", state, "foreign ownership must prevent an unknown journal transition")
		assert.Equal(t, foreignFence, common.RDB.Get(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Val())
		assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	})
}

func TestSubscriptionBalanceUnknownFinalizerRecreatesEvictedJournalAtomically(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	tradeNo := "missing-unknown-journal-" + common.GetRandomString(8)
	result, err := reserveSubscriptionBalanceCacheQuota(user.Id, 4, tradeNo)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	journalKey := subscriptionBalanceQuotaOperationKey(tradeNo)
	require.NoError(t, common.RDB.Del(context.Background(), journalKey).Err())

	require.NoError(t, markSubscriptionBalanceCacheCommitUnknown(user.Id, 4, tradeNo))
	journal, err := common.RDB.HGetAll(context.Background(), journalKey).Result()
	require.NoError(t, err)
	assert.Equal(t, "unknown", journal["state"])
	assert.Equal(t, fmt.Sprint(user.Id), journal["user_id"])
	assert.Equal(t, "4", journal["quota"])
	ttl, err := common.RDB.TTL(context.Background(), journalKey).Result()
	require.NoError(t, err)
	assert.Positive(t, ttl, "a recreated unknown journal must never become a permanent half-key")
	assert.Equal(t, "subscription_commit_unknown:"+tradeNo,
		common.RDB.Get(context.Background(), getUserQuotaUncertaintyKey(user.Id)).Val())
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
}

func TestSubscriptionBalanceCommitFinalizeLostReplyRemainsRecoverable(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)
	common.RDB.AddHook(&failAfterMatchingRedisCommand{
		needle: "'state', 'committed'",
		err:    errors.New("commit cache finalize reply lost"),
	})

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id),
		"the durable purchase remains successful when only its idempotent cache-finalize reply is lost")
	assert.Equal(t, 6, getUserQuotaFromDB(t, user.Id))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 6, cached.Quota)
	operationKeys, err := common.RDB.Keys(context.Background(), "subscription_balance_quota:*").Result()
	require.NoError(t, err)
	require.Len(t, operationKeys, 1)
	state, err := common.RDB.HGet(context.Background(), operationKeys[0], "state").Result()
	require.NoError(t, err)
	assert.Equal(t, "committed", state)
}

func TestSubscriptionBalanceReserveLostReplyFailsClosed(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)
	lostReplyErr := errors.New("reserved quota reply lost")
	common.RDB.AddHook(&failAfterMatchingRedisCommand{needle: "'state', 'reserved'", err: lostReplyErr})

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, lostReplyErr)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id), "an uncertain Redis reserve cannot reach the durable debit")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "the possibly debited hash must be removed")
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.Zero(t, orders)
	assert.Zero(t, subscriptions)

	require.NoError(t, ensureUserQuotaCacheAvailable(user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 10, cached.Quota)
}

func TestSubscriptionBalanceCompensationLostReplyCannotCreateStaleHighCache(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useUnitSubscriptionBalancePrice(t)
	user := createReserveTestUser(t, 10)
	plan := createBalancePurchasePlan(t, 4)
	forcedRollbackErr := errors.New("forced rollback after subscription writes")
	lostReplyErr := errors.New("compensation reply lost")
	common.RDB.AddHook(&failAfterMatchingRedisCommand{needle: "'state', 'compensated'", err: lostReplyErr})

	previousBeforeCommit := beforeSubscriptionBalanceTransactionCommit
	beforeSubscriptionBalanceTransactionCommit = func() error { return forcedRollbackErr }
	t.Cleanup(func() { beforeSubscriptionBalanceTransactionCommit = previousBeforeCommit })

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id)
	assert.ErrorIs(t, err, forcedRollbackErr)
	assert.ErrorIs(t, err, lostReplyErr)
	assert.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)),
		"an executed compensation with a lost reply must be fenced, never replayed into stale-high quota")
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	orders, subscriptions := countBalancePurchaseRows(t, user.Id)
	assert.Zero(t, orders)
	assert.Zero(t, subscriptions)

	operationKeys, keysErr := common.RDB.Keys(context.Background(), "subscription_balance_quota:*").Result()
	require.NoError(t, keysErr)
	require.Len(t, operationKeys, 1)
	tradeNo := strings.TrimPrefix(operationKeys[0], "subscription_balance_quota:")
	state, stateErr := common.RDB.HGet(context.Background(), operationKeys[0], "state").Result()
	require.NoError(t, stateErr)
	assert.Equal(t, "compensated", state, "Redis executed the compensation before its reply was lost")

	require.NoError(t, ensureUserQuotaCacheAvailable(user.Id))
	require.NoError(t, compensateSubscriptionBalanceCacheDebit(user.Id, 4, tradeNo))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, getUserQuotaFromDB(t, user.Id), cached.Quota,
		"replaying compensation after reconciliation must remain exactly once")
}
