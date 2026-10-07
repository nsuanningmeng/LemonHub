package model

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests require an empty, explicitly named disposable database. They do
// not use truncateTables or alter the package's ordinary SQLite fixture.
func TestSubscriptionSettlementDatabases(t *testing.T) {
	for _, backend := range []struct {
		kind common.DatabaseType
		env  string
	}{
		{kind: common.DatabaseTypeMySQL, env: "MYSQL_SUBSCRIPTION_SETTLEMENT_DSN"},
		{kind: common.DatabaseTypePostgreSQL, env: "POSTGRES_SUBSCRIPTION_SETTLEMENT_DSN"},
	} {
		t.Run(string(backend.kind), func(t *testing.T) {
			openSubscriptionSettlementTestDatabase(t, backend.kind, backend.env)

			t.Run("same request settles once across connections", func(t *testing.T) {
				fixture := newDatabaseSubscriptionSettlementFixture(t, 60)
				receipts := runConcurrentSubscriptionSettlements(t, fixture.params, fixture.params)
				assert.Equal(t, receipts[0], receipts[1])
				assert.EqualValues(t, 40, receipts[0].SubscriptionDelta)
				assert.EqualValues(t, 60, receipts[0].WalletDelta)
				assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
				var records int64
				require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).
					Where("request_id = ? AND status = ?", fixture.params.RequestId, "settled").Count(&records).Error)
				assert.EqualValues(t, 1, records)
			})

			t.Run("different requests share the remaining subscription quota", func(t *testing.T) {
				fixture := newDatabaseSubscriptionSettlementFixture(t, 30)
				second := fixture.params
				second.RequestId = "database-second-" + common.GetUUID()
				_, err := PreConsumeUserSubscription(second.RequestId, fixture.user.Id, "test-model", 0, 30)
				require.NoError(t, err)
				require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).
					Updates(map[string]interface{}{"remain_quota": 940, "used_quota": 60}).Error)
				fixture.params.ActualQuota, second.ActualQuota = 100, 100
				receipts := runConcurrentSubscriptionSettlements(t, fixture.params, second)
				assert.EqualValues(t, 40, receipts[0].SubscriptionDelta+receipts[1].SubscriptionDelta)
				assert.EqualValues(t, 100, receipts[0].WalletDelta+receipts[1].WalletDelta)
				assertSubscriptionSettlementBalances(t, fixture, 100, 900, 800, 200)
			})

			t.Run("every write failure rolls back the whole receipt", func(t *testing.T) {
				for _, table := range []string{"user_subscriptions", "users", "tokens", "subscription_pre_consume_records"} {
					t.Run(table, func(t *testing.T) {
						fixture := newDatabaseSubscriptionSettlementFixture(t, 60)
						forcedErr := errors.New("database settlement write rejected")
						callbackName := "test:database_subscription_write:" + common.GetUUID()
						require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
							if tx.Statement != nil && tx.Statement.Table == table {
								tx.AddError(forcedErr)
							}
						}))
						t.Cleanup(func() { DB.Callback().Update().Remove(callbackName) })
						_, err := SettleSubscriptionBilling(fixture.params)
						require.ErrorIs(t, err, forcedErr)
						assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
						var record SubscriptionPreConsumeRecord
						require.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error)
						assert.Equal(t, "consumed", record.Status)
						require.NoError(t, DB.Callback().Update().Remove(callbackName))
						_, err = SettleSubscriptionBilling(fixture.params)
						require.NoError(t, err, "a rolled-back request remains retryable")
						assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
					})
				}
			})

			t.Run("negative wallet and wide token balances stay exact", func(t *testing.T) {
				fixture := newDatabaseSubscriptionSettlementFixture(t, 60)
				const accumulated = 1 << 32
				require.NoError(t, DB.Model(&User{}).Where("id = ?", fixture.user.Id).Update("quota", 20).Error)
				require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).
					Updates(map[string]interface{}{"remain_quota": accumulated + 940, "used_quota": accumulated + 60}).Error)
				receipt, err := SettleSubscriptionBilling(fixture.params)
				require.NoError(t, err)
				assert.Equal(t, 100, receipt.TokenDelta)
				assertSubscriptionSettlementBalances(t, fixture, 100, -40, accumulated+840, accumulated+160)
				repeated, err := SettleSubscriptionBilling(fixture.params)
				require.NoError(t, err)
				assert.Equal(t, receipt, repeated)
				assertSubscriptionSettlementBalances(t, fixture, 100, -40, accumulated+840, accumulated+160)
			})
		})
	}
}

