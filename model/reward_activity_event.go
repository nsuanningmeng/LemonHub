package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// RewardActivityEvent is a short-lived, durable source queue. SQL source writes
// and their event commit together; ClickHouse writes are preceded by a main-DB
// intent which can be confirmed by its exact source identity after a restart.
// IDs only identify rows: consumers must never use a maximum-ID checkpoint,
// because concurrent source transactions can commit in a different order.
type RewardActivityEvent struct {
	Id            int64  `gorm:"primaryKey;index:idx_reward_activity_ready,priority:3"`
	Kind          string `gorm:"type:varchar(16);not null"`
	UserID        int    `gorm:"not null"`
	OccurredAt    int64  `gorm:"type:bigint;not null"`
	SourceID      int64  `gorm:"type:bigint;not null"`
	RequestID     string `gorm:"type:varchar(64);not null"`
	Confirmed     bool   `gorm:"not null;index:idx_reward_activity_ready,priority:1"`
	NextAttemptAt int64  `gorm:"type:bigint;not null;index:idx_reward_activity_ready,priority:2"`
	CreatedAt     int64  `gorm:"type:bigint;not null"`
}

type PendingRewardActivityEvent struct {
	Event  RewardActivityEvent
	Source string
}

const (
	RewardActivitySourceMain = "main"
	RewardActivitySourceLog  = "log"
	rewardActivityRetryDelay = int64(30)
)

// Registration is explicit so isolated legacy model tests need not migrate
// unrelated infrastructure. Every production node MUST call
// InitializeRewardActivityCapture after both databases have been initialized.
// No schema inspection or campaign-cache lookup is performed on the hot path.
var rewardActivityStores = struct {
	sync.RWMutex
	ready map[*sql.DB]struct{}
}{ready: make(map[*sql.DB]struct{})}

var rewardActivitySingleRowTurn atomic.Uint32

func registerRewardActivityStore(db *gorm.DB) error {
	if db == nil {
		return errors.New("reward activity database is not initialized")
	}
	connection, err := db.DB()
	if err != nil {
		return err
	}
	rewardActivityStores.Lock()
	rewardActivityStores.ready[connection] = struct{}{}
	rewardActivityStores.Unlock()
	return nil
}

func rewardActivityCaptureEnabled(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	connection, err := db.DB()
	if err != nil {
		return false
	}
	rewardActivityStores.RLock()
	_, ready := rewardActivityStores.ready[connection]
	rewardActivityStores.RUnlock()
	return ready
}

// MigrateRewardActivityEvents only creates the small source queue. It does not
// alter the existing logs or top_ups table, their indexes, or historical rows.
func MigrateRewardActivityEvents(db *gorm.DB) error {
	if db == nil {
		return errors.New("reward activity database is not initialized")
	}
	if err := db.AutoMigrate(&RewardActivityEvent{}); err != nil {
		return err
	}
	return registerRewardActivityStore(db)
}

// InitializeRewardActivityCapture runs on master and non-master nodes alike.
// Missing source tables are a startup error, not permission to silently omit
// events. The caller must fail startup if initialization fails.
func InitializeRewardActivityCapture() error {
	stores := []*gorm.DB{DB}
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		stores = append(stores, LOG_DB)
	}
	for _, db := range stores {
		if db == nil || !db.Migrator().HasTable(&RewardActivityEvent{}) {
			return errors.New("reward activity source table is missing; run database migrations before starting this node")
		}
	}
	for _, db := range stores {
		if err := registerRewardActivityStore(db); err != nil {
			return fmt.Errorf("initialize reward activity capture: %w", err)
		}
	}
	return nil
}

func recordRewardTopUpActivity(tx *gorm.DB, topUp *TopUp) error {
	if topUp == nil || topUp.SiteId != 0 || topUp.UserId <= 0 ||
		topUp.Status != common.TopUpStatusSuccess || topUp.ClawedBackQuota != 0 ||
		!rewardActivityCaptureEnabled(tx) {
		return nil
	}
	return tx.Create(&RewardActivityEvent{
		Kind: "topup", UserID: topUp.UserId, OccurredAt: topUp.CompleteTime,
		SourceID: int64(topUp.Id), Confirmed: true, CreatedAt: common.GetTimestamp(),
	}).Error
}

