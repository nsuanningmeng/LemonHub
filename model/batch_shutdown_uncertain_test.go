package model

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// batchOutcomeTestPool wraps a real SQLite transaction so commit success and
// returned driver errors can disagree, as when the commit acknowledgement is lost.
type batchOutcomeTestPool struct {
	*sql.DB
	mode   string
	forced error
	begins int
}

func (p *batchOutcomeTestPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	p.begins++
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &batchOutcomeTestTx{Tx: tx, mode: p.mode, forced: p.forced}, nil
}

type batchOutcomeTestTx struct {
	*sql.Tx
	mode   string
	forced error
}

func (t *batchOutcomeTestTx) Commit() error {
	if t.mode == "commit_not_applied" {
		_ = t.Tx.Rollback()
		return t.forced
	}
	err := t.Tx.Commit()
	if err == nil && t.mode == "commit_applied" {
		return t.forced
	}
	return err
}
func (t *batchOutcomeTestTx) Rollback() error {
	err := t.Tx.Rollback()
	if err == nil && t.mode == "rollback_unknown" {
		return t.forced
	}
	return err
}
func TestBatchOuterTransactionUncertaintyNeverReplays(t *testing.T) {
	for _, mode := range []string{"commit_applied", "commit_not_applied", "rollback_unknown"} {
		t.Run(mode, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			common.BatchUpdateEnabled = true
			channel := Channel{Name: "uncertain-outer-transaction", Key: "test"}
			require.NoError(t, DB.Create(&channel).Error)
			sqlDB, err := DB.DB()
			require.NoError(t, err)
			pool := &batchOutcomeTestPool{DB: sqlDB, mode: mode, forced: errors.New("forced outer transaction outcome unknown")}
			originalPool, originalStatementPool := DB.ConnPool, DB.Statement.ConnPool
			DB.ConnPool = pool
			DB.Statement.ConnPool = pool
			t.Cleanup(func() { DB.ConnPool = originalPool; DB.Statement.ConnPool = originalStatementPool })
			if mode == "rollback_unknown" {
				const cb = "test:batch_outer_rollback"
				require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(cb, func(tx *gorm.DB) {
					if tx.Statement.Table == "channels" {
						tx.AddError(errors.New("forced statement failure before SQL"))
					}
				}))
				t.Cleanup(func() { _ = DB.Callback().Update().Remove(cb) })
			}
			UpdateChannelUsedQuota(channel.Id, 5)
			require.ErrorIs(t, FlushBatchUpdates(context.Background()), pool.forced)
			require.Error(t, FlushBatchUpdates(context.Background()), "unknown transaction cannot be blindly retried")
			assert.Equal(t, 1, pool.begins, "second flush must not issue any new transaction")
			var got Channel
			require.NoError(t, DB.First(&got, channel.Id).Error)
			expected := int64(0)
			if mode == "commit_applied" {
				expected = 5
			}
			assert.Equal(t, expected, got.UsedQuota)
			assert.True(t, hasPendingBatchUpdate(BatchUpdateTypeChannelUsedQuota, channel.Id), "unknown transaction delta must remain visible for reconciliation")
		})
	}
}