func openSubscriptionSettlementTestDatabase(t *testing.T, kind common.DatabaseType, envName string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(envName))
	if dsn == "" {
		t.Skipf("set %s to an empty lemonhub_subscription_settlement_test_* database", envName)
	}
	allowedName := regexp.MustCompile(`^lemonhub_subscription_(receipt|settlement)_test_[a-z0-9_]+$`)
	var dialector gorm.Dialector
	var expectedDatabase, databaseQuery, tablesQuery string
	switch kind {
	case common.DatabaseTypeMySQL:
		config, err := mysqldriver.ParseDSN(dsn)
		require.NoError(t, err)
		require.Regexp(t, allowedName, config.DBName, "refusing to modify a database outside the dedicated test namespace")
		config.ParseTime = true
		dialector = gormmysql.Open(config.FormatDSN())
		expectedDatabase, databaseQuery = config.DBName, "SELECT DATABASE()"
		tablesQuery = "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()"
	case common.DatabaseTypePostgreSQL:
		config, err := pgx.ParseConfig(dsn)
		require.NoError(t, err)
		require.Regexp(t, allowedName, config.Database, "refusing to modify a database outside the dedicated test namespace")
		dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		expectedDatabase, databaseQuery = config.Database, "SELECT current_database()"
		tablesQuery = "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')"
	default:
		require.FailNow(t, "unsupported settlement test database")
	}
	db, err := gorm.Open(dialector, newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	// Separate connections exercise server row locks instead of serializing all
	// transactions through the ordinary single-connection SQLite test pool.
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	var selectedDatabase string
	require.NoError(t, db.Raw(databaseQuery).Scan(&selectedDatabase).Error)
	require.Equal(t, expectedDatabase, selectedDatabase)
	var existingTables int64
	require.NoError(t, db.Raw(tablesQuery).Scan(&existingTables).Error)
	require.Zero(t, existingTables, "refusing to modify a nonempty database")

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(kind, kind)
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch
		initCol()
	})
	tables := []interface{}{&SubscriptionPreConsumeRecord{}, &UserSubscription{}, &SubscriptionPlan{}, &Token{}, &User{}}
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(tables...)) })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}))
}

func newDatabaseSubscriptionSettlementFixture(t *testing.T, preConsumed int) subscriptionSettlementFixture {
	t.Helper()
	user := createReserveTestUser(t, 1000)
	user.SiteId = 17
	require.NoError(t, DB.Model(&user).Update("site_id", user.SiteId).Error)
	plan := SubscriptionPlan{
		Title: "database-settlement-plan", Enabled: true, TotalAmount: 100,
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() { InvalidateSubscriptionPlanCache(plan.Id) })
	now := GetDBTimestamp()
	sub := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: 100, AllowWalletOverflow: true,
		Status: "active", StartTime: now - 60, EndTime: now + 3600,
	}
	require.NoError(t, DB.Create(&sub).Error)
	requestID := "database-settlement-" + common.GetUUID()
	result, err := PreConsumeUserSubscription(requestID, user.Id, "test-model", 0, int64(preConsumed))
	require.NoError(t, err)
	require.Equal(t, sub.Id, result.UserSubscriptionId)
	token := Token{
		UserId: user.Id, SiteId: user.SiteId, Key: "database-settlement-token-" + common.GetUUID(),
		Status: common.TokenStatusEnabled, ExpiredTime: -1,
		RemainQuota: 1000 - preConsumed, UsedQuota: preConsumed,
	}
	require.NoError(t, DB.Create(&token).Error)
	return subscriptionSettlementFixture{
		user: user, token: token, sub: sub,
		params: SubscriptionBillingParams{
			RequestId: requestID, UserId: user.Id, SubscriptionId: sub.Id,
			TokenId: token.Id, TokenKey: token.Key,
			PreConsumedQuota: preConsumed, TokenConsumedQuota: preConsumed,
			ActualQuota: 160, AllowWalletOverflow: true,
		},
	}
}

func runConcurrentSubscriptionSettlements(t *testing.T, first, second SubscriptionBillingParams) []*SubscriptionBillingReceipt {
	t.Helper()
	start := make(chan struct{})
	var ready, workers sync.WaitGroup
	ready.Add(2)
	workers.Add(2)
	receipts := make([]*SubscriptionBillingReceipt, 2)
	errors := make([]error, 2)
	for index, params := range []SubscriptionBillingParams{first, second} {
		go func(index int, params SubscriptionBillingParams) {
			defer workers.Done()
			ready.Done()
			<-start
			receipts[index], errors[index] = SettleSubscriptionBilling(params)
		}(index, params)
	}
	ready.Wait()
	close(start)
	workers.Wait()
	for index := range receipts {
		require.NoError(t, errors[index])
		require.NotNil(t, receipts[index])
	}
	return receipts
}
