package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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

// This is the schema before durable subscription settlement receipts existed.
// Keep it independent of the current model so new fields cannot enter the fixture.
type legacySubscriptionPreConsumeRecord struct {
	Id                 int    `json:"id"`
	RequestId          string `gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `gorm:"index"`
	UserSubscriptionId int    `gorm:"index"`
	PreConsumed        int64  `gorm:"type:bigint;not null;default:0"`
	Status             string `gorm:"type:varchar(32);index"`
	CreatedAt          int64  `gorm:"bigint"`
	UpdatedAt          int64  `gorm:"bigint;index"`
}

func (legacySubscriptionPreConsumeRecord) TableName() string {
	return "subscription_pre_consume_records"
}

func TestSubscriptionReceiptMigrationPreservesLegacyRows(t *testing.T) {
	for _, backend := range []struct {
		name, env    string
		databaseType common.DatabaseType
	}{
		{name: "sqlite", databaseType: common.DatabaseTypeSQLite},
		{name: "mysql", env: "MYSQL_SUBSCRIPTION_RECEIPT_MIGRATION_DSN", databaseType: common.DatabaseTypeMySQL},
		{name: "postgres", env: "POSTGRES_SUBSCRIPTION_RECEIPT_MIGRATION_DSN", databaseType: common.DatabaseTypePostgreSQL},
	} {
		t.Run(backend.name, func(t *testing.T) {
			var dialector gorm.Dialector
			var expectedDatabase string
			dsn := strings.TrimSpace(os.Getenv(backend.env))
			if backend.env != "" && dsn == "" {
				t.Skip("set " + backend.env + " to a fresh lemonhub_subscription_receipt_test_* database")
			}
			switch backend.name {
			case "sqlite":
				dialector = sqlite.Open(filepath.Join(t.TempDir(), "receipt-migration.db"))
			case "mysql":
				config, err := mysqldriver.ParseDSN(dsn)
				require.NoError(t, err)
				config.ParseTime = true
				expectedDatabase = config.DBName
				dialector = gormmysql.Open(config.FormatDSN())
			case "postgres":
				config, err := pgx.ParseConfig(dsn)
				require.NoError(t, err)
				expectedDatabase = config.Database
				dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			if backend.env != "" {
				require.Regexp(t, regexp.MustCompile(`^lemonhub_subscription_receipt_test_[a-z0-9_]+$`), expectedDatabase,
					"refusing migration fixture outside an explicitly named disposable database")
			}
			db, err := gorm.Open(dialector, newGormConfig(false))
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			if backend.env != "" {
				query := "SELECT DATABASE()"
				if backend.name == "postgres" {
					query = "SELECT current_database()"
				}
				var actualDatabase string
				require.NoError(t, db.Raw(query).Scan(&actualDatabase).Error)
				require.Equal(t, expectedDatabase, actualDatabase)
			}
			existingTables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			require.Empty(t, existingTables, "refusing to change a nonempty fixture database")
			t.Cleanup(func() {
				tables, err := db.Migrator().GetTables()
				require.NoError(t, err)
				for _, table := range tables {
					require.NoError(t, db.Migrator().DropTable(table))
				}
			})

			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(backend.databaseType, backend.databaseType)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
			})
			require.NoError(t, db.AutoMigrate(&legacySubscriptionPreConsumeRecord{}))
			require.False(t, db.Migrator().HasColumn(&legacySubscriptionPreConsumeRecord{}, "reserved_quota"))
			legacyRows := []legacySubscriptionPreConsumeRecord{
				{Id: 701, RequestId: "legacy-consumed-中文", UserId: 101, UserSubscriptionId: 201,
					PreConsumed: 5_000_000_123, Status: "consumed", CreatedAt: 1_800_000_000, UpdatedAt: 1_800_000_100},
				{Id: 702, RequestId: "legacy-refunded", UserId: 102, UserSubscriptionId: 202,
					PreConsumed: 987654, Status: "refunded", CreatedAt: 1_800_000_200, UpdatedAt: 1_800_000_300},
			}
			require.NoError(t, db.Create(&legacyRows).Error)
			var firstMigratedRows []map[string]interface{}
			for startup := 1; startup <= 2; startup++ {
				require.NoError(t, migrateDB(), "real startup migration %d", startup)
				var persistedLegacy []legacySubscriptionPreConsumeRecord
				require.NoError(t, db.Order("id").Find(&persistedLegacy).Error)
				assert.Equal(t, legacyRows, persistedLegacy, "migration must preserve legacy statuses, amounts and timestamps")
				var receipts []SubscriptionPreConsumeRecord
				require.NoError(t, db.Order("id").Find(&receipts).Error)
				require.Len(t, receipts, len(legacyRows))
				for _, receipt := range receipts {
					assert.Zero(t, receipt.ReservedQuota, "zero retains the legacy PreConsumed fallback")
					assert.False(t, receipt.TokenBound)
					assert.Zero(t, receipt.TokenId)
					assert.Zero(t, receipt.TokenConsumedQuota)
					assert.Zero(t, receipt.SettledQuota)
					assert.Zero(t, receipt.SubscriptionDelta)
					assert.Zero(t, receipt.WalletDelta)
					assert.Zero(t, receipt.TokenDelta)
					assert.Zero(t, receipt.SubscriptionUsedAfter)
					assert.Zero(t, receipt.SubscriptionTotal)
					assert.False(t, receipt.ReconciliationRequired, "legacy NULL must not require reconciliation")
				}
				var rawRows []map[string]interface{}
				require.NoError(t, db.Table("subscription_pre_consume_records").Order("id").Find(&rawRows).Error)
				for _, row := range rawRows {
					for _, column := range []string{"reserved_quota", "subscription_delta", "wallet_delta", "subscription_used_after", "subscription_total"} {
						require.Contains(t, row, column)
						require.NotNil(t, row[column], "legacy %s must receive the declared zero default", column)
					}
					for _, column := range []string{"token_bound", "token_id", "token_consumed_quota", "settled_quota", "token_delta", "reconciliation_required"} {
						require.Contains(t, row, column)
						assert.Nil(t, row[column], "migration must retain the nullable legacy %s state", column)
					}
				}
				if startup == 1 {
					firstMigratedRows = rawRows
				} else {
					assert.Equal(t, firstMigratedRows, rawRows, "repeated startup must retain raw NULL/zero receipt values")
				}
			}
		})
	}
}
