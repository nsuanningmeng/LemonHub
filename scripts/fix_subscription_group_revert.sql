-- ============================================================================
-- Remediation: subscription-expiry group revert (existing stuck users)
-- ============================================================================
--
-- WHY THIS IS NEEDED
--   The expiry-revert code fix (model/subscription.go) only fires when a
--   subscription transitions active -> expired. Users whose subscription ALREADY
--   expired before the fix was deployed are stuck in the upgraded group forever:
--   there is no future expiry event left to trigger the corrected logic. This
--   script reverts them once, using the same chain-aware rule as the code fix:
--   recover the EARLIEST recorded original group (prev_user_group) for the group
--   the user is currently stuck in.
--
-- WHO IT TOUCHES (intentionally conservative — mirrors the code guards)
--   A user is corrected only if ALL hold:
--     1. their current group equals the upgrade_group of some EXPIRED subscription
--        (so manually-assigned groups are never touched),
--     2. they have NO ACTIVE subscription still granting that group,
--     3. a recoverable baseline exists (some sub recorded a non-empty
--        prev_user_group different from the current group).
--   Users with no recoverable baseline (e.g. placed into the group manually
--   before ever subscribing) are deliberately left untouched for manual review.
--
-- HOW TO RUN
--   1. BACK UP the database first.
--   2. Run the SELECT (preview) block to review exactly who will change.
--   3. Pick the UPDATE block matching your database and run it in a transaction.
--   This script is idempotent: re-running it changes nothing once users are fixed.
--
-- IMPORTANT: `group` is a reserved word. PostgreSQL quotes it with "group";
-- MySQL/SQLite quote it with `group`. Use the block matching your DB.
-- ============================================================================


-- ----------------------------------------------------------------------------
-- 1) PREVIEW (PostgreSQL) — review before changing anything
-- ----------------------------------------------------------------------------
SELECT u.id,
       u."group"                                   AS current_group,
       (SELECT r.prev_user_group
          FROM user_subscriptions r
         WHERE r.user_id = u.id
           AND r.upgrade_group = u."group"
           AND r.prev_user_group <> ''
           AND r.prev_user_group <> u."group"
         ORDER BY r.start_time ASC, r.id ASC
         LIMIT 1)                                   AS will_revert_to
  FROM users u
 WHERE EXISTS (SELECT 1 FROM user_subscriptions e
                WHERE e.user_id = u.id AND e.status = 'expired' AND e.upgrade_group = u."group")
   AND NOT EXISTS (SELECT 1 FROM user_subscriptions a
                    WHERE a.user_id = u.id AND a.status = 'active' AND a.upgrade_group = u."group")
   AND EXISTS (SELECT 1 FROM user_subscriptions r
                WHERE r.user_id = u.id AND r.upgrade_group = u."group"
                  AND r.prev_user_group <> '' AND r.prev_user_group <> u."group");


-- ----------------------------------------------------------------------------
-- 2a) FIX (PostgreSQL)
-- ----------------------------------------------------------------------------
BEGIN;
UPDATE users u
   SET "group" = (SELECT r.prev_user_group
                    FROM user_subscriptions r
                   WHERE r.user_id = u.id
                     AND r.upgrade_group = u."group"
                     AND r.prev_user_group <> ''
                     AND r.prev_user_group <> u."group"
                   ORDER BY r.start_time ASC, r.id ASC
                   LIMIT 1)
 WHERE EXISTS (SELECT 1 FROM user_subscriptions e
                WHERE e.user_id = u.id AND e.status = 'expired' AND e.upgrade_group = u."group")
   AND NOT EXISTS (SELECT 1 FROM user_subscriptions a
                    WHERE a.user_id = u.id AND a.status = 'active' AND a.upgrade_group = u."group")
   AND EXISTS (SELECT 1 FROM user_subscriptions r
                WHERE r.user_id = u.id AND r.upgrade_group = u."group"
                  AND r.prev_user_group <> '' AND r.prev_user_group <> u."group");
COMMIT;


-- ----------------------------------------------------------------------------
-- 2b) FIX (MySQL >= 5.7 / SQLite) — same logic, backtick-quoted `group`.
--     Correlated subquery references user_subscriptions (a different table from
--     the UPDATE target), which is permitted on all three databases.
-- ----------------------------------------------------------------------------
-- MySQL: wrap with START TRANSACTION; ... COMMIT;
-- SQLite: wrap with BEGIN; ... COMMIT;
UPDATE users
   SET `group` = (SELECT r.prev_user_group
                    FROM user_subscriptions r
                   WHERE r.user_id = users.id
                     AND r.upgrade_group = users.`group`
                     AND r.prev_user_group <> ''
                     AND r.prev_user_group <> users.`group`
                   ORDER BY r.start_time ASC, r.id ASC
                   LIMIT 1)
 WHERE EXISTS (SELECT 1 FROM user_subscriptions e
                WHERE e.user_id = users.id AND e.status = 'expired' AND e.upgrade_group = users.`group`)
   AND NOT EXISTS (SELECT 1 FROM user_subscriptions a
                    WHERE a.user_id = users.id AND a.status = 'active' AND a.upgrade_group = users.`group`)
   AND EXISTS (SELECT 1 FROM user_subscriptions r
                WHERE r.user_id = users.id AND r.upgrade_group = users.`group`
                  AND r.prev_user_group <> '' AND r.prev_user_group <> users.`group`);


-- ----------------------------------------------------------------------------
-- 3) NOTE on Redis cache
--   If Redis is enabled, also clear the affected users' cache so the new group
--   is read immediately (the code fix to common.RedisHSetField handles this for
--   live reverts going forward). Either let the cache expire, or delete the keys:
--     redis-cli --scan --pattern 'user:cache:*' | xargs redis-cli del   (adjust prefix)
--   The exact key prefix is defined by getUserCacheKey in model/user_cache.go.
-- ============================================================================
