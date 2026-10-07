package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetQuotaDataDurabilityState(t *testing.T) {
	t.Helper()
	CacheQuotaDataLock.Lock()
	previous := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	previousUncertain, previousError := quotaDataUncertain, quotaDataFlushError
	quotaDataUncertain, quotaDataFlushError = nil, nil
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = previous
		CacheQuotaDataLock.Unlock()
		quotaDataUncertain, quotaDataFlushError = previousUncertain, previousError
	})
}

func TestQuotaDataFlushRetainsWholeBatchAfterWriteFailure(t *testing.T) {
	truncateTables(t)
	resetQuotaDataDurabilityState(t)
	for _, id := range []int{8201, 8202} {
		LogQuotaData(QuotaDataLogParams{UserID: id, Username: "durability", ModelName: "test", CreatedAt: 3600, Quota: 7, TokenUsed: 11})
	}
	forced := errors.New("quota data second insert rejected")
	writes := 0
	const callback = "test:quota_data_second_insert"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data" {
			writes++
			if writes == 2 {
				tx.AddError(forced)
			}
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	require.ErrorIs(t, SaveQuotaDataCache(), forced)
	var count int64
	require.NoError(t, DB.Model(&QuotaData{}).Count(&count).Error)
	assert.Zero(t, count, "a rejected row must roll back the whole batch")
	CacheQuotaDataLock.Lock()
	assert.Len(t, CacheQuotaData, 2, "known failed writes must remain retryable")
	CacheQuotaDataLock.Unlock()
	require.NoError(t, DB.Callback().Create().Remove(callback))
	require.NoError(t, SaveQuotaDataCache())
	require.NoError(t, SaveQuotaDataCache(), "successful snapshots must not be replayed")
	var rows []QuotaData
	require.NoError(t, DB.Order("user_id").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, 1, row.Count)
		assert.Equal(t, 7, row.Quota)
		assert.Equal(t, int64(11), row.TokenUsed)
	}
}

func TestQuotaDataFlushQueryAndUpdateFailuresRemainRetryable(t *testing.T) {
	for _, mode := range []string{"query", "update", "missing_row"} {
		t.Run(mode, func(t *testing.T) {
			truncateTables(t)
			resetQuotaDataDurabilityState(t)
			row := QuotaData{UserID: 8201, Username: "durability", CreatedAt: 3600, Count: 3, Quota: 20, TokenUsed: 30}
			require.NoError(t, DB.Create(&row).Error)
			LogQuotaData(QuotaDataLogParams{UserID: row.UserID, Username: row.Username, CreatedAt: row.CreatedAt, Quota: 7, TokenUsed: 11})
			forced := errors.New("dashboard read/write rejected")
			callback := "test:quota_data_" + mode
			if mode == "query" {
				require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "quota_data" {
						tx.AddError(forced)
					}
				}))
				t.Cleanup(func() { _ = DB.Callback().Query().Remove(callback) })
			} else if mode == "missing_row" {
				require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "quota_data" {
						tx.AddError(tx.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM quota_data WHERE id = ?", row.Id).Error)
					}
				}))
				t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
			} else {
				require.NoError(t, DB.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "quota_data" {
						tx.AddError(forced)
					}
				}))
				t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
			}
			err := SaveQuotaDataCache()
			require.Error(t, err)
			if mode != "missing_row" {
				require.ErrorIs(t, err, forced)
			}
			if mode == "query" {
				require.NoError(t, DB.Callback().Query().Remove(callback))
			} else {
				require.NoError(t, DB.Callback().Update().Remove(callback))
			}
			var got QuotaData
			require.NoError(t, DB.First(&got, row.Id).Error)
			assert.Equal(t, row, got, "failed writes must not partially change persisted statistics")
			require.NoError(t, SaveQuotaDataCache())
			require.NoError(t, DB.First(&got, row.Id).Error)
			assert.Equal(t, 4, got.Count)
			assert.Equal(t, 27, got.Quota)
			assert.Equal(t, int64(41), got.TokenUsed)
		})
	}
}

func TestQuotaDataFlushPreservesConcurrentTailAndCancellableWait(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[rollback], func(t *testing.T) {
			truncateTables(t)
			resetQuotaDataDurabilityState(t)
			params := QuotaDataLogParams{UserID: 8201, Username: "durability", CreatedAt: 3600, Quota: 7, TokenUsed: 11}
			LogQuotaData(params)
			entered, release := make(chan struct{}), make(chan struct{})
			forced := errors.New("blocked dashboard insert rejected")
			const callback = "test:quota_data_blocked_insert"
			require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "quota_data" {
					close(entered)
					<-release
					if rollback {
						tx.AddError(forced)
					}
				}
			}))
			t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
			done := make(chan error, 1)
			go func() { done <- SaveQuotaDataCache() }()
			<-entered
			LogQuotaData(params)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			assert.ErrorIs(t, SaveQuotaDataCacheContext(ctx), context.Canceled)
			close(release)
			err := <-done
			if rollback {
				require.ErrorIs(t, err, forced)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, DB.Callback().Create().Remove(callback))
			require.NoError(t, SaveQuotaDataCache())
			var rows []QuotaData
			require.NoError(t, DB.Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, 2, rows[0].Count)
			assert.Equal(t, 14, rows[0].Quota)
			assert.Equal(t, int64(22), rows[0].TokenUsed)
		})
	}
}

