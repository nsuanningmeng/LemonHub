package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex
var batchUncertainStores []map[int]int

// Access is serialized by batchFlushGate. An uncertain commit is never replayed.
var batchUncertainError error

// Serialize periodic and final flushes; waiting for the gate is cancellable.
var batchFlushGate = make(chan struct{}, 1)
var batchUpdaterMu sync.Mutex
var batchUpdaterCancel context.CancelFunc
var batchUpdaterDone chan struct{}

func init() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUncertainStores = append(batchUncertainStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	batchUpdaterMu.Lock()
	defer batchUpdaterMu.Unlock()
	if batchUpdaterDone != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	batchUpdaterCancel = cancel
	batchUpdaterDone = make(chan struct{})
	done := batchUpdaterDone
	interval := time.Duration(common.BatchUpdateInterval) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				// Stopping the ticker must not cancel an additive SQL write that
				// may already have committed. Wait for it instead; a shutdown
				// timeout is reported without starting a concurrent final flush.
				if err := FlushBatchUpdates(context.Background()); err != nil {
					common.SysError("batch update failed: " + err.Error())
				}
			}
		}
	}()
}

// StopBatchUpdater stops periodic writes and waits for an in-flight flush to
// finish or requeue its failed records. Stop accounting producers first, then
// call FlushBatchUpdates with a live shutdown context to persist the tail.
func StopBatchUpdater(ctx context.Context) error {
	batchUpdaterMu.Lock()
	cancel, done := batchUpdaterCancel, batchUpdaterDone
	if cancel != nil {
		cancel()
	}
	batchUpdaterMu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func addNewRecord(type_ int, id int, value int) {
	if value == 0 {
		return
	}
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	if _, ok := batchUpdateStores[type_][id]; !ok {
		batchUpdateStores[type_][id] = value
	} else {
		batchUpdateStores[type_][id] += value
	}
}

// hasPendingBatchUpdate reports whether this process still has an unapplied
// balance delta for one record. A Redis cache miss must not be hydrated from
// the database while such a delta exists: in batch mode that database row is
// intentionally stale and would make the re-created cache authorize quota a
// second time.
func hasPendingBatchUpdate(type_ int, id int) bool {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	return batchUpdateStores[type_][id] != 0 || batchUncertainStores[type_][id] != 0
}

func batchUpdate() {
	if err := FlushBatchUpdates(context.Background()); err != nil {
		common.SysError("batch update failed: " + err.Error())
	}
}

// FlushBatchUpdates commits one atomic snapshot. Only a confirmed rollback can
// put deltas back in the retry queue. An uncertain commit/rollback is isolated
// for reconciliation and blocks automatic replay. Producers must be stopped
// before this function can be used as the final shutdown flush.
func FlushBatchUpdates(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case batchFlushGate <- struct{}{}:
		defer func() { <-batchFlushGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if batchUncertainError != nil {
		return batchUncertainError
	}
	// check if there's any data to update
	hasData := false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}

	if !hasData {
		return nil
	}

	common.SysLog("batch update started")
	stores := make([]map[int]int, BatchUpdateTypeCount)
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		stores[i] = batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}

	batchID := common.GetUUID()
	// Explicit transaction ownership makes statement failure distinguishable
	// from commit uncertainty. Inner updates must not commit independently.
	tx := DB.WithContext(ctx).Session(&gorm.Session{SkipDefaultTransaction: true}).Begin()
	if tx.Error != nil {
		requeueBatchUpdates(stores)
		return fmt.Errorf("batch %s begin: %w", batchID, tx.Error)
	}
	if err := applyBatchUpdates(tx, stores); err != nil {
		if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
			return quarantineBatchUpdates(batchID, stores, errors.Join(err, fmt.Errorf("rollback outcome unknown: %w", rollbackErr)))
		}
		requeueBatchUpdates(stores)
		return fmt.Errorf("batch %s rolled back: %w", batchID, err)
	}
	if err := tx.Commit().Error; err != nil {
		return quarantineBatchUpdates(batchID, stores, fmt.Errorf("commit outcome unknown: %w", err))
	}
	common.SysLog("batch update finished: " + batchID)
	return nil
}

func requeueBatchUpdates(stores []map[int]int) {
	for kind, store := range stores {
		for id, delta := range store {
			addNewRecord(kind, id, delta)
		}
	}
}

func quarantineBatchUpdates(batchID string, stores []map[int]int, err error) error {
	for kind, store := range stores {
		batchUpdateLocks[kind].Lock()
		batchUncertainStores[kind] = store
		batchUpdateLocks[kind].Unlock()
	}
	batchUncertainError = fmt.Errorf("batch %s requires reconciliation; automatic replay disabled: %w", batchID, err)
	// Keep the exact deltas in the operator log as well as in memory. This is
	// diagnostic evidence, not a durable exactly-once recovery journal.
	deltas, _ := common.Marshal(map[string]any{
		"user_quota":         stores[BatchUpdateTypeUserQuota],
		"token_quota":        stores[BatchUpdateTypeTokenQuota],
		"user_used_quota":    stores[BatchUpdateTypeUsedQuota],
		"channel_used_quota": stores[BatchUpdateTypeChannelUsedQuota],
		"request_count":      stores[BatchUpdateTypeRequestCount],
	})
	common.SysError(fmt.Sprintf("%v; pending_deltas=%s", batchUncertainError, deltas))
	return batchUncertainError
}

func applyBatchUpdates(tx *gorm.DB, stores []map[int]int) error {
	for id, delta := range stores[BatchUpdateTypeTokenQuota] {
		if err := increaseTokenQuota(tx, id, delta); err != nil {
			return fmt.Errorf("token %d quota: %w", id, err)
		}
	}
	for id, delta := range stores[BatchUpdateTypeChannelUsedQuota] {
		if err := updateChannelUsedQuota(tx, id, delta); err != nil {
			return fmt.Errorf("channel %d usage: %w", id, err)
		}
	}
	userQuotaStore := stores[BatchUpdateTypeUserQuota]
	usedQuotaStore := stores[BatchUpdateTypeUsedQuota]
	requestCountStore := stores[BatchUpdateTypeRequestCount]
	userIDs := make(map[int]struct{}, len(userQuotaStore)+len(usedQuotaStore)+len(requestCountStore))
	for id := range userQuotaStore {
		userIDs[id] = struct{}{}
	}
	for id := range usedQuotaStore {
		userIDs[id] = struct{}{}
	}
	for id := range requestCountStore {
		userIDs[id] = struct{}{}
	}
	for id := range userIDs {
		if err := updateUserQuotaUsedQuotaAndRequestCount(tx, id, userQuotaStore[id], usedQuotaStore[id], requestCountStore[id]); err != nil {
			return fmt.Errorf("user %d accounting: %w", id, err)
		}
	}
	return nil
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
