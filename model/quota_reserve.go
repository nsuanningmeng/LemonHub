package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type cacheQuotaResult int

var ErrQuotaCacheUnavailable = errors.New("quota cache unavailable")

const (
	cacheQuotaInsufficient cacheQuotaResult = iota
	cacheQuotaOK
	cacheQuotaMiss
	cacheQuotaFenced
)

const userQuotaReserveScript = `
if redis.call('EXISTS', KEYS[2]) == 1 or redis.call('EXISTS', KEYS[3]) == 1 then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
local quota = tonumber(redis.call('HGET', KEYS[1], 'Quota'))
if quota == nil or quota < tonumber(ARGV[1]) then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'Quota', -tonumber(ARGV[1]))
return 1`

const userQuotaDeltaScript = `
if redis.call('EXISTS', KEYS[2]) == 1 or redis.call('EXISTS', KEYS[3]) == 1 then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'Quota', tonumber(ARGV[1]))
return 1`

const tokenQuotaReserveScript = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
local remain = tonumber(redis.call('HGET', KEYS[1], 'RemainQuota'))
if remain == nil or remain < tonumber(ARGV[1]) then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', -tonumber(ARGV[1]))
redis.call('HINCRBY', KEYS[1], 'UsedQuota', tonumber(ARGV[1]))
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

const tokenQuotaDeltaScript = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', tonumber(ARGV[1]))
redis.call('HINCRBY', KEYS[1], 'UsedQuota', -tonumber(ARGV[1]))
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

func quotaResultFromLua(result int, err error) (cacheQuotaResult, error) {
	if err != nil {
		return cacheQuotaMiss, err
	}
	switch result {
	case 1:
		return cacheQuotaOK, nil
	case 0:
		return cacheQuotaInsufficient, nil
	case -2:
		return cacheQuotaFenced, nil
	default:
		return cacheQuotaMiss, nil
	}
}

func cacheTryReserveUserQuota(userID int, amount int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), userQuotaReserveScript,
		[]string{getUserCacheKey(userID), getUserQuotaUncertaintyKey(userID), getTaskBillingUserQuotaFenceKey(userID)},
		amount, userID, userCacheSchemaVersion).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyUserQuotaDelta(userID int, delta int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), userQuotaDeltaScript,
		[]string{getUserCacheKey(userID), getUserQuotaUncertaintyKey(userID), getTaskBillingUserQuotaFenceKey(userID)},
		delta, userID, userCacheSchemaVersion).Int()
	return quotaResultFromLua(result, err)
}

func getUserQuotaUncertaintyKey(userID int) string {
	return fmt.Sprintf("user_quota_uncertain:%d", userID)
}

// fenceUserQuotaCacheUncertainty atomically blocks every cache-authoritative
// mutation and removes the possibly tentative balance. It is used only when a
// transaction callback completed but COMMIT's outcome cannot be read back.
func fenceUserQuotaCacheUncertainty(userID int, operation string, expectedOwners ...string) error {
	if !common.RedisEnabled {
		return nil
	}
	expectedOwner := ""
	if len(expectedOwners) > 0 {
		expectedOwner = expectedOwners[0]
	}
	const script = `
local existing = redis.call('GET', KEYS[2])
if existing and (ARGV[3] == '' or existing ~= ARGV[3]) then
  local ttl = redis.call('TTL', KEYS[2])
  if ttl > 0 and ttl < tonumber(ARGV[2]) then
    redis.call('EXPIRE', KEYS[2], ARGV[2])
  end
  redis.call('DEL', KEYS[1])
  return 2
end
redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[2])
redis.call('DEL', KEYS[1])
return 1`
	ttl := userCacheTTLSeconds() * 10
	if ttl < 300 {
		ttl = 300
	}
	if err := common.RDB.Eval(context.Background(), script,
		[]string{getUserCacheKey(userID), getUserQuotaUncertaintyKey(userID)}, operation, ttl, expectedOwner).Err(); err != nil {
		common.SysError(fmt.Sprintf("failed to fence uncertain user quota cache: user=%d operation=%s error=%v", userID, operation, err))
		return err
	}
	return nil
}