func createLogWithRewardActivity(log *Log) error {
	if log.Type != LogTypeConsume || log.SiteId != 0 || log.UserId <= 0 {
		return LOG_DB.Create(log).Error
	}
	store := LOG_DB
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		store = DB
	}
	if !rewardActivityCaptureEnabled(store) {
		return LOG_DB.Create(log).Error
	}
	event := RewardActivityEvent{
		Kind: "usage", UserID: log.UserId, OccurredAt: log.CreatedAt,
		RequestID: log.RequestId, CreatedAt: common.GetTimestamp(),
	}
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return LOG_DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(log).Error; err != nil {
				return err
			}
			event.SourceID = int64(log.Id)
			event.Confirmed = true
			return tx.Create(&event).Error
		})
	}
	// Do not write an untracked ClickHouse log when its durable intent fails.
	// A log failure or an uncertain response leaves the intent for reconciliation.
	if err := DB.Create(&event).Error; err != nil {
		return err
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		return err
	}
	return DB.Model(&RewardActivityEvent{}).Where("id = ?", event.Id).
		Update("confirmed", true).Error
}

// confirmRewardActivityIntents queries the original timestamp AND request ID.
// ClickHouse orders logs by (created_at, request_id); a request-ID-only lookup
// would scan unrelated history. Missing/uncertain writes remain durable and are
// retried fairly, without a time-based deletion or a maximum-ID watermark.
func confirmRewardActivityIntents(ctx context.Context, limit int, now int64) error {
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil
	}
	var intents []RewardActivityEvent
	if err := DB.WithContext(ctx).Where("confirmed = ? AND next_attempt_at <= ?", false, now).
		Order("next_attempt_at, id").Limit(limit).Find(&intents).Error; err != nil {
		return err
	}
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return err
		}
		var source struct{ RequestId string }
		err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("request_id").
			Where("created_at = ? AND request_id = ? AND user_id = ? AND site_id = ? AND type = ?",
				intent.OccurredAt, intent.RequestID, intent.UserID, 0, LogTypeConsume).
			Take(&source).Error
		updates := map[string]interface{}{"next_attempt_at": now + rewardActivityRetryDelay}
		if err == nil {
			updates["confirmed"] = true
			updates["next_attempt_at"] = int64(0)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			// A log-store outage must not prevent ready main-store top-ups from
			// being delivered. Persist the retry and make the failure observable.
			common.SysError("reward activity source confirmation failed: " + err.Error())
		}
		if err := DB.WithContext(ctx).Model(&RewardActivityEvent{}).
			Where("id = ? AND confirmed = ?", intent.Id, false).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListPendingRewardActivityEvents returns at most limit confirmed source rows.
// The caller commits all main-DB effects before acknowledging each delivery.
// Concurrent readers may receive the same event; delivery is at-least-once.
func ListPendingRewardActivityEvents(ctx context.Context, limit int) ([]PendingRewardActivityEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	now := common.GetTimestamp()
	if err := confirmRewardActivityIntents(ctx, limit, now); err != nil {
		return nil, err
	}
	stores := []struct {
		name string
		db   *gorm.DB
	}{{RewardActivitySourceMain, DB}}
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		mainConnection, mainErr := DB.DB()
		logConnection, logErr := LOG_DB.DB()
		if mainErr != nil || logErr != nil {
			return nil, errors.Join(mainErr, logErr)
		}
		if mainConnection != logConnection {
			stores = append(stores, struct {
				name string
				db   *gorm.DB
			}{RewardActivitySourceLog, LOG_DB})
		}
	}
	if limit == 1 && len(stores) == 2 && rewardActivitySingleRowTurn.Add(1)%2 == 0 {
		stores[0], stores[1] = stores[1], stores[0]
	}
	result := make([]PendingRewardActivityEvent, 0, limit)
	for index, store := range stores {
		// Both busy sources get a share. Unused capacity from an earlier source
		// is available to the next one, without starving logs behind top-ups.
		batchSize := (limit - len(result) + len(stores) - index - 1) / (len(stores) - index)
		if batchSize == 0 {
			break
		}
		var events []RewardActivityEvent
		if err := store.db.WithContext(ctx).Where("confirmed = ? AND next_attempt_at <= ?", true, now).
			Order("next_attempt_at, id").Limit(batchSize).Find(&events).Error; err != nil {
			return nil, err
		}
		for _, event := range events {
			result = append(result, PendingRewardActivityEvent{Event: event, Source: store.name})
		}
	}
	return result, nil
}

func AckRewardActivityEvent(ctx context.Context, item PendingRewardActivityEvent) error {
	var db *gorm.DB
	switch item.Source {
	case RewardActivitySourceMain:
		db = DB
	case RewardActivitySourceLog:
		if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			return errors.New("ClickHouse reward activity intents are stored in the main database")
		}
		db = LOG_DB
	default:
		return errors.New("invalid reward activity source")
	}
	if item.Event.Id <= 0 || !item.Event.Confirmed {
		return errors.New("invalid reward activity acknowledgement")
	}
	return db.WithContext(ctx).Where("id = ? AND kind = ? AND user_id = ? AND occurred_at = ? AND confirmed = ?",
		item.Event.Id, item.Event.Kind, item.Event.UserID, item.Event.OccurredAt, true).
		Delete(&RewardActivityEvent{}).Error
}
