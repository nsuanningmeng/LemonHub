package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// GetDBTimestamp returns a UNIX timestamp from database time.
// Falls back to application time on error.
func GetDBTimestamp() int64 {
	return GetDBTimestampTx(DB)
}

// GetDBTimestampTx reads database time through the caller's transaction. This
// keeps the query on the same connection and avoids borrowing a second pool
// connection while a transaction is holding the first one.
func GetDBTimestampTx(tx *gorm.DB) int64 {
	if tx == nil {
		tx = DB
	}
	var ts int64
	var err error
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		err = tx.Raw("SELECT EXTRACT(EPOCH FROM clock_timestamp())::bigint").Scan(&ts).Error
	case common.UsingMainDatabase(common.DatabaseTypeSQLite):
		err = tx.Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = tx.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