// fenceUserQuotaCacheUntilReconciled is reserved for a database COMMIT whose
// result is genuinely unknown. Absence of a journal row on one new connection
// cannot prove rollback, so this fence has no time-based automatic release.
func fenceUserQuotaCacheUntilReconciled(userID int, operation string, expectedOwners ...string) error {
	if !common.RedisEnabled {
		return nil
	}
	expectedOwner := operation
	if len(expectedOwners) > 0 {
		expectedOwner = expectedOwners[0]
	}
	const script = `
local existing = redis.call('GET', KEYS[2])
if existing and existing ~= ARGV[1] and existing ~= ARGV[2] then
  redis.call('DEL', KEYS[1])
  return -1
end
redis.call('SET', KEYS[2], ARGV[1])
redis.call('DEL', KEYS[1])
return 1`
	result, err := common.RDB.Eval(context.Background(), script,
		[]string{getUserCacheKey(userID), getUserQuotaUncertaintyKey(userID)}, operation, expectedOwner).Int()
	if err != nil || result != 1 {
		common.SysError(fmt.Sprintf("failed to persistently fence uncertain user quota cache: user=%d operation=%s error=%v",
			userID, operation, err))
		return fmt.Errorf("%w: user %d persistent quota fence ownership mismatch: result=%d: %v",
			ErrQuotaCacheUnavailable, userID, result, err)
	}
	return nil
}

// resolveUserQuotaCacheUncertainty elects one resolver, reloads the durable
// balance while all cache-authoritative mutations remain fenced, and only then
// releases the fence. Concurrent resolvers cannot overwrite a newer debit with
// an older database snapshot.
func resolveUserQuotaCacheUncertainty(userID int) error {
	if !common.RedisEnabled {
		return nil
	}
	if err := resolveTaskBillingUserQuotaFences(userID); err != nil {
		return err
	}
	fenceValue, err := common.RDB.Get(context.Background(), getUserQuotaUncertaintyKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, userID)
	}
	claimableCommitUnknown := ""
	const subscriptionCommitUnknownPrefix = "subscription_commit_unknown:"
	if strings.HasPrefix(fenceValue, subscriptionCommitUnknownPrefix) {
		tradeNo := strings.TrimPrefix(fenceValue, subscriptionCommitUnknownPrefix)
		committed, resolveErr := resolveSubscriptionBalancePurchaseCommitByTradeNo(tradeNo, userID)
		if resolveErr != nil || !committed {
			return fmt.Errorf("%w: user %d subscription commit is still uncertain", ErrQuotaCacheUnavailable, userID)
		}
		claimableCommitUnknown = fenceValue
	}
	const taskBillingCommitUnknownPrefix = "inflight:task-billing|"
	if strings.HasPrefix(fenceValue, taskBillingCommitUnknownPrefix) {
		committed, resolveErr := resolveTaskBillingUserQuotaFence(fenceValue, userID)
		if resolveErr != nil || !committed {
			return fmt.Errorf("%w: user %d task billing commit is still uncertain", ErrQuotaCacheUnavailable, userID)
		}
		claimableCommitUnknown = fenceValue
	}
	token := "resolving:" + common.GetUUID()
	const claimScript = `
local value = redis.call('GET', KEYS[1])
if not value then return 0 end
if string.sub(value, 1, 9) == 'inflight:' and value ~= ARGV[3] then return -2 end
if string.sub(value, 1, 10) == 'resolving:' then return -1 end
if string.sub(value, 1, 28) == 'subscription_commit_unknown:' and value ~= ARGV[3] then return -3 end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return 1`
	claimed, err := common.RDB.Eval(context.Background(), claimScript,
		[]string{getUserQuotaUncertaintyKey(userID)}, token, 300, claimableCommitUnknown).Int()
	if err != nil {
		return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, userID)
	}
	if claimed == 0 {
		return nil
	}
	if claimed == -2 {
		return fmt.Errorf("%w: user %d quota mutation in progress", ErrQuotaCacheUnavailable, userID)
	}
	if claimed != 1 {
		return fmt.Errorf("%w: user %d reconciliation in progress", ErrQuotaCacheUnavailable, userID)
	}

	restoreFence := func() {
		const script = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
end
return 1`
		ttl := userCacheTTLSeconds() * 10
		if ttl < 300 {
			ttl = 300
		}
		_ = common.RDB.Eval(context.Background(), script,
			[]string{getUserQuotaUncertaintyKey(userID)}, token, "uncertain", ttl).Err()
	}

	var user User
	if err := DB.Where("id = ?", userID).First(&user).Error; err != nil {
		restoreFence()
		return fmt.Errorf("%w: user %d: %v", ErrQuotaCacheUnavailable, userID, err)
	}
	if err := populateUserCacheUnderQuotaFence(user, token); err != nil {
		restoreFence()
		return fmt.Errorf("%w: user %d: %v", ErrQuotaCacheUnavailable, userID, err)
	}
	const releaseScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`
	released, err := common.RDB.Eval(context.Background(), releaseScript,
		[]string{getUserQuotaUncertaintyKey(userID)}, token).Int()
	if err != nil || released != 1 {
		return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, userID)
	}
	return nil
}

