package model

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func enableSubscriptionSettlementCache(t *testing.T, fixture subscriptionSettlementFixture) {
	t.Helper()
	useUserCacheMiniRedis(t)
	require.NoError(t, populateUserCache(fixture.user))
	_, err := GetTokenByKey(fixture.token.Key, true)
	require.NoError(t, err)
}

func TestSubscriptionBillingPreservesGenericDebitAlreadyAppliedToLiveCache(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	enableSubscriptionSettlementCache(t, fixture)
	// Pause the real generic debit between its Redis and synchronous SQL stages.
	// DB still says 1000 while the live cache already includes the other -100.
	result, err := cacheApplyUserQuotaDelta(fixture.user.Id, -100)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	require.Equal(t, 1000, getUserQuotaFromDB(t, fixture.user.Id))
	_, err = SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	cached, err := cacheGetUserBase(fixture.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 840, cached.Quota, "settlement must preserve the other request's pending debit")
	require.NoError(t, persistUserQuotaDeltaDirect(fixture.user.Id, -100))
	assertSubscriptionSettlementBalances(t, fixture, 100, 840, 840, 160)
	cached, err = cacheGetUserBase(fixture.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 840, cached.Quota)
	assert.Zero(t, common.RDB.SCard(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id)).Val())
}

func TestSubscriptionBillingConfirmedRollbackRestoresOnlyItsCacheDeltas(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	enableSubscriptionSettlementCache(t, fixture)
	forced := errors.New("receipt write rejected")
	const callback = "test:subscription_cache_rollback"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "subscription_pre_consume_records" {
			tx.AddError(forced)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	_, err := SettleSubscriptionBilling(fixture.params)
	require.ErrorIs(t, err, forced)
	assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
	cached, err := cacheGetUserBase(fixture.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1000, cached.Quota)
	token, err := cacheGetTokenByKey(fixture.token.Key)
	require.NoError(t, err)
	assert.Equal(t, 940, token.RemainQuota)
	assert.Equal(t, 60, token.UsedQuota)
	assert.Zero(t, common.RDB.SCard(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id)).Val())
	assert.Zero(t, common.RDB.SCard(context.Background(), getTaskBillingTokenQuotaFenceKey(fixture.token.Key)).Val())
}

type subscriptionCacheLostReply struct {
	script string
	fired  atomic.Bool
}

func (h *subscriptionCacheLostReply) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (h *subscriptionCacheLostReply) AfterProcess(_ context.Context, cmd redis.Cmder) error {
	args := cmd.Args()
	if len(args) < 2 || args[1] != h.script || h.fired.Load() {
		return nil
	}
	for _, arg := range args {
		if v, ok := arg.(string); ok && strings.HasPrefix(v, subscriptionBillingFencePrefix) && h.fired.CompareAndSwap(false, true) {
			return errors.New("subscription Redis acknowledgement lost after execution")
		}
	}
	return nil
}
func (h *subscriptionCacheLostReply) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (h *subscriptionCacheLostReply) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestSubscriptionBillingLostCacheAcknowledgementNeverBlindlyCompensates(t *testing.T) {
	for _, mode := range []string{"wallet_debit", "token_debit", "token_credit"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			enableSubscriptionSettlementCache(t, fixture)
			script := taskTokenQuotaDeltaScript
			if mode == "wallet_debit" {
				script = taskUserQuotaDeltaScript
			}
			if mode == "token_credit" {
				fixture.params.ActualQuota = 0
			}
			hook := &subscriptionCacheLostReply{script: script}
			common.RDB.AddHook(hook)
			_, err := SettleSubscriptionBilling(fixture.params)
			require.True(t, hook.fired.Load())
			if mode == "token_credit" {
				require.NoError(t, err, "committed credit is successful even when cache sync is uncertain")
				assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
				_, err = SettleSubscriptionBilling(fixture.params)
				require.NoError(t, err)
				assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
			} else {
				require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
				assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
			}
			if mode == "wallet_debit" {
				_, err = TryReserveUserQuota(fixture.user.Id, 1)
				require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
				assert.Positive(t, common.RDB.SCard(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id)).Val())
			} else {
				_, err = TryReserveTokenQuota(fixture.token.Id, fixture.token.Key, 1, false)
				require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
				assert.Positive(t, common.RDB.SCard(context.Background(), getTaskBillingTokenQuotaFenceKey(fixture.token.Key)).Val())
			}
		})
	}
}

