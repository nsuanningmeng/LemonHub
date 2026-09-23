package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Keep unrelated user fields and indexes identical while reproducing the four
// 32-bit wallet columns that existed before large recharge support.
type legacyIntWalletUser struct {
	User
	Quota           int32 `gorm:"type:int;default:0"`
	UsedQuota       int32 `gorm:"type:int;default:0;column:used_quota"`
	AffQuota        int32 `gorm:"type:int;default:0;column:aff_quota"`
	AffHistoryQuota int32 `gorm:"type:int;default:0;column:aff_history"`
}

func (legacyIntWalletUser) TableName() string { return "users" }

var walletQuotaMigrationDatabaseName = regexp.MustCompile(`^lemonhub_wallet_quota_migration_test_[a-z0-9_]+$`)

func TestSQLiteWalletQuotaMigrationPreservesData(t *testing.T) {
	recorder := &migrationSQLRecorder{}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "wallet.db")), &gorm.Config{Logger: recorder})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	testWalletQuotaMigration(t, db, recorder)
}

func TestMySQLWalletQuotaMigrationPreservesData(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_WALLET_MIGRATION_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_WALLET_MIGRATION_DSN to a disposable lemonhub_wallet_quota_migration_test_* database")
	}
	config, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexp(t, walletQuotaMigrationDatabaseName, config.DBName, "refusing migration test outside a disposable wallet test database")
	config.ParseTime = true
	recorder := &migrationSQLRecorder{}
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), &gorm.Config{Logger: recorder})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Equal(t, config.DBName, selectedDatabase)
	require.Regexp(t, walletQuotaMigrationDatabaseName, selectedDatabase)
	testWalletQuotaMigration(t, db, recorder)
}

func TestPostgresWalletQuotaMigrationPreservesData(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("POSTGRES_WALLET_MIGRATION_DSN"))
	if rawDSN == "" {
		t.Skip("set POSTGRES_WALLET_MIGRATION_DSN to a disposable lemonhub_wallet_quota_migration_test_* database")
	}
	config, err := pgx.ParseConfig(rawDSN)
	require.NoError(t, err)
	require.Regexp(t, walletQuotaMigrationDatabaseName, config.Database, "refusing migration test outside a disposable wallet test database")
	recorder := &migrationSQLRecorder{}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: rawDSN, PreferSimpleProtocol: true}), &gorm.Config{Logger: recorder})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&selectedDatabase).Error)
	require.Equal(t, config.Database, selectedDatabase)
	require.Regexp(t, walletQuotaMigrationDatabaseName, selectedDatabase)
	testWalletQuotaMigration(t, db, recorder)
}