func cacheTryReserveTokenQuota(id int, key string, amount int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), tokenQuotaReserveScript,
		[]string{getTokenCacheKey(key), getTaskBillingTokenQuotaFenceKey(key)},
		amount, id, common.GetTimestamp()).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyTokenQuotaDelta(id int, key string, delta int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), tokenQuotaDeltaScript,
		[]string{getTokenCacheKey(key), getTaskBillingTokenQuotaFenceKey(key)},
		delta, id, common.GetTimestamp()).Int()
	return quotaResultFromLua(result, err)
}

// ensureUserQuotaCacheAvailable hydrates and validates the complete user hash
// before a business transaction starts. Callers that mutate cache while a DB
// transaction is open can then fail closed on a miss without performing a
// second database read through another connection.
func ensureUserQuotaCacheAvailable(id int) error {
	if !common.RedisEnabled {
		return nil
	}
	if err := resolveUserQuotaCacheUncertainty(id); err != nil {
		return err
	}
	result, err := cacheApplyUserQuotaDelta(id, 0)
	if err == nil && result == cacheQuotaOK {
		return nil
	}
	if err == nil && result == cacheQuotaMiss {
		if common.BatchUpdateEnabled && hasPendingBatchUpdate(BatchUpdateTypeUserQuota, id) {
			return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, id)
		}
		if _, hydrateErr := GetUserCache(id); hydrateErr != nil {
			return fmt.Errorf("%w: user %d: %v", ErrQuotaCacheUnavailable, id, hydrateErr)
		}
		result, err = cacheApplyUserQuotaDelta(id, 0)
	}
	if err != nil || result != cacheQuotaOK {
		return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, id)
	}
	return nil
}

// applyPreparedUserQuotaCacheDelta mutates an already-hydrated authoritative
// hash without falling back to the database. The boolean tells transaction
// callers whether a Redis compensation is required on rollback.
func applyPreparedUserQuotaCacheDelta(id int, delta int64) (bool, error) {
	if !common.RedisEnabled || delta == 0 {
		return false, nil
	}
	result, err := cacheApplyUserQuotaDelta(id, delta)
	if err != nil || result != cacheQuotaOK {
		return false, fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, id)
	}
	return true, nil
}

func compensatePreparedUserQuotaCacheDelta(id int, delta int64, operation string) {
	if !common.RedisEnabled || delta == 0 {
		return
	}
	result, err := cacheApplyUserQuotaDelta(id, -delta)
	if err != nil || result != cacheQuotaOK {
		common.SysError(fmt.Sprintf("failed to compensate %s user quota cache delta: user=%d delta=%d result=%d error=%v",
			operation, id, delta, result, err))
	}
}

// persistUserQuotaDelta durably stores a delta already applied to Redis.
func persistUserQuotaDelta(id int, delta int) error {
	// Spendable balances are never queued only in process memory. Redis may
	// expose the delta first, but the database write completes before success is
	// returned so a process crash cannot resurrect quota after cache expiry.
	return persistUserQuotaDeltaDirect(id, delta)
}

func persistUserQuotaDeltaDirect(id int, delta int) error {
	result := DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", delta))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func persistTokenQuotaDelta(id int, delta int) error {
	return persistTokenQuotaDeltaDirect(id, delta)
}