func TestSubscriptionBillingCommitUnknownRetainsFenceAndNeverReplays(t *testing.T) {
	for _, mode := range []string{"commit_applied", "commit_not_applied", "rollback_unknown"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			enableSubscriptionSettlementCache(t, fixture)
			sqlDB, err := DB.DB()
			require.NoError(t, err)
			original, statement := DB.ConnPool, DB.Statement.ConnPool
			pool := &batchOutcomeTestPool{DB: sqlDB, mode: mode, forced: errors.New("subscription outer transaction acknowledgement lost")}
			DB.ConnPool, DB.Statement.ConnPool = pool, pool
			t.Cleanup(func() { DB.ConnPool, DB.Statement.ConnPool = original, statement })
			if mode == "rollback_unknown" {
				const callback = "test:subscription_rollback_unknown"
				require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "subscription_pre_consume_records" {
						tx.AddError(errors.New("receipt rejected"))
					}
				}))
				t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
			}
			_, err = SettleSubscriptionBilling(fixture.params)
			require.ErrorIs(t, err, ErrSubscriptionBillingUncertain)
			DB.ConnPool, DB.Statement.ConnPool = original, statement
			assert.Positive(t, common.RDB.SCard(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id)).Val())
			assert.Positive(t, common.RDB.SCard(context.Background(), getTaskBillingTokenQuotaFenceKey(fixture.token.Key)).Val())
			_, err = TryReserveUserQuota(fixture.user.Id, 1)
			require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
			_, err = SettleSubscriptionBilling(fixture.params)
			if mode == "commit_applied" {
				require.NoError(t, err, "a matching durable terminal receipt may be read without replaying money")
				assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
			} else {
				require.Error(t, err)
				assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
			}
			_, err = TryReserveUserQuota(fixture.user.Id, 1)
			require.ErrorIs(t, err, ErrQuotaCacheUnavailable, "a SQL receipt cannot prove lost cache acknowledgements")
		})
	}
}

func TestSubscriptionBillingUsesFullTokenBalanceDomain(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	remain, used := int(-int64(1)<<40), int(int64(1)<<41)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).Updates(map[string]any{"remain_quota": remain, "used_quota": used, "unlimited_quota": true}).Error)
	_, err := SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	token := getTokenFromDB(t, fixture.token.Id)
	assert.Equal(t, remain-100, token.RemainQuota)
	assert.Equal(t, used+100, token.UsedQuota)
}

func TestSubscriptionBillingCleanupKeepsTerminalReceiptsAndUnresolvedRequests(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	_, err := SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", fixture.params.RequestId).UpdateColumns(map[string]any{"updated_at": common.GetTimestamp() - int64((30*24*time.Hour)/time.Second), "reconciliation_required": true}).Error)
	_, err = CleanupSubscriptionPreConsumeRecords(1)
	require.NoError(t, err)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error)
	assert.Equal(t, "settled", record.Status)
}

func TestSubscriptionBillingKnownCacheRejectionReleasesOnlyItsFence(t *testing.T) {
	for _, mode := range []string{"expired_hash", "changed_generation", "foreign_fence"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			enableSubscriptionSettlementCache(t, fixture)
			const callback = "test:subscription_change_cache_before_lock"
			var once atomic.Bool
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != "subscription_pre_consume_records" || !once.CompareAndSwap(false, true) {
					return
				}
				switch mode {
				case "expired_hash":
					require.NoError(t, common.RDB.Del(context.Background(), getUserCacheKey(fixture.user.Id)).Err())
				case "changed_generation":
					require.NoError(t, common.RDB.HSet(context.Background(), getUserCacheKey(fixture.user.Id), "QuotaGeneration", "new-generation").Err())
				case "foreign_fence":
					require.NoError(t, common.RDB.SAdd(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id), "unrelated-operation").Err())
				}
			}))
			t.Cleanup(func() { _ = DB.Callback().Query().Remove(callback) })
			_, err := SettleSubscriptionBilling(fixture.params)
			require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
			assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
			members, err := common.RDB.SMembers(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id)).Result()
			require.NoError(t, err)
			if mode == "foreign_fence" {
				assert.Equal(t, []string{"unrelated-operation"}, members)
			} else {
				assert.Empty(t, members)
			}
		})
	}
}

func TestSubscriptionBillingPeerFencePreservesLiveDeltasAndOtherOwner(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	enableSubscriptionSettlementCache(t, fixture)
	result, err := cacheApplyUserQuotaDelta(fixture.user.Id, -100)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)
	peerFence := subscriptionBillingFencePrefix + "other-request|other-operation"
	require.NoError(t, common.RDB.SAdd(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id), peerFence).Err())
	_, err = SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err, "a peer's live generation must not make this completed request free")
	assert.Equal(t, "840", common.RDB.HGet(context.Background(), getUserCacheKey(fixture.user.Id), "Quota").Val())
	members, err := common.RDB.SMembers(context.Background(), getTaskBillingUserQuotaFenceKey(fixture.user.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, []string{peerFence}, members, "only this request's fence may be released")
	_, err = TryReserveUserQuota(fixture.user.Id, 1)
	require.ErrorIs(t, err, ErrQuotaCacheUnavailable, "generic spending remains blocked by the other owner")
}

func TestSubscriptionBillingZeroDeltaKeepsPriorUncommittedCacheEvidence(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	enableSubscriptionSettlementCache(t, fixture)
	hook := &subscriptionCacheLostReply{script: taskUserQuotaDeltaScript}
	common.RDB.AddHook(hook)
	_, err := SettleSubscriptionBilling(fixture.params)
	require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	fixture.params.ActualQuota = fixture.params.PreConsumedQuota
	_, err = SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error)
	assert.True(t, record.ReconciliationRequired, "zero-delta success must retain evidence of an earlier Redis acknowledgement loss")
	_, err = TryReserveUserQuota(fixture.user.Id, 1)
	require.ErrorIs(t, err, ErrQuotaCacheUnavailable)
}
