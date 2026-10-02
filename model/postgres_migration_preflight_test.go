package model

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgresMigrationPreflightPreservesPrices(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_MIGRATION_SAFETY_DSN"))
	if dsn == "" {
		t.Skip("set POSTGRES_MIGRATION_SAFETY_DSN to a fresh lemonhub_migration_safety_test_* database")
	}
	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^lemonhub_migration_safety_test_[a-z0-9_]+$`), config.Database)
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	require.Equal(t, config.Database, databaseName)
	var existingTables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')").Scan(&existingTables).Error)
	require.Zero(t, existingTables, "refusing to modify a nonempty database")

	previousDB := DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, previousLogType)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
	})

	for _, test := range []struct {
		name, columnDDL, value string
		blocked                bool
	}{
		{name: "wide numeric magnitude", columnDDL: "NUMERIC(20,8)", value: "12345.12345678", blocked: true},
		{name: "numeric fraction below float64 resolution", columnDDL: "NUMERIC(38,20)", value: "1.00000000000000000001", blocked: true},
		{name: "null numeric cannot become zero", columnDDL: "NUMERIC(10,6)", value: "NULL", blocked: true},
		{name: "NaN numeric", columnDDL: "NUMERIC(10,6)", value: "'NaN'", blocked: true},
		{name: "wide numeric exact value", columnDDL: "NUMERIC(20,8)", value: "9.99000000"},
		{name: "legacy double exact value", columnDDL: "DOUBLE PRECISION", value: "9.99"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, db.Exec("CREATE TABLE subscription_plans (id BIGINT PRIMARY KEY, title VARCHAR(128) NOT NULL, price_amount "+test.columnDDL+" NULL DEFAULT NULL)").Error)
			t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE subscription_plans").Error) })
			// Values are closed test cases so SQL NULL and NaN can be represented.
			require.NoError(t, db.Exec("INSERT INTO subscription_plans VALUES (7, 'preserve-title', "+test.value+")").Error)
			beforeSchema := postgresPriceMigrationColumns(t, db)
			var beforeValue string
			require.NoError(t, db.Raw("SELECT COALESCE(price_amount::text, 'NULL') FROM subscription_plans WHERE id = 7").Scan(&beforeValue).Error)
			migrationErr := migrateSubscriptionPlanPriceAmount()
			if migrationErr == nil {
				autoErr := db.AutoMigrate(&SubscriptionPlan{})
				if !test.blocked {
					require.NoError(t, autoErr)
				}
			}
			var afterValue, title string
			require.NoError(t, db.Raw("SELECT COALESCE(price_amount::text, 'NULL') FROM subscription_plans WHERE id = 7").Scan(&afterValue).Error)
			require.NoError(t, db.Raw("SELECT title FROM subscription_plans WHERE id = 7").Scan(&title).Error)
			assert.Equal(t, "preserve-title", title)
			if test.blocked {
				assert.Error(t, migrationErr)
				assert.Equal(t, beforeSchema, postgresPriceMigrationColumns(t, db))
				assert.Equal(t, beforeValue, afterValue)
				for _, migrate := range []func() error{migrateDB, migrateDBFast} {
					require.ErrorContains(t, migrate(), "subscription_plans.price_amount")
					assert.Equal(t, beforeSchema, postgresPriceMigrationColumns(t, db))
				}
				var tables int64
				require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')").Scan(&tables).Error)
				assert.EqualValues(t, 1, tables, "startup must stop before unrelated AutoMigrate runs")
				return
			}
			require.NoError(t, migrationErr)
			beforeAmount, err := decimal.NewFromString(beforeValue)
			require.NoError(t, err)
			afterAmount, err := decimal.NewFromString(afterValue)
			require.NoError(t, err)
			assert.True(t, beforeAmount.Equal(afterAmount), "%s must equal %s exactly", beforeValue, afterValue)
			upgradedSchema := postgresPriceMigrationColumns(t, db)
			require.NoError(t, migrateSubscriptionPlanPriceAmount())
			require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
			assert.Equal(t, upgradedSchema, postgresPriceMigrationColumns(t, db))
		})
	}
}

func postgresPriceMigrationColumns(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var columns string
	require.NoError(t, db.Raw(`SELECT string_agg(column_name || ':' || data_type || ':' || is_nullable || ':' ||
		COALESCE(column_default, '') || ':' || COALESCE(numeric_precision::text, '') || ':' ||
		COALESCE(numeric_scale::text, ''), ',' ORDER BY ordinal_position)
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'subscription_plans'`).Scan(&columns).Error)
	return columns
}
