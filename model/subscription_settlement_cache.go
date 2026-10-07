package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Unlike older uncertainty resolvers, subscription settlement fences preserve
// the live hash. Reloading DB here could erase another request's Redis debit
// whose synchronous SQL write is still in flight. Uncertain fences therefore
// require explicit reconciliation; a terminal SQL receipt alone is not enough
// to prove whether an interrupted Redis command applied.
var errSubscriptionCacheRejected = errors.New("subscription cache delta was not applied")

const subscriptionBillingFencePrefix = "inflight:subscription-billing|"

type subscriptionBillingPreparedCache struct {
	target taskBillingCacheMutation
	err    error
}
type subscriptionBillingPreparedCaches map[string]subscriptionBillingPreparedCache

type subscriptionBillingCacheMutation struct {
	target           taskBillingCacheMutation
	debitApplied     bool
	requireAvailable bool
	uncertain        bool
}

func prepareSubscriptionBillingCaches(p SubscriptionBillingParams, operation string) subscriptionBillingPreparedCaches {
	prepared := subscriptionBillingPreparedCaches{}
	if !common.RedisEnabled {
		return prepared
	}
	actual := p.ActualQuota
	if operation == "refund" {
		actual = 0
	}
	if actual > p.PreConsumedQuota && p.AllowWalletOverflow && operation == "settle" {
		target, err := prepareSubscriptionBillingCacheTarget(taskBillingCacheMutation{kind: taskBillingCacheUser, userId: p.UserId, delta: -1}, p.RequestId)
		prepared[taskBillingCacheUser] = subscriptionBillingPreparedCache{target: target, err: err}
	}
	if p.TokenId > 0 && actual != p.TokenConsumedQuota {
		delta := int64(p.TokenConsumedQuota) - int64(actual)
		target, err := prepareSubscriptionBillingCacheTarget(taskBillingCacheMutation{kind: taskBillingCacheToken, tokenId: p.TokenId, tokenKey: p.TokenKey, delta: delta, tokenUsedDelta: -delta}, p.RequestId)
		prepared[taskBillingCacheToken] = subscriptionBillingPreparedCache{target: target, err: err}
	}
	return prepared
}

// Peer subscription operations preserve the same live generation and apply
// additive deltas. They may overlap without rebuilding or releasing each
// other's fences; generic spending stays blocked until all owners finish.
func prepareSubscriptionBillingCacheTarget(target taskBillingCacheMutation, requestID string) (taskBillingCacheMutation, error) {
	prepared, err := prepareTaskBillingCacheDebitWithTarget(target)
	if err == nil {
		return prepared, nil
	}
	m := &subscriptionBillingCacheMutation{target: prepared}
	members, readErr := common.RDB.SMembers(context.Background(), m.fenceKey()).Result()
	if readErr != nil || len(members) == 0 {
		return prepared, err
	}
	for _, member := range members {
		if !strings.HasPrefix(member, subscriptionBillingFencePrefix) || strings.HasPrefix(member, subscriptionBillingFencePrefix+requestID+"|") {
			return prepared, err
		}
	}
	captureTaskBillingCacheGeneration(&prepared)
	if prepared.creditGeneration == "" {
		return prepared, err
	}
	prepared.operationFence = members[0]
	m.target = prepared
	if inspectErr := m.apply(0); inspectErr != nil {
		return prepared, err
	}
	prepared.operationFence = ""
	return prepared, nil
}

// A prior failed attempt may have left a fence even if its SQL rolled back.
// Preserve its request receipt across subsequent zero-delta or refund stages.
func subscriptionBillingHasRequestFence(p SubscriptionBillingParams) bool {
	if !common.RedisEnabled {
		return false
	}
	keys := []string{getTaskBillingUserQuotaFenceKey(p.UserId)}
	if p.TokenId > 0 {
		keys = append(keys, getTaskBillingTokenQuotaFenceKey(p.TokenKey))
	}
	for _, key := range keys {
		members, err := common.RDB.SMembers(context.Background(), key).Result()
		if err != nil {
			return true
		}
		for _, member := range members {
			if strings.HasPrefix(member, subscriptionBillingFencePrefix+p.RequestId+"|") {
				return true
			}
		}
	}
	return false
}

