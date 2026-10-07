package model

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This test touches only an empty, explicitly named disposable schema. The
// trigger rejects a real server write after an earlier batch write succeeded.
func TestMySQLQuotaDataFlushDurability(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_QUOTA_DATA_DURABILITY_DSN"))
	if dsn == "" {
		t.Skip("set MYSQL_QUOTA_DATA_DURABILITY_DSN to an empty lemonhub_quota_data_durability_test_* database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	allowed := regexp.MustCompile(`^lemonhub_quota_data_durability_test_[a-z0-9_]+$`)
	require.Regexp(t, allowed, config.DBName, "refusing a non-test database")
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	var selected string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selected).Error)
	require.Equal(t, config.DBName, selected)
	var tableCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tableCount).Error)
	require.Zero(t, tableCount, "refusing to alter a nonempty schema")
	previousDB := DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeMySQL)
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(&QuotaData{}))
		DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	resetQuotaDataDurabilityState(t)
	require.NoError(t, db.AutoMigrate(&QuotaData{}))
	for _, operation := range []string{"INSERT", "UPDATE"} {
		t.Run(operation, func(t *testing.T) {
			for _, id := range []int{8201, 8202} {
				LogQuotaData(QuotaDataLogParams{UserID: id, Username: "native-durability", CreatedAt: 3600, Quota: 7, TokenUsed: 5000000000})
			}
			require.NoError(t, db.Exec("CREATE TRIGGER quota_data_reject BEFORE "+operation+" ON quota_data FOR EACH ROW BEGIN IF NEW.user_id = 8202 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'controlled dashboard write rejection'; END IF; END").Error)
			t.Cleanup(func() { assert.NoError(t, db.Exec("DROP TRIGGER IF EXISTS quota_data_reject").Error) })
			require.ErrorContains(t, SaveQuotaDataCache(), "controlled dashboard write rejection")
			var rows []QuotaData
			require.NoError(t, db.Order("user_id").Find(&rows).Error)
			if operation == "INSERT" {
				assert.Empty(t, rows, "first INSERT must roll back when second server write fails")
			} else {
				require.Len(t, rows, 2)
				for _, row := range rows {
					assert.Equal(t, 1, row.Count)
					assert.Equal(t, 7, row.Quota)
					assert.Equal(t, int64(5000000000), row.TokenUsed)
				}
			}
			CacheQuotaDataLock.Lock()
			assert.Len(t, CacheQuotaData, 2)
			CacheQuotaDataLock.Unlock()
			require.NoError(t, db.Exec("DROP TRIGGER quota_data_reject").Error)
			require.NoError(t, SaveQuotaDataCache())
			require.NoError(t, SaveQuotaDataCache())
			require.NoError(t, db.Order("user_id").Find(&rows).Error)
			require.Len(t, rows, 2)
			multiplier := 1
			if operation == "UPDATE" {
				multiplier = 2
			}
			for _, row := range rows {
				assert.Equal(t, multiplier, row.Count)
				assert.Equal(t, 7*multiplier, row.Quota)
				assert.Equal(t, int64(5000000000)*int64(multiplier), row.TokenUsed)
			}
		})
	}
	t.Run("nullable legacy active and untouched rows", func(t *testing.T) {
		require.NoError(t, db.Exec("UPDATE quota_data SET count = NULL, token_used = NULL WHERE user_id = 8201").Error)
		require.NoError(t, db.Exec("UPDATE quota_data SET count = NULL, quota = NULL, token_used = NULL WHERE user_id = 8202").Error)
		LogQuotaData(QuotaDataLogParams{UserID: 8201, Username: "native-durability", CreatedAt: 3600, Quota: 7, TokenUsed: 5000000000})
		require.NoError(t, SaveQuotaDataCache())
		var active QuotaData
		require.NoError(t, db.Where("user_id = ?", 8201).First(&active).Error)
		assert.Equal(t, 1, active.Count)
		assert.Equal(t, 21, active.Quota)
		assert.Equal(t, int64(5000000000), active.TokenUsed)
		var idle struct {
			Count     *int
			Quota     *int
			TokenUsed *int64
		}
		require.NoError(t, db.Table("quota_data").Where("user_id = ?", 8202).First(&idle).Error)
		assert.Nil(t, idle.Count)
		assert.Nil(t, idle.Quota)
		assert.Nil(t, idle.TokenUsed)
	})
}
