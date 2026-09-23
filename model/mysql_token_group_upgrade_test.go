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

// Existing unassigned keys must survive an application upgrade unchanged so their
// owners can repair the group in place without replacing credentials or balances.
// This integration test only accepts a disposable database, never an application DB.
func TestMySQLTokenGroupUpgradePreservesExistingKeys(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_TOKEN_GROUP_UPGRADE_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_TOKEN_GROUP_UPGRADE_DSN to a dedicated lemonhub_token_group_upgrade_test_* database")
	}
	config, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	databasePattern := regexp.MustCompile(`^lemonhub_token_group_upgrade_test_[a-z0-9_]+$`)
	require.Regexp(t, databasePattern, config.DBName)
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(true))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Regexp(t, databasePattern, selectedDatabase)
	var existingTables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()").Scan(&existingTables).Error)
	require.Zero(t, existingTables, "refusing to overwrite an existing fixture; provide a fresh empty disposable database")

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
	initCol()
	t.Cleanup(func() {
		rc25DropAllMySQLTablesBestEffort(db)
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
	})

	require.NoError(t, db.AutoMigrate(&mysqlLegacyIntToken{}))
	legacy := []mysqlLegacyIntToken{
		{Id: 1, UserId: 11, Key: "upgrade-empty-group", Name: "blank group", Group: "", RemainQuota: 123456, UsedQuota: 789, Status: 1},
		{Id: 2, UserId: 11, Key: "upgrade-blank-auto", Name: "blank priority group", Group: "auto", AutoGroups: `[""]`, RemainQuota: 123457, UsedQuota: 790, Status: 1},
		{Id: 3, UserId: 12, SiteId: 7, Key: "upgrade-whitespace-group", Name: "disabled legacy key", Group: " ", AutoGroups: `[" "]`, RemainQuota: 123458, UsedQuota: 791, Status: 2},
		{Id: 4, UserId: 12, SiteId: 7, Key: "upgrade-explicit-groups", Name: "valid key", Group: "auto", AutoGroups: `["premium","default"]`, RemainQuota: 123459, UsedQuota: 792, Status: 1},
	}
	require.NoError(t, db.Create(&legacy).Error)
	for pass := 1; pass <= 2; pass++ {
		require.NoError(t, migrateDB(), "application startup migration pass %d", pass)
		var migrated []Token
		require.NoError(t, db.Unscoped().Order("id").Find(&migrated).Error)
		require.Len(t, migrated, len(legacy))
		for i, expected := range legacy {
			actual := migrated[i]
			assert.Equal(t, expected.Id, actual.Id)
			assert.Equal(t, expected.UserId, actual.UserId)
			assert.Equal(t, expected.SiteId, actual.SiteId)
			assert.Equal(t, expected.Key, actual.Key, "startup must not rotate existing credentials")
			assert.Equal(t, expected.Name, actual.Name)
			assert.Equal(t, expected.Status, actual.Status, "startup must not enable or revoke existing keys")
			assert.Equal(t, expected.Group, actual.Group, "group remediation requires an explicit owner choice")
			assert.Equal(t, expected.AutoGroups, actual.AutoGroups)
			assert.Equal(t, int(expected.RemainQuota), actual.RemainQuota)
			assert.Equal(t, int(expected.UsedQuota), actual.UsedQuota)
			assert.False(t, actual.DeletedAt.Valid)
		}
	}
}
