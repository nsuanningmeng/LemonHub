package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func getTokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func getTokenCacheFenceKey(key string) string {
	return fmt.Sprintf("token:fence:%s", common.GenerateHMAC(key))
}

func tokenCacheTTLSeconds() int {
	ttl := common.RedisKeyCacheSeconds()
	if ttl <= 0 {
		return 60
	}
	return ttl
}

// tokenCacheFenceSeconds must outlive a token mutation's database write plus
// any in-flight reader's DB-read-to-cache-init gap. The fence is not deleted
// after commit; it expires naturally so a reader holding a pre-mutation
// snapshot cannot publish it right after the mutation cleared the cache.
// While the fence exists readers simply serve the database without caching.
const tokenCacheFenceSeconds = 10

// invalidateTokenCacheForMutation is called before a token metadata mutation
// writes to the database: it raises the fence and drops the cached hash so no
// reader can act on (or re-publish) the pre-mutation state.
func invalidateTokenCacheForMutation(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	if err := resolveTaskBillingTokenQuotaFences(key, 0); err != nil {
		return err
	}
	ctx := context.Background()
	err := common.RDB.Set(ctx, getTokenCacheFenceKey(key), 1, time.Duration(tokenCacheFenceSeconds)*time.Second).Err()
	if err != nil {
		return err
	}
	return common.RDB.Del(ctx, getTokenCacheKey(key)).Err()
}

// cacheInitToken publishes a database snapshot only when no mutation fence is
// active and the hash is cold. An existing hash only gets its TTL refreshed:
// its RemainQuota may already be ahead of this snapshot because atomic
// pre-consume decrements Redis first, so a snapshot must never overwrite any
// field of a live hash.
// 返回值：0=被 fence 拦截，1=完成初始化，2=哈希已存在，仅刷新 TTL。
func cacheInitToken(token Token) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	const script = `
if redis.call('EXISTS', KEYS[2]) == 1 or redis.call('EXISTS', KEYS[3]) == 1 then
  return 0
end
if redis.call('EXISTS', KEYS[1]) == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[17])
  return 2
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'AccessedTime', ARGV[6], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'RemainQuota', ARGV[15], 'UsedQuota', ARGV[16])
redis.call('EXPIRE', KEYS[1], ARGV[17])
return 1`

	return common.RDB.Eval(context.Background(), script, []string{
		getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key), getTaskBillingTokenQuotaFenceKey(token.Key),
	},
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota,
		tokenCacheTTLSeconds(),
	).Int()
}

// populateTokenCacheAfterTaskBillingFences publishes one fresh database
// snapshot and releases exactly the task-billing attempts that authorized it.
// The membership check, hash write, and set deletion are one Redis operation,
// so a concurrently arriving unknown attempt either wins before this script
// (and blocks it) or wins afterwards (and removes the just-written hash).
func populateTokenCacheAfterTaskBillingFences(token Token, fenceValues []string) (bool, error) {
	return populateTokenCacheWithTaskBillingFences(token, fenceValues, true)
}

func populateTokenCacheWhileTaskBillingFences(token Token, fenceValues []string) (bool, error) {
	return populateTokenCacheWithTaskBillingFences(token, fenceValues, false)
}

func populateTokenCacheWithTaskBillingFences(token Token, fenceValues []string, releaseTaskFences bool) (bool, error) {
	if !common.RedisEnabled || token.Key == "" || len(fenceValues) == 0 {
		return false, nil
	}
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	const script = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return -1
end
if redis.call('SCARD', KEYS[3]) ~= tonumber(ARGV[18]) then
  return 0
end
for i = 1, tonumber(ARGV[18]) do
  if redis.call('SISMEMBER', KEYS[3], ARGV[19 + i]) == 0 then
    return 0
  end
end
if ARGV[19] == '0'
  and redis.call('EXISTS', KEYS[1]) == 1
  and redis.call('HEXISTS', KEYS[1], 'Id') == 1
  and redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 1
  and redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[17])
  return 2
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'AccessedTime', ARGV[6], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'RemainQuota', ARGV[15], 'UsedQuota', ARGV[16])
redis.call('EXPIRE', KEYS[1], ARGV[17])
if ARGV[19] == '1' then
  redis.call('DEL', KEYS[3])
end
return 1`
	releaseTaskFencesArg := "0"
	if releaseTaskFences {
		releaseTaskFencesArg = "1"
	}
	args := []interface{}{
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota,
		tokenCacheTTLSeconds(), len(fenceValues), releaseTaskFencesArg,
	}
	for _, fenceValue := range fenceValues {
		args = append(args, fenceValue)
	}
	result, err := common.RDB.Eval(context.Background(), script, []string{
		getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key), getTaskBillingTokenQuotaFenceKey(token.Key),
	}, args...).Int()
	if err != nil {
		return false, err
	}
	if result != 1 && result != 2 {
		return false, fmt.Errorf("%w: token %d task billing reconciliation changed", ErrQuotaCacheUnavailable, token.Id)
	}
	return true, nil
}

// cacheGetTokenByKey 从缓存读取 token；不完整的哈希（如仅有配额字段）会被拒绝。
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	if err := failIfTaskBillingTokenQuotaFenced(key); err != nil {
		return nil, err
	}
	var token Token
	if err := common.RedisHGetObj(getTokenCacheKey(key), &token); err != nil {
		return nil, err
	}
	if err := failIfTaskBillingTokenQuotaFenced(key); err != nil {
		return nil, err
	}
	if token.Id <= 0 {
		return nil, fmt.Errorf("token cache is incomplete")
	}
	token.Key = key
	return &token, nil
}

func failIfTaskBillingTokenQuotaFenced(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	fenced, err := common.RDB.Exists(context.Background(), getTaskBillingTokenQuotaFenceKey(key)).Result()
	if err != nil {
		return fmt.Errorf("%w: token quota fence read failed", ErrQuotaCacheUnavailable)
	}
	if fenced != 0 {
		return fmt.Errorf("%w: token quota reconciliation is pending", ErrQuotaCacheUnavailable)
	}
	return nil
}
