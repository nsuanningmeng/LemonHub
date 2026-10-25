package model

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRewardActivityEventTestDB(t *testing.T) {
	t.Helper()
	setupRewardTestDB(t)
	require.NoError(t, MigrateRewardActivityEvents(DB))
	require.NoError(t, MigrateRewardActivityEvents(LOG_DB))
	mainConnection, err := DB.DB()
	require.NoError(t, err)
	logConnection, err := LOG_DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		rewardActivityStores.Lock()
		delete(rewardActivityStores.ready, mainConnection)
		delete(rewardActivityStores.ready, logConnection)
		rewardActivityStores.Unlock()
	})
}

func TestRewardActivitySQLLogCommitsOneConfirmedEventWithOriginalIdentity(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	log := &Log{UserId: 17, SiteId: 0, CreatedAt: 123, Type: LogTypeConsume, RequestId: "source-usage"}
	require.NoError(t, createLog(log))
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, RewardActivitySourceLog, rows[0].Source)
	assert.Equal(t, "usage", rows[0].Event.Kind)
	assert.Equal(t, 17, rows[0].Event.UserID)
	assert.Equal(t, int64(123), rows[0].Event.OccurredAt)
	assert.Equal(t, int64(log.Id), rows[0].Event.SourceID)
	assert.Equal(t, "source-usage", rows[0].Event.RequestID)
	assert.True(t, rows[0].Event.Confirmed)
	require.NoError(t, AckRewardActivityEvent(context.Background(), rows[0]))
	require.NoError(t, AckRewardActivityEvent(context.Background(), rows[0]))
	rows, err = ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	var preserved Log
	require.NoError(t, LOG_DB.First(&preserved, log.Id).Error)
}

func TestRewardActivitySQLLogRollsBackWhenEventWriteFails(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	const callback = "test:reward-activity-log-event-failure"
	require.NoError(t, LOG_DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "RewardActivityEvent" {
			tx.AddError(errors.New("source event unavailable"))
		}
	}))
	t.Cleanup(func() { _ = LOG_DB.Callback().Create().Remove(callback) })
	err := createLog(&Log{UserId: 17, SiteId: 0, CreatedAt: 123, Type: LogTypeConsume})
	require.ErrorContains(t, err, "source event unavailable")
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRewardActivityTopUpCompletionIsAtomicAndDuplicateCompletionDoesNotRepeatEvent(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	user := createReserveTestUser(t, 0)
	order := &TopUp{UserId: user.Id, TradeNo: "activity-pending-order", Amount: 1,
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending, CreateTime: 100}
	require.NoError(t, order.Insert())
	const callback = "test:reward-activity-topup-event-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "RewardActivityEvent" {
			tx.AddError(errors.New("source event unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	require.ErrorContains(t, ManualCompleteTopUp(order.TradeNo, ""), "source event unavailable")
	var pending TopUp
	require.NoError(t, DB.First(&pending, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, pending.Status)
	var unchanged User
	require.NoError(t, DB.First(&unchanged, user.Id).Error)
	assert.Zero(t, unchanged.Quota)
	require.NoError(t, DB.Callback().Create().Remove(callback))
	require.NoError(t, ManualCompleteTopUp(order.TradeNo, ""))
	require.NoError(t, ManualCompleteTopUp(order.TradeNo, ""))
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, RewardActivitySourceMain, rows[0].Source)
	assert.Equal(t, "topup", rows[0].Event.Kind)
	assert.Equal(t, int64(order.Id), rows[0].Event.SourceID)
	require.NoError(t, DB.First(&pending, order.Id).Error)
	assert.Equal(t, pending.CompleteTime, rows[0].Event.OccurredAt)
}

func TestRewardActivityExcludesErrorSystemTopUpAndSubsiteLogs(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	for _, log := range []Log{
		{UserId: 17, CreatedAt: 123, Type: LogTypeError},
		{UserId: 17, CreatedAt: 123, Type: LogTypeSystem},
		{UserId: 17, CreatedAt: 123, Type: LogTypeTopup},
		{UserId: 17, SiteId: 8, CreatedAt: 123, Type: LogTypeConsume},
		{UserId: 0, CreatedAt: 123, Type: LogTypeConsume},
	} {
		require.NoError(t, createLog(&log))
	}
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	assert.Equal(t, int64(5), count)
}

func TestRewardActivityCapturesEachMainSitePaymentCompletionPath(t *testing.T) {
	for _, provider := range []string{PaymentProviderEpay, PaymentProviderStripe,
		PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			setupRewardActivityEventTestDB(t)
			user := createReserveTestUser(t, 0)
			order := &TopUp{UserId: user.Id, TradeNo: "activity-" + provider, Amount: 1, Money: 1,
				PaymentProvider: provider, Status: common.TopUpStatusPending, CreateTime: 100}
			require.NoError(t, order.Insert())
			var err error
			switch provider {
			case PaymentProviderEpay:
				_, err = RechargeEpay(order.TradeNo, "alipay", "")
			case PaymentProviderStripe:
				err = Recharge(order.TradeNo, "", "", "")
			case PaymentProviderCreem:
				err = RechargeCreem(order.TradeNo, "", "", "")
			case PaymentProviderWaffo:
				err = RechargeWaffo(order.TradeNo, "")
			case PaymentProviderWaffoPancake:
				err = RechargeWaffoPancake(order.TradeNo)
			}
			require.NoError(t, err)
			rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, "topup", rows[0].Event.Kind)
			assert.Equal(t, user.Id, rows[0].Event.UserID)
			assert.Equal(t, int64(order.Id), rows[0].Event.SourceID)
		})
	}
}