func (prepared subscriptionBillingPreparedCaches) begin(kind string, delta int64, requestID string, requireAvailable bool) (*subscriptionBillingCacheMutation, error) {
	if !common.RedisEnabled || delta == 0 {
		return nil, nil
	}
	entry, ok := prepared[kind]
	if !ok || entry.err != nil {
		return nil, fmt.Errorf("%w: prepare subscription %s cache: %v", ErrQuotaCacheUnavailable, kind, entry.err)
	}
	target := entry.target
	target.delta = delta
	if kind == taskBillingCacheToken {
		target.tokenUsedDelta = -delta
	}
	// A credit can leave an absent/stale-low mirror untouched; it must never
	// hydrate and overwrite a concurrently created cache generation.
	if delta > 0 && target.creditGeneration == "" {
		return nil, nil
	}
	target.operationFence = subscriptionBillingFencePrefix + requestID + "|" + common.GetUUID()
	mutation := &subscriptionBillingCacheMutation{target: target, requireAvailable: requireAvailable}
	const register = `redis.call('SADD', KEYS[1], ARGV[1]); return 1`
	if err := common.RDB.Eval(context.Background(), register, []string{mutation.fenceKey()}, target.operationFence).Err(); err != nil {
		mutation.uncertain = true
		return mutation, fmt.Errorf("%w: register subscription cache fence: %v", ErrQuotaCacheUnavailable, err)
	}
	if delta < 0 {
		if err := mutation.apply(delta); err != nil {
			mutation.uncertain = !errors.Is(err, errSubscriptionCacheRejected)
			return mutation, err
		}
		mutation.debitApplied = true
	}
	return mutation, nil
}

func (m *subscriptionBillingCacheMutation) fenceKey() string {
	if m.target.kind == taskBillingCacheUser {
		return getTaskBillingUserQuotaFenceKey(m.target.userId)
	}
	return getTaskBillingTokenQuotaFenceKey(m.target.tokenKey)
}

func (m *subscriptionBillingCacheMutation) apply(delta int64) error {
	t := m.target
	var value int
	var err error
	if t.kind == taskBillingCacheUser {
		value, err = common.RDB.Eval(context.Background(), taskUserQuotaDeltaScript, []string{getUserCacheKey(t.userId), getUserQuotaUncertaintyKey(t.userId), getTaskBillingUserQuotaFenceKey(t.userId)}, delta, t.userId, userCacheSchemaVersion, t.operationFence, subscriptionBillingFencePrefix, t.creditGeneration, common.MaxWalletQuota).Int()
	} else {
		reserveMode := ""
		if m.requireAvailable {
			reserveMode = "reserve"
		}
		value, err = common.RDB.Eval(context.Background(), taskTokenQuotaDeltaScript, []string{getTokenCacheKey(t.tokenKey), getTaskBillingTokenQuotaFenceKey(t.tokenKey)}, delta, -delta, t.tokenId, common.GetTimestamp(), t.operationFence, subscriptionBillingFencePrefix, t.creditGeneration, reserveMode).Int()
	}
	if err == nil && value == 0 && m.requireAvailable && delta < 0 {
		return errors.Join(errSubscriptionCacheRejected, ErrSubscriptionTokenQuotaInsufficient)
	}
	if err == nil && value != 1 {
		return errors.Join(ErrQuotaCacheUnavailable, errSubscriptionCacheRejected)
	}
	result, err := quotaResultFromLua(value, err)
	if err != nil || result != cacheQuotaOK {
		return fmt.Errorf("%w: subscription %s cache delta acknowledgement unknown/rejected: %v (%d)", ErrQuotaCacheUnavailable, t.kind, err, result)
	}
	return nil
}

func (m *subscriptionBillingCacheMutation) finish(committed bool) bool {
	if m.uncertain {
		m.report()
		return false
	}
	var delta int64
	if committed && m.target.delta > 0 {
		delta = m.target.delta
	}
	if !committed && m.debitApplied {
		delta = -m.target.delta
	}
	if delta != 0 {
		if err := m.apply(delta); err != nil {
			m.uncertain = true
			m.report()
			return false
		}
	}
	if !committed && !m.debitApplied {
		// An explicit Lua rejection (or an unapplied credit) changed no balance.
		// Remove only our marker, even if expiry replaced the cache generation.
		if err := common.RDB.SRem(context.Background(), m.fenceKey(), m.target.operationFence).Err(); err != nil {
			m.report()
			return false
		}
		return true
	}
	cacheKey := getUserCacheKey(m.target.userId)
	if m.target.kind == taskBillingCacheToken {
		cacheKey = getTokenCacheKey(m.target.tokenKey)
	}
	const release = `
if redis.call('HGET', KEYS[1], 'QuotaGeneration') ~= ARGV[2] then return 0 end
redis.call('SREM', KEYS[2], ARGV[1])
return 1`
	result, err := common.RDB.Eval(context.Background(), release, []string{cacheKey, m.fenceKey()}, m.target.operationFence, m.target.creditGeneration).Int()
	if err != nil || result != 1 {
		m.uncertain = true
		m.report()
		return false
	}
	return true
}

func (m *subscriptionBillingCacheMutation) report() {
	common.SysError(fmt.Sprintf("subscription billing cache requires reconciliation: fence=%s target=%s user=%d token=%d delta=%d; automatic cache reload disabled", m.target.operationFence, m.target.kind, m.target.userId, m.target.tokenId, m.target.delta))
}