func persistTokenQuotaDeltaDirect(id int, delta int) error {
	result := DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", delta),
			"used_quota":    gorm.Expr("used_quota - ?", delta),
			"accessed_time": common.GetTimestamp(),
		},
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// applyUserQuotaDelta makes the cache delta visible first, then durably writes
// the same delta before returning. A failed database write synchronously
// compensates Redis.
func applyUserQuotaDelta(id int, delta int, forceDB bool) error {
	if delta == 0 {
		return nil
	}
	// db is retained on the public API for compatibility. Balance deltas are now
	// always durable before return, so both modes intentionally share this path.
	_ = forceDB
	persist := persistUserQuotaDeltaDirect
	if delta > 0 {
		// Credits/refunds are DB-first. Exposing a cache-only credit before its
		// durable write would allow spend from money that may never commit. Once
		// the DB succeeds, cache sync is only an availability optimization: failure
		// leaves a stale-low balance and must not make callers retry the credit.
		if err := persist(id, delta); err != nil {
			return err
		}
		if !common.RedisEnabled {
			return nil
		}
		result, cacheErr := cacheApplyUserQuotaDelta(id, int64(delta))
		if cacheErr == nil && result == cacheQuotaOK {
			return nil
		}
		common.SysLog(fmt.Sprintf("failed to sync committed user quota credit: user=%d delta=%d result=%d error=%v", id, delta, result, cacheErr))
		if fenceErr := fenceUserQuotaCacheUncertainty(id, "user_quota_credit_sync_error"); fenceErr != nil {
			common.SysError(fmt.Sprintf("failed to fence user quota after credit cache sync error: user=%d error=%v", id, fenceErr))
		}
		return nil
	}

	if !common.RedisEnabled {
		return persist(id, delta)
	}
	if err := resolveUserQuotaCacheUncertainty(id); err != nil {
		return err
	}

	result, err := cacheApplyUserQuotaDelta(id, int64(delta))
	if err == nil && result == cacheQuotaMiss {
		if common.BatchUpdateEnabled && hasPendingBatchUpdate(BatchUpdateTypeUserQuota, id) {
			return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, id)
		}
		if _, hydrateErr := GetUserCache(id); hydrateErr == nil {
			result, err = cacheApplyUserQuotaDelta(id, int64(delta))
		} else {
			err = hydrateErr
		}
	}
	if err != nil || result != cacheQuotaOK {
		// Once Redis is enabled its live hash may survive an outage. Writing only
		// the database here would let that stale hash authorize the old balance
		// when Redis recovers, so every cache-authoritative mutation fails closed.
		return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, id)
	}

	if err := persist(id, delta); err != nil {
		// The driver may report an error after the statement committed. Restoring
		// a debit in Redis would then expose the old higher balance. Fence and
		// discard the mirror; the next mutation rehydrates the actual DB outcome.
		_ = fenceUserQuotaCacheUncertainty(id, "user_quota_persist_error")
		return err
	}
	return nil
}

// applyTokenQuotaDelta is the token counterpart of applyUserQuotaDelta. It
// updates both RemainQuota and UsedQuota atomically in the live token hash.
func applyTokenQuotaDelta(id int, key string, delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		if err := persistTokenQuotaDeltaDirect(id, delta); err != nil {
			return err
		}
		if !common.RedisEnabled {
			return nil
		}
		result, cacheErr := cacheApplyTokenQuotaDelta(id, key, int64(delta))
		if cacheErr == nil && result == cacheQuotaOK {
			return nil
		}
		common.SysLog(fmt.Sprintf("failed to sync committed token quota credit: token=%d delta=%d result=%d error=%v", id, delta, result, cacheErr))
		if fenceErr := invalidateTokenCacheForMutation(key); fenceErr != nil {
			common.SysError(fmt.Sprintf("failed to fence token after credit cache sync error: token=%d error=%v", id, fenceErr))
		}
		return nil
	}
	if !common.RedisEnabled {
		return persistTokenQuotaDeltaDirect(id, delta)
	}
	if err := resolveTaskBillingTokenQuotaFences(key, id); err != nil {
		return err
	}

	result, err := cacheApplyTokenQuotaDelta(id, key, int64(delta))
	if err == nil && result == cacheQuotaMiss {
		if common.BatchUpdateEnabled && hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, id) {
			return fmt.Errorf("%w: token %d", ErrQuotaCacheUnavailable, id)
		}
		if _, hydrateErr := GetTokenByKey(key, true); hydrateErr == nil {
			result, err = cacheApplyTokenQuotaDelta(id, key, int64(delta))
		} else {
			err = hydrateErr
		}
	}
	if err != nil || result != cacheQuotaOK {
		return fmt.Errorf("%w: token %d", ErrQuotaCacheUnavailable, id)
	}

	if err := persistTokenQuotaDelta(id, delta); err != nil {
		if fenceErr := invalidateTokenCacheForMutation(key); fenceErr != nil {
			common.SysError(fmt.Sprintf("failed to fence token quota after uncertain debit: token=%d error=%v", id, fenceErr))
		}
		return err
	}
	return nil
}

