package model

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionReserveRejectsQuotaAlreadyReservedByConcurrentRequest(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 200, 60)
	useUserCacheMiniRedis(t)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).Update("remain_quota", 100).Error)
	_, err := GetTokenByKey(fixture.token.Key, true)
	require.NoError(t, err)

	entered, release := make(chan struct{}), make(chan struct{})
	type reserveResult struct {
		reserved bool
		err      error
	}
	done := make(chan reserveResult, 1)
	var releaseOnce sync.Once
	var blocked atomic.Bool
	const callback = "test:subscription-concurrent-token-reserve"
	require.NoError(t, DB.Callback().Update().Before("gorm:begin_transaction").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" && blocked.CompareAndSwap(false, true) {
			// The other real request already reserved Redis quota, but has not
			// opened its SQL transaction. A DB-only allowance check is stale.
			close(entered)
			<-release
		}
	}))
	drained := false
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		if !drained {
			select {
			case result := <-done:
				assert.NoError(t, result.err)
			case <-time.After(2 * time.Second):
				t.Error("concurrent token reservation did not finish cleanup")
			}
		}
		assert.NoError(t, DB.Callback().Update().Remove(callback))
	})
	go func() {
		reserved, err := TryReserveTokenQuota(fixture.token.Id, fixture.token.Key, 80, false)
		done <- reserveResult{reserved: reserved, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent request did not reach its SQL persistence stage")
	}

	fixture.params.ActualQuota = 90 // Extra 30 exceeds the 20 left in the live cache.
	_, err = ReserveSubscriptionBilling(fixture.params)
	require.ErrorIs(t, err, ErrSubscriptionTokenQuotaInsufficient)
	cached, err := cacheGetTokenByKey(fixture.token.Key)
	require.NoError(t, err, "a confirmed rejection must not permanently fence the token")
	assert.Equal(t, 20, cached.RemainQuota)
	var sub UserSubscription
	require.NoError(t, DB.First(&sub, fixture.sub.Id).Error)
	assert.EqualValues(t, 60, sub.AmountUsed, "rejected token allowance must roll back the subscription reservation")
	assert.Equal(t, 100, getTokenFromDB(t, fixture.token.Id).RemainQuota)

	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-done:
		drained = true
		require.NoError(t, result.err)
		assert.True(t, result.reserved)
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent token reservation did not persist")
	}
	assert.Equal(t, 20, getTokenFromDB(t, fixture.token.Id).RemainQuota)
}

func TestSubscriptionSettlementRetainsEarlierReservationReconciliation(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 200, 60)
	enableSubscriptionSettlementCache(t, fixture)
	const callback = "test:subscription-reserve-cache-expiry"
	var once atomic.Bool
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "subscription_pre_consume_records" && once.CompareAndSwap(false, true) {
			// The database reserve commits, but expiry prevents its cache
			// finalizer from confirming the generation it originally debited.
			require.NoError(t, common.RDB.Del(context.Background(), getTokenCacheKey(fixture.token.Key)).Err())
		}
	}))
	t.Cleanup(func() { assert.NoError(t, DB.Callback().Update().Remove(callback)) })
	fixture.params.ActualQuota = 120
	_, err := ReserveSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error)
	require.True(t, record.ReconciliationRequired)
	require.Positive(t, common.RDB.SCard(context.Background(), getTaskBillingTokenQuotaFenceKey(fixture.token.Key)).Val())

	fixture.params.PreConsumedQuota = 120
	fixture.params.TokenConsumedQuota = 120
	_, err = SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	require.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error)
	assert.True(t, record.ReconciliationRequired, "settlement with no further delta cannot confirm the earlier reserve's cache outcome")
	assertSubscriptionSettlementBalances(t, fixture, 120, 1000, 880, 120)

	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("id = ?", record.Id).
		UpdateColumn("updated_at", common.GetTimestamp()-86400).Error)
	_, err = CleanupSubscriptionPreConsumeRecords(1)
	require.NoError(t, err)
	assert.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error,
		"the unresolved reservation fence must retain its durable receipt")
}