func TestQuotaDataFlushUnknownOutcomeNeverReplays(t *testing.T) {
	for _, mode := range []string{"commit_applied", "commit_not_applied", "rollback_unknown"} {
		t.Run(mode, func(t *testing.T) {
			truncateTables(t)
			resetQuotaDataDurabilityState(t)
			sqlDB, err := DB.DB()
			require.NoError(t, err)
			pool := &batchOutcomeTestPool{DB: sqlDB, mode: mode, forced: errors.New("dashboard transaction acknowledgement lost")}
			originalPool, originalStatementPool := DB.ConnPool, DB.Statement.ConnPool
			DB.ConnPool, DB.Statement.ConnPool = pool, pool
			t.Cleanup(func() { DB.ConnPool, DB.Statement.ConnPool = originalPool, originalStatementPool })
			if mode == "rollback_unknown" {
				const callback = "test:quota_data_unknown_rollback"
				require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "quota_data" {
						tx.AddError(errors.New("dashboard insert rejected"))
					}
				}))
				t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
			}
			params := QuotaDataLogParams{UserID: 8201, Username: "durability", CreatedAt: 3600, Quota: 7, TokenUsed: 11}
			LogQuotaData(params)
			require.ErrorIs(t, SaveQuotaDataCache(), pool.forced)
			LogQuotaData(params)
			require.ErrorIs(t, SaveQuotaDataCache(), pool.forced)
			assert.Equal(t, 1, pool.begins, "unknown additive writes must not start another transaction")
			var rows []QuotaData
			require.NoError(t, DB.Find(&rows).Error)
			if mode == "commit_applied" {
				require.Len(t, rows, 1)
				assert.Equal(t, 1, rows[0].Count)
				assert.Equal(t, 7, rows[0].Quota)
			} else {
				assert.Empty(t, rows)
			}
			require.Len(t, quotaDataUncertain, 1)
			for _, row := range quotaDataUncertain {
				assert.Equal(t, 7, row.Quota)
				assert.Equal(t, 1, row.Count)
			}
			CacheQuotaDataLock.Lock()
			assert.Len(t, CacheQuotaData, 1, "new tail must remain separate from the unknown snapshot")
			CacheQuotaDataLock.Unlock()
		})
	}
}

func TestQuotaDataFlushAddsDeltaOnlyOnceAcrossLegacyDuplicateDimensions(t *testing.T) {
	truncateTables(t)
	resetQuotaDataDurabilityState(t)
	for range 2 {
		require.NoError(t, DB.Create(&QuotaData{UserID: 8201, Username: "durability", CreatedAt: 3600, Count: 1, Quota: 5, TokenUsed: 3}).Error)
	}
	LogQuotaData(QuotaDataLogParams{UserID: 8201, Username: "durability", CreatedAt: 3600, Quota: 7, TokenUsed: 11})
	require.NoError(t, SaveQuotaDataCache())
	var rows []QuotaData
	require.NoError(t, DB.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, 3, rows[0].Count+rows[1].Count)
	assert.Equal(t, 17, rows[0].Quota+rows[1].Quota)
	assert.Equal(t, int64(17), rows[0].TokenUsed+rows[1].TokenUsed)
}

func TestQuotaDataFlushPreservesNewDeltasOnNullableLegacyRows(t *testing.T) {
	truncateTables(t)
	resetQuotaDataDurabilityState(t)
	rows := []QuotaData{
		{UserID: 8201, Username: "null-active", CreatedAt: 3600, Count: 3, Quota: 20, TokenUsed: 30},
		{UserID: 8202, Username: "null-idle", CreatedAt: 3600},
	}
	require.NoError(t, DB.Create(&rows).Error)
	require.NoError(t, DB.Exec("UPDATE quota_data SET count = NULL, token_used = NULL WHERE id = ?", rows[0].Id).Error)
	require.NoError(t, DB.Exec("UPDATE quota_data SET count = NULL, quota = NULL, token_used = NULL WHERE id = ?", rows[1].Id).Error)
	LogQuotaData(QuotaDataLogParams{UserID: 8201, Username: "null-active", CreatedAt: 3600, Quota: 7, TokenUsed: 11})
	require.NoError(t, SaveQuotaDataCache())
	var got QuotaData
	require.NoError(t, DB.First(&got, rows[0].Id).Error)
	assert.Equal(t, 1, got.Count)
	assert.Equal(t, 27, got.Quota)
	assert.Equal(t, int64(11), got.TokenUsed)
	var idle struct {
		Count     *int
		Quota     *int
		TokenUsed *int64
	}
	require.NoError(t, DB.Table("quota_data").Where("id = ?", rows[1].Id).First(&idle).Error)
	assert.Nil(t, idle.Count)
	assert.Nil(t, idle.Quota)
	assert.Nil(t, idle.TokenUsed)
}
