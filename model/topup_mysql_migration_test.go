package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// oldTopUp reproduces the TopUp schema BEFORE this change: Status is an untyped Go
// string (GORM maps it to LONGTEXT on MySQL), payment_provider/create_time carry no
// index, and there is no idx_topup_provider_status_time. AutoMigrating this first lets
// the test faithfully simulate a pre-upgrade production table, then upgrade it.
type oldTopUp struct {
	Id              int     `json:"id"`
	SiteId          int     `json:"site_id" gorm:"type:int;default:0;index"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	PaymentIntent   string  `json:"payment_intent" gorm:"type:varchar(255);index;default:''"`
	ClawedBackQuota int64   `json:"clawed_back_quota" gorm:"default:0"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
}

func (oldTopUp) TableName() string { return "top_ups" }

// TestMySQLTopUpMigrationSafety verifies the epay-reconcile schema change is a SAFE
// in-place upgrade on a real MySQL 8 instance: on an EXISTING top_ups table with data,
// AutoMigrate must (1) narrow Status LONGTEXT -> varchar(32) without losing/truncating
// rows, (2) create the composite index idx_topup_provider_status_time in the right
// column order, (3) be idempotent (a second AutoMigrate issues no further DDL — no
// restart churn), and (4) keep the reconcile queries working under the MySQL dialect.
//
// Guarded by MYSQL_MIGRATION_DSN so the normal SQLite suite skips it; point it at a
// throwaway MySQL database to run, e.g.
//
//	MYSQL_MIGRATION_DSN='root:@tcp(127.0.0.1:3307)/lemonhub_migtest?charset=utf8mb4&parseTime=True&loc=Local'
func TestMySQLTopUpMigrationSafety(t *testing.T) {
	dsn := os.Getenv("MYSQL_MIGRATION_DSN")
	if dsn == "" {
		t.Skip("set MYSQL_MIGRATION_DSN to run the MySQL migration-safety test")
	}

	mdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := mdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// Start from a clean table, then build the OLD schema and seed data.
	require.NoError(t, mdb.Migrator().DropTable("top_ups"))
	require.NoError(t, mdb.AutoMigrate(&oldTopUp{}))

	// Confirm we really reproduced the old schema: Status is longtext/text, no composite index.
	statusType := mysqlColumnType(t, mdb, "top_ups", "status")
	require.Contains(t, strings.ToLower(statusType), "text", "precondition: old status column must be a TEXT type")
	require.False(t, mysqlIndexExists(t, mdb, "top_ups", "idx_topup_provider_status_time"), "precondition: composite index must not exist yet")

	// Seed rows covering every status literal + both site scopes + multiple providers.
	seed := []oldTopUp{
		{UserId: 1, Amount: 10, Money: 100, TradeNo: "MIG_pending", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending, CreateTime: 1000, SiteId: 0},
		{UserId: 2, Amount: 10, Money: 100, TradeNo: "MIG_success", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, CreateTime: 2000, SiteId: 0},
		{UserId: 3, Amount: 10, Money: 100, TradeNo: "MIG_manual", PaymentProvider: PaymentProviderEpay, Status: TopUpStatusManualReview, CreateTime: 3000, SiteId: 42},
		{UserId: 4, Amount: 10, Money: 100, TradeNo: "MIG_expired", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusExpired, CreateTime: 4000, SiteId: 0},
		{UserId: 5, Amount: 10, Money: 100, TradeNo: "MIG_refunded", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusRefunded, CreateTime: 5000, SiteId: 0},
		{UserId: 6, Amount: 10, Money: 100, TradeNo: "MIG_disputed", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusDisputed, CreateTime: 6000, SiteId: 0},
	}
	require.NoError(t, mdb.Create(&seed).Error)
	var beforeCount int64
	require.NoError(t, mdb.Table("top_ups").Count(&beforeCount).Error)
	require.Equal(t, int64(len(seed)), beforeCount)

	// --- The upgrade: AutoMigrate the CURRENT TopUp struct onto the old table. ---
	origDB := DB
	t.Cleanup(func() { DB = origDB })
	DB = mdb
	require.NoError(t, mdb.AutoMigrate(&TopUp{}), "AutoMigrate old->new must succeed on MySQL")

	// (1) Status narrowed to varchar(32); no data lost or truncated.
	newType := mysqlColumnType(t, mdb, "top_ups", "status")
	assert.Truef(t, strings.HasPrefix(strings.ToLower(newType), "varchar"), "status must become varchar, got %q", newType)
	var afterCount int64
	require.NoError(t, mdb.Table("top_ups").Count(&afterCount).Error)
	assert.Equal(t, beforeCount, afterCount, "no rows may be lost by the migration")
	for _, s := range seed {
		var got TopUp
		require.NoError(t, mdb.Where("trade_no = ?", s.TradeNo).First(&got).Error)
		assert.Equal(t, s.Status, got.Status, "status value must survive the type change intact")
	}

	// (2) Composite index created over (payment_provider, status, create_time), in order.
	require.True(t, mysqlIndexExists(t, mdb, "top_ups", "idx_topup_provider_status_time"), "composite index must be created")
	assert.Equal(t, []string{"payment_provider", "status", "create_time"},
		mysqlIndexColumns(t, mdb, "top_ups", "idx_topup_provider_status_time"), "index column order must match the query predicate")

	// (3) Idempotency / no restart churn — proven two ways:
	//   (a) SHOW CREATE TABLE is byte-identical before and after a second AutoMigrate.
	//   (b) The MySQL general log records ZERO ALTER/CREATE-INDEX statements against
	//       top_ups during that second AutoMigrate (the definitive churn check — the
	//       AGENTS.md boolean-default problem is exactly a repeated-ALTER-on-restart).
	createBefore := mysqlShowCreate(t, mdb, "top_ups")
	require.NoError(t, mdb.Exec("SET GLOBAL log_output = 'TABLE'").Error)
	require.NoError(t, mdb.Exec("TRUNCATE TABLE mysql.general_log").Error)
	require.NoError(t, mdb.Exec("SET GLOBAL general_log = 'ON'").Error)
	require.NoError(t, mdb.AutoMigrate(&TopUp{}), "second AutoMigrate must be a clean no-op")
	require.NoError(t, mdb.Exec("SET GLOBAL general_log = 'OFF'").Error)

	createAfter := mysqlShowCreate(t, mdb, "top_ups")
	assert.Equal(t, createBefore, createAfter, "second AutoMigrate must not alter the schema (no churn on restart)")

	var churnDDL int64
	require.NoError(t, mdb.Raw(
		"SELECT COUNT(*) FROM mysql.general_log WHERE CONVERT(argument USING utf8mb4) REGEXP ? ",
		"(ALTER|CREATE INDEX|DROP INDEX).*top_ups").Scan(&churnDDL).Error)
	assert.Equal(t, int64(0), churnDDL, "a settled schema must issue NO ALTER/CREATE-INDEX on re-migrate (restart churn)")

	// (4) The reconcile queries run under the MySQL dialect and return correct rows.
	assert.True(t, HasPendingEpayTopUps(0, 9999), "MySQL: pending epay order must be found")
	list, err := GetPendingEpayTopUps(0, 9999, 100)
	require.NoError(t, err)
	require.Len(t, list, 1, "MySQL: exactly the one pending epay order in window")
	assert.Equal(t, "MIG_pending", list[0].TradeNo)
}

func mysqlColumnType(t *testing.T, db *gorm.DB, table, column string) string {
	t.Helper()
	var colType string
	require.NoError(t, db.Raw(
		"SELECT COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?",
		table, column).Scan(&colType).Error)
	return colType
}

func mysqlIndexExists(t *testing.T, db *gorm.DB, table, index string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table, index).Scan(&n).Error)
	return n > 0
}

func mysqlIndexColumns(t *testing.T, db *gorm.DB, table, index string) []string {
	t.Helper()
	var cols []string
	require.NoError(t, db.Raw(
		"SELECT COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ? ORDER BY SEQ_IN_INDEX",
		table, index).Scan(&cols).Error)
	return cols
}

func mysqlShowCreate(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	var name, ddl string
	require.NoError(t, db.Raw("SHOW CREATE TABLE `"+table+"`").Row().Scan(&name, &ddl))
	return ddl
}