func TestRewardActivitySubscriptionTopUpMappingCommitsAndRollsBackWithItsEvent(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	order := &SubscriptionOrder{UserId: 17, TradeNo: "subscription-activity", Money: 5,
		PaymentMethod: PaymentMethodStripe, CreateTime: 100}
	err := DB.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, upsertSubscriptionTopUpTx(tx, order))
		return errors.New("subscription transaction rolled back")
	})
	require.ErrorContains(t, err, "rolled back")
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&RewardActivityEvent{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return upsertSubscriptionTopUpTx(tx, order) }))
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "topup", rows[0].Event.Kind)
	assert.Equal(t, 17, rows[0].Event.UserID)
}

func TestRewardActivityPendingQueueFindsLateLowerIDsAndSharesBatchesAcrossStores(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	for _, db := range []*gorm.DB{DB, LOG_DB} {
		for id := int64(10); id < 13; id++ {
			require.NoError(t, db.Create(&RewardActivityEvent{Id: id, Kind: "usage", UserID: 17,
				OccurredAt: 123, Confirmed: true}).Error)
		}
	}
	rows, err := ListPendingRewardActivityEvents(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	bySource := map[string]int{}
	for _, row := range rows {
		bySource[row.Source]++
		require.NoError(t, AckRewardActivityEvent(context.Background(), row))
	}
	assert.Equal(t, map[string]int{RewardActivitySourceMain: 2, RewardActivitySourceLog: 2}, bySource)
	// This row became visible after higher IDs were acknowledged. It must not
	// be hidden by a persisted or in-memory maximum-ID checkpoint.
	require.NoError(t, DB.Create(&RewardActivityEvent{Id: 3, Kind: "topup", UserID: 19,
		OccurredAt: 123, Confirmed: true}).Error)
	rows, err = ListPendingRewardActivityEvents(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, int64(3), rows[0].Event.Id)
}

func TestRewardActivitySingleRowBatchesDoNotStarveEitherBusySource(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	for _, db := range []*gorm.DB{DB, LOG_DB} {
		require.NoError(t, db.Create(&RewardActivityEvent{Kind: "usage", UserID: 17,
			OccurredAt: 123, Confirmed: true}).Error)
	}
	first, err := ListPendingRewardActivityEvents(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := ListPendingRewardActivityEvents(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.NotEqual(t, first[0].Source, second[0].Source)
}

func TestRewardActivityStartupRequiresBothStoresAndMigrationPreservesPendingRows(t *testing.T) {
	setupRewardTestDB(t)
	require.ErrorContains(t, InitializeRewardActivityCapture(), "source table is missing")
	require.NoError(t, MigrateRewardActivityEvents(DB))
	require.ErrorContains(t, InitializeRewardActivityCapture(), "source table is missing")
	require.NoError(t, MigrateRewardActivityEvents(LOG_DB))
	event := RewardActivityEvent{Kind: "usage", UserID: 17, OccurredAt: 123, Confirmed: true, RequestID: "retained"}
	require.NoError(t, LOG_DB.Create(&event).Error)
	require.NoError(t, MigrateRewardActivityEvents(LOG_DB))
	require.NoError(t, InitializeRewardActivityCapture())
	var preserved RewardActivityEvent
	require.NoError(t, LOG_DB.First(&preserved, event.Id).Error)
	assert.Equal(t, event, preserved)
}

func TestRewardActivityStartupRegistersExistingTablesWithoutRunningMigrations(t *testing.T) {
	setupRewardTestDB(t)
	// A non-master node does not migrate. Startup still checks and registers
	// both existing stores before source writes are accepted.
	require.NoError(t, DB.AutoMigrate(&RewardActivityEvent{}))
	require.NoError(t, LOG_DB.AutoMigrate(&RewardActivityEvent{}))
	assert.False(t, rewardActivityCaptureEnabled(DB))
	assert.False(t, rewardActivityCaptureEnabled(LOG_DB))
	require.NoError(t, InitializeRewardActivityCapture())
	assert.True(t, rewardActivityCaptureEnabled(DB))
	assert.True(t, rewardActivityCaptureEnabled(LOG_DB))
	require.NoError(t, createLog(&Log{UserId: 17, CreatedAt: 123, Type: LogTypeConsume}))
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestRewardActivitySharedMainAndLogStoreReturnsEachEventOnce(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	previousLogDB := LOG_DB
	LOG_DB = DB
	t.Cleanup(func() { LOG_DB = previousLogDB })
	require.NoError(t, DB.AutoMigrate(&Log{}))
	require.NoError(t, createLog(&Log{UserId: 17, CreatedAt: 123, Type: LogTypeConsume}))
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, RewardActivitySourceMain, rows[0].Source)
	require.NoError(t, AckRewardActivityEvent(context.Background(), rows[0]))
	rows, err = ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// These tests exercise the ClickHouse intent protocol with a separate SQLite
// source store and injected write failures. They are not a real ClickHouse
// compatibility test and do not pretend SQLite supports ClickHouse semantics.
func TestRewardActivityClickHouseIntentSurvivesSourceFailureAndMatchesExactIdentity(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	const callback = "test:reward-activity-source-failure"
	require.NoError(t, LOG_DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		tx.AddError(errors.New("source commit response unknown"))
	}))
	t.Cleanup(func() { _ = LOG_DB.Callback().Create().Remove(callback) })
	log := Log{UserId: 17, CreatedAt: 123, Type: LogTypeConsume, RequestId: "source-identity"}
	require.ErrorContains(t, createLog(&log), "source commit response unknown")
	var intent RewardActivityEvent
	require.NoError(t, DB.First(&intent).Error)
	assert.False(t, intent.Confirmed)
	require.NoError(t, LOG_DB.Callback().Create().Remove(callback))
	// The same request ID alone is insufficient evidence: a different timestamp,
	// user, site, or log type cannot confirm this successful-usage intent.
	for _, wrong := range []Log{
		{UserId: 17, CreatedAt: 124, Type: LogTypeConsume, RequestId: log.RequestId},
		{UserId: 18, CreatedAt: 123, Type: LogTypeConsume, RequestId: log.RequestId},
		{UserId: 17, SiteId: 9, CreatedAt: 123, Type: LogTypeConsume, RequestId: log.RequestId},
		{UserId: 17, CreatedAt: 123, Type: LogTypeError, RequestId: log.RequestId},
	} {
		require.NoError(t, LOG_DB.Create(&wrong).Error)
	}
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	var waiting RewardActivityEvent
	require.NoError(t, DB.First(&waiting, intent.Id).Error)
	assert.False(t, waiting.Confirmed)
	assert.Greater(t, waiting.NextAttemptAt, int64(0))
	// Simulate visibility of the original source commit after an uncertain
	// response, followed by a process restart and its persisted due retry.
	require.NoError(t, LOG_DB.Create(&log).Error)
	require.NoError(t, DB.Model(&waiting).Update("next_attempt_at", 0).Error)
	require.NoError(t, InitializeRewardActivityCapture())
	rows, err = ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, RewardActivitySourceMain, rows[0].Source)
	assert.Equal(t, intent.Id, rows[0].Event.Id)
	assert.True(t, rows[0].Event.Confirmed)
}

func TestRewardActivityClickHouseIntentFailurePreventsUntrackedSourceWrite(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	const callback = "test:reward-activity-intent-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "RewardActivityEvent" {
			tx.AddError(errors.New("intent unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	require.ErrorContains(t, createLog(&Log{UserId: 17, CreatedAt: 123, Type: LogTypeConsume}), "intent unavailable")
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRewardActivityClickHouseConfirmationFailureKeepsRecoverableIntent(t *testing.T) {
	setupRewardActivityEventTestDB(t)
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	const callback = "test:reward-activity-confirmation-failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		tx.AddError(errors.New("confirmation unavailable"))
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	log := Log{UserId: 17, CreatedAt: 123, Type: LogTypeConsume, RequestId: "recover-confirmation"}
	require.ErrorContains(t, createLog(&log), "confirmation unavailable")
	var persisted Log
	require.NoError(t, LOG_DB.First(&persisted).Error)
	var intent RewardActivityEvent
	require.NoError(t, DB.First(&intent).Error)
	assert.False(t, intent.Confirmed)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	rows, err := ListPendingRewardActivityEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, intent.Id, rows[0].Event.Id)
}
