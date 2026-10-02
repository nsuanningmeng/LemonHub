package model

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This test owns only a fresh, explicitly named disposable database. It exercises
// the preflight followed by the real AutoMigrate, including non-STRICT servers
// where unsafe MODIFY statements succeed while silently changing stored data.
func TestMySQLMigrationPreflightPreservesWideColumns(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_MIGRATION_SAFETY_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_MIGRATION_SAFETY_DSN to a fresh lemonhub_migration_safety_test_* database")
	}
	config, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	databaseName := regexp.MustCompile(`^lemonhub_migration_safety_test_[a-z0-9_]+$`)
	require.Regexp(t, databaseName, config.DBName, "refusing migration test outside a disposable database")
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Equal(t, config.DBName, selectedDatabase)
	var existingTables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&existingTables).Error)
	require.Zero(t, existingTables, "refusing to modify a nonempty database")

	previousDB := DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, previousLogType)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
	})

	for _, sqlMode := range []string{"", "STRICT_ALL_TABLES"} {
		t.Run("sql_mode="+sqlMode, func(t *testing.T) {
			require.NoError(t, db.Exec("SET SESSION sql_mode = ?", sqlMode).Error)
			for _, test := range []struct {
				name, columnDDL, tableCollation, value string
				blocked                                bool
			}{
				{name: "mediumtext over TEXT byte limit", columnDDL: "MEDIUMTEXT", tableCollation: "utf8mb4_unicode_ci", value: strings.Repeat("x", 70000), blocked: true},
				{name: "longtext multibyte over TEXT byte limit", columnDDL: "LONGTEXT", tableCollation: "utf8mb4_unicode_ci", value: strings.Repeat("界", 22000), blocked: true},
				{name: "varchar explicit charset must not fall back", columnDDL: "VARCHAR(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", tableCollation: "utf8_general_ci", value: "模型-😀", blocked: true},
				{name: "text comment change must not reset charset", columnDDL: "TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci COMMENT 'custom'", tableCollation: "utf8_general_ci", value: "模型-😀", blocked: true},
				{name: "legacy varchar lossless upgrade", columnDDL: "VARCHAR(1024)", tableCollation: "utf8mb4_unicode_ci", value: "gpt-6-luna,模型"},
			} {
				t.Run(test.name, func(t *testing.T) {
					require.NoError(t, db.Exec("CREATE TABLE tokens (id BIGINT PRIMARY KEY, model_limits "+test.columnDDL+") DEFAULT COLLATE "+test.tableCollation).Error)
					t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE tokens").Error) })
					require.NoError(t, db.Exec("INSERT INTO tokens (id, model_limits) VALUES (7, ?)", test.value).Error)
					beforeSchema := migrationPreflightTableDDL(t, db, "tokens")
					migrationErr := migrateTokenModelLimitsToText()
					if migrationErr == nil {
						autoErr := db.AutoMigrate(&Token{})
						if !test.blocked {
							require.NoError(t, autoErr)
						}
					}
					if test.blocked {
						assert.Error(t, migrationErr, "must refuse before either migration issues unsafe DDL")
						assert.Equal(t, beforeSchema, migrationPreflightTableDDL(t, db, "tokens"))
					} else {
						require.NoError(t, migrationErr)
						require.NoError(t, migrateTokenModelLimitsToText())
					}
					var actual string
					require.NoError(t, db.Raw("SELECT model_limits FROM tokens WHERE id = 7").Scan(&actual).Error)
					assert.Equal(t, test.value, actual, "token restrictions must remain byte-for-byte intact")
				})
			}
			for _, test := range []struct {
				name, columnDDL, value string
				blocked                bool
			}{
				{name: "wide decimal magnitude", columnDDL: "DECIMAL(20,8) NULL DEFAULT NULL", value: "12345.12345678", blocked: true},
				{name: "wide decimal fractional precision", columnDDL: "DECIMAL(20,8) NULL DEFAULT NULL", value: "1.12345678", blocked: true},
				{name: "decimal fraction below float64 resolution", columnDDL: "DECIMAL(38,20) NULL DEFAULT NULL", value: "1.00000000000000000001", blocked: true},
				{name: "null decimal cannot become zero", columnDDL: "DECIMAL(10,6) NULL DEFAULT NULL", value: "NULL", blocked: true},
				{name: "wide decimal exact value", columnDDL: "DECIMAL(20,8) NULL DEFAULT NULL", value: "9.99000000"},
				{name: "legacy double exact value", columnDDL: "DOUBLE NULL DEFAULT NULL", value: "9.99"},
				{name: "legacy double excessive precision", columnDDL: "DOUBLE NULL DEFAULT NULL", value: "1.1234567", blocked: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					require.NoError(t, db.Exec("CREATE TABLE subscription_plans (id BIGINT PRIMARY KEY, title VARCHAR(128) NOT NULL, price_amount "+test.columnDDL+") DEFAULT COLLATE utf8mb4_unicode_ci").Error)
					t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE subscription_plans").Error) })
					// Values are closed test cases, including SQL NULL, not user input.
					require.NoError(t, db.Exec("INSERT INTO subscription_plans (id, title, price_amount) VALUES (7, 'preserve-title', "+test.value+")").Error)
					beforeSchema := migrationPreflightTableDDL(t, db, "subscription_plans")
					var beforeValue string
					require.NoError(t, db.Raw("SELECT COALESCE(CAST(price_amount AS CHAR), 'NULL') FROM subscription_plans WHERE id = 7").Scan(&beforeValue).Error)
					migrationErr := migrateSubscriptionPlanPriceAmount()
					if migrationErr == nil {
						autoErr := db.AutoMigrate(&SubscriptionPlan{})
						if !test.blocked {
							require.NoError(t, autoErr)
						}
					}
					var afterValue, title string
					require.NoError(t, db.Raw("SELECT COALESCE(CAST(price_amount AS CHAR), 'NULL') FROM subscription_plans WHERE id = 7").Scan(&afterValue).Error)
					require.NoError(t, db.Raw("SELECT title FROM subscription_plans WHERE id = 7").Scan(&title).Error)
					assert.Equal(t, "preserve-title", title)
					if test.blocked {
						assert.Error(t, migrationErr, "must refuse before AutoMigrate can narrow the existing decimal")
						assert.Equal(t, beforeSchema, migrationPreflightTableDDL(t, db, "subscription_plans"))
						assert.Equal(t, beforeValue, afterValue, "rejected prices must keep their exact persisted digits")
					} else {
						require.NoError(t, migrationErr)
						beforeAmount, err := decimal.NewFromString(beforeValue)
						require.NoError(t, err)
						afterAmount, err := decimal.NewFromString(afterValue)
						require.NoError(t, err)
						assert.True(t, beforeAmount.Equal(afterAmount), "%s must equal %s exactly", beforeValue, afterValue)
						upgradedSchema := migrationPreflightTableDDL(t, db, "subscription_plans")
						require.NoError(t, migrateSubscriptionPlanPriceAmount())
						require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
						assert.Equal(t, upgradedSchema, migrationPreflightTableDDL(t, db, "subscription_plans"), "restart must be idempotent")
					}
				})
			}
			for _, entry := range []struct {
				name    string
				migrate func() error
			}{
				{name: "serial startup", migrate: migrateDB},
				{name: "parallel startup", migrate: migrateDBFast},
			} {
				for _, fixture := range []struct {
					table, createSQL, insertSQL, valueSQL, value, column string
				}{
					{table: "tokens", createSQL: "CREATE TABLE tokens (id BIGINT PRIMARY KEY, model_limits MEDIUMTEXT) DEFAULT COLLATE utf8mb4_unicode_ci", insertSQL: "INSERT INTO tokens VALUES (7, ?)", valueSQL: "SELECT model_limits FROM tokens WHERE id = 7", value: strings.Repeat("x", 70000), column: "tokens.model_limits"},
					{table: "subscription_plans", createSQL: "CREATE TABLE subscription_plans (id BIGINT PRIMARY KEY, price_amount DECIMAL(38,20) NULL DEFAULT NULL) DEFAULT COLLATE utf8mb4_unicode_ci", insertSQL: "INSERT INTO subscription_plans VALUES (7, ?)", valueSQL: "SELECT CAST(price_amount AS CHAR) FROM subscription_plans WHERE id = 7", value: "1.00000000000000000001", column: "subscription_plans.price_amount"},
				} {
					t.Run(entry.name+"/"+fixture.table, func(t *testing.T) {
						require.NoError(t, db.Exec(fixture.createSQL).Error)
						t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE "+fixture.table).Error) })
						require.NoError(t, db.Exec(fixture.insertSQL, fixture.value).Error)
						beforeSchema := migrationPreflightTableDDL(t, db, fixture.table)
						require.ErrorContains(t, entry.migrate(), fixture.column)
						assert.Equal(t, beforeSchema, migrationPreflightTableDDL(t, db, fixture.table))
						var afterValue string
						require.NoError(t, db.Raw(fixture.valueSQL).Scan(&afterValue).Error)
						assert.Equal(t, fixture.value, afterValue)
						var tables int64
						require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error)
						assert.EqualValues(t, 1, tables, "startup must stop before unrelated AutoMigrate runs")
					})
				}
			}
		})
	}
}

func migrationPreflightTableDDL(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	require.Contains(t, []string{"tokens", "subscription_plans"}, table)
	var tableName, ddl string
	require.NoError(t, db.Raw("SHOW CREATE TABLE `"+table+"`").Row().Scan(&tableName, &ddl))
	return ddl
}