func testWalletQuotaMigration(t *testing.T, db *gorm.DB, recorder *migrationSQLRecorder) {
	t.Helper()
	if strconv.IntSize < 64 {
		t.Skip("large wallet balances require a 64-bit server build")
	}
	require.NoError(t, db.Migrator().DropTable("users"))
	t.Cleanup(func() { _ = db.Migrator().DropTable("users") })
	require.NoError(t, db.AutoMigrate(&legacyIntWalletUser{}))
	legacy := []legacyIntWalletUser{
		{
			User: User{
				Id: 401, SiteId: 7, Username: "wallet-migration", Password: "legacy-password",
				DisplayName: "Existing wallet", Email: "wallet@example.com", Group: "premium",
				AffCode: "migration-active", Setting: `{"currency":"CNY"}`,
				CreatedAt: 1_700_050_000, LastLoginAt: 1_700_050_100, RequestCount: 123,
			},
			Quota: 2_147_483_647, UsedQuota: 1_234_567_890,
			AffQuota: 123_456_789, AffHistoryQuota: 2_000_000_000,
		},
		{
			User: User{
				Id: 402, SiteId: 8, Username: "deleted-wallet", Password: "deleted-password",
				AffCode: "migration-deleted", CreatedAt: 1_700_040_000,
				DeletedAt: gorm.DeletedAt{Time: time.Unix(1_700_060_000, 0).UTC(), Valid: true},
			},
			Quota: -100, UsedQuota: 500, AffQuota: -50, AffHistoryQuota: -25,
		},
	}
	require.NoError(t, db.Create(&legacy).Error)
	// Some dialects repeatedly alter existing uniqueIndex columns even before
	// widening wallets. New quota types must not introduce any additional DDL.
	recorder.reset()
	require.NoError(t, db.AutoMigrate(&legacyIntWalletUser{}))
	existingSchemaMutations := recorder.schemaMutations()
	wantInitialQuota := int(legacy[0].Quota)
	if db.Dialector.Name() == "sqlite" {
		// SQLite's legacy INTEGER column already accepts existing large balances.
		// Preserve the production-shaped value when declaring BIGINT explicitly.
		wantInitialQuota = 999_994_138_683_436
		require.NoError(t, db.Model(&User{}).Where("id = ?", legacy[0].Id).Update("quota", wantInitialQuota).Error)
	}
	for column, dataType := range walletQuotaColumnTypes(t, db) {
		require.Contains(t, []string{"int", "integer", "int4"}, dataType, "legacy %s must reproduce an INT column", column)
	}
	var before []User
	require.NoError(t, db.Unscoped().Order("id").Find(&before).Error)
	require.Len(t, before, 2)
	require.Equal(t, wantInitialQuota, before[0].Quota)
	require.Equal(t, int(legacy[1].AffQuota), before[1].AffQuota)

	require.NoError(t, db.AutoMigrate(&User{}), "wallet INT to BIGINT migration must succeed")
	for column, dataType := range walletQuotaColumnTypes(t, db) {
		assert.Contains(t, []string{"bigint", "int8"}, dataType, "%s must support signed 64-bit balances", column)
	}
	var after []User
	require.NoError(t, db.Unscoped().Order("id").Find(&after).Error)
	assert.Equal(t, before, after, "migration must preserve every field and soft-deleted user")

	// JavaScript's largest exact integer is the inclusive wallet balance limit.
	const maxWalletQuota = common.MaxWalletQuota
	require.NoError(t, db.Model(&User{}).Where("id = ?", legacy[0].Id).Updates(map[string]any{
		"quota": maxWalletQuota, "used_quota": maxWalletQuota - 1,
		"aff_quota": maxWalletQuota - 2, "aff_history": maxWalletQuota - 3,
	}).Error)
	var widened User
	require.NoError(t, db.First(&widened, legacy[0].Id).Error)
	assert.Equal(t, int(maxWalletQuota), widened.Quota)
	assert.Equal(t, int(maxWalletQuota-1), widened.UsedQuota)
	assert.Equal(t, int(maxWalletQuota-2), widened.AffQuota)
	assert.Equal(t, int(maxWalletQuota-3), widened.AffHistoryQuota)

	var sqliteSchemaBefore []string
	if db.Dialector.Name() == "sqlite" {
		require.NoError(t, db.Table("sqlite_master").Where("tbl_name = ? AND sql IS NOT NULL", "users").
			Order("type, name").Pluck("sql", &sqliteSchemaBefore).Error)
	}
	recorder.reset()
	require.NoError(t, db.AutoMigrate(&User{}), "wallet migration must be safe to repeat")
	if db.Dialector.Name() == "sqlite" {
		// This SQLite driver rebuilds users for existing uniqueIndex metadata on
		// every migration. Verify its resulting schema and indexes stay identical.
		var sqliteSchemaAfter []string
		require.NoError(t, db.Table("sqlite_master").Where("tbl_name = ? AND sql IS NOT NULL", "users").
			Order("type, name").Pluck("sql", &sqliteSchemaAfter).Error)
		assert.Equal(t, sqliteSchemaBefore, sqliteSchemaAfter)
	} else {
		assert.Equal(t, existingSchemaMutations, recorder.schemaMutations(),
			"wallet widening must not introduce recurring table-changing DDL")
	}
	var final []User
	require.NoError(t, db.Unscoped().Order("id").Find(&final).Error)
	assert.Equal(t, []User{widened, before[1]}, final, "repeated migration must preserve large balances and other rows")
}

func walletQuotaColumnTypes(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	columns, err := db.Migrator().ColumnTypes(&User{})
	require.NoError(t, err)
	quotaTypes := make(map[string]string)
	for _, column := range columns {
		switch column.Name() {
		case "quota", "used_quota", "aff_quota", "aff_history":
			quotaTypes[column.Name()] = strings.ToLower(column.DatabaseTypeName())
		}
	}
	require.Len(t, quotaTypes, 4)
	return quotaTypes
}