func reserveUserQuotaDB(id int, quota int) (bool, error) {
	result := DB.Model(&User{}).
		Where("id = ? AND quota >= ?", id, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	return result.RowsAffected == 1, result.Error
}

func reserveTokenQuotaDB(id int, quota int) (bool, error) {
	result := DB.Model(&Token{}).
		Where("id = ? AND remain_quota >= ?", id, quota).
		Updates(map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		})
	return result.RowsAffected == 1, result.Error
}

// TryReserveUserQuota atomically checks and deducts a user's wallet quota.
// Redis is authoritative whenever enabled; failures fail closed because a
// surviving stale hash must never be bypassed by a database-only debit.
func TryReserveUserQuota(id int, quota int) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return true, nil
	}
	if !common.RedisEnabled {
		return reserveUserQuotaDB(id, quota)
	}
	if err := resolveUserQuotaCacheUncertainty(id); err != nil {
		return false, err
	}

	result, err := cacheTryReserveUserQuota(id, int64(quota))
	if err == nil && result == cacheQuotaMiss {
		if common.BatchUpdateEnabled && hasPendingBatchUpdate(BatchUpdateTypeUserQuota, id) {
			return false, ErrQuotaCacheUnavailable
		}
		if _, hydrateErr := GetUserCache(id); hydrateErr == nil {
			result, err = cacheTryReserveUserQuota(id, int64(quota))
		}
	}
	if err != nil || result == cacheQuotaMiss || result == cacheQuotaFenced {
		// A Redis error does not prove that the old hash disappeared. Database
		// fallback would debit only the row while the surviving hash can spend
		// the pre-outage balance after recovery.
		return false, fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, id)
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistUserQuotaDelta(id, -quota); err != nil {
		_ = fenceUserQuotaCacheUncertainty(id, "user_quota_reserve_persist_error")
		return false, err
	}
	return true, nil
}

// TryReserveTokenQuota atomically checks and deducts a token quota. Unlimited
// tokens skip the balance check but still update remain/used accounting.
func TryReserveTokenQuota(id int, key string, quota int, unlimited bool) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return true, nil
	}
	if unlimited {
		return true, DecreaseTokenQuota(id, key, quota)
	}
	if !common.RedisEnabled {
		return reserveTokenQuotaDB(id, quota)
	}
	if err := resolveTaskBillingTokenQuotaFences(key, id); err != nil {
		return false, err
	}

	result, err := cacheTryReserveTokenQuota(id, key, int64(quota))
	if err == nil && result == cacheQuotaMiss {
		if common.BatchUpdateEnabled && hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, id) {
			return false, ErrQuotaCacheUnavailable
		}
		if _, hydrateErr := GetTokenByKey(key, true); hydrateErr == nil {
			result, err = cacheTryReserveTokenQuota(id, key, int64(quota))
		}
	}
	if err != nil || result == cacheQuotaMiss || result == cacheQuotaFenced {
		return false, fmt.Errorf("%w: token %d", ErrQuotaCacheUnavailable, id)
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistTokenQuotaDelta(id, -quota); err != nil {
		if fenceErr := invalidateTokenCacheForMutation(key); fenceErr != nil {
			common.SysError(fmt.Sprintf("failed to fence token quota after uncertain reserve: token=%d error=%v", id, fenceErr))
		}
		return false, err
	}
	return true, nil
}
