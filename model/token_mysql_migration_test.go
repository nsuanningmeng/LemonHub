package model

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type mysqlLegacyIntToken struct {
	Id                 int    `gorm:"primaryKey"`
	SiteId             int    `gorm:"type:int;default:0;index"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"type:varchar(128);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int32  `gorm:"type:int;default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int32   `gorm:"type:int;default:0"`
	Group              string  `gorm:"default:''"`
	CrossGroupRetry    bool
	AutoGroups         string         `gorm:"type:text"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (mysqlLegacyIntToken) TableName() string { return "tokens" }

var mysqlTokenQuotaMigrationDatabaseName = regexp.MustCompile(`^lemonhub_token_quota_migration_test_[a-z0-9_]+$`)

func TestMySQLTokenQuotaMigrationPreservesData(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large token quotas require a 64-bit server build")
	}

	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_TOKEN_MIGRATION_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_TOKEN_MIGRATION_DSN to run the MySQL token quota migration test")
	}

	dsnConfig, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexpf(t, mysqlTokenQuotaMigrationDatabaseName, strings.ToLower(dsnConfig.DBName),
		"refusing destructive token migration test against database %q: its name must match lemonhub_token_quota_migration_test_*", dsnConfig.DBName)
	dsnConfig.ParseTime = true

	mdb, err := gorm.Open(gormmysql.Open(dsnConfig.FormatDSN()), newGormConfig(true))
	require.NoError(t, err)
	sqlDB, err := mdb.DB()
	require.NoError(t, err)

	var selectedDatabase string
	require.NoError(t, mdb.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Regexpf(t, mysqlTokenQuotaMigrationDatabaseName, strings.ToLower(selectedDatabase),
		"refusing destructive token migration test against selected database %q", selectedDatabase)

	t.Cleanup(func() {
		_ = mdb.Migrator().DropTable("tokens")
		_ = sqlDB.Close()
	})
	require.NoError(t, mdb.Migrator().DropTable("tokens"))
	require.NoError(t, mdb.AutoMigrate(&mysqlLegacyIntToken{}))

	allowIPs := "127.0.0.1\n10.0.0.0/8"
	legacy := mysqlLegacyIntToken{
		Id: 401, SiteId: 7, UserId: 101, Key: "sk-token-quota-migration-sentinel",
		Status: 1, Name: "legacy-int-token", CreatedTime: 1_700_050_000,
		AccessedTime: 1_700_050_100, ExpiredTime: -1,
		RemainQuota: 2_147_483_647, UsedQuota: 1_234_567_890,
		ModelLimitsEnabled: true, ModelLimits: "gpt-4o,gpt-4.1", AllowIps: &allowIPs,
		Group: "legacy-premium", CrossGroupRetry: true, AutoGroups: `["legacy-premium","default"]`,
	}
	require.NoError(t, mdb.Create(&legacy).Error)
	require.Equal(t, "int", mysqlTokenQuotaDataType(t, mdb, "remain_quota"))
	require.Equal(t, "int", mysqlTokenQuotaDataType(t, mdb, "used_quota"))

	var beforeCount int64
	require.NoError(t, mdb.Table("tokens").Count(&beforeCount).Error)
	require.Equal(t, int64(1), beforeCount)

	require.NoError(t, mdb.AutoMigrate(&Token{}), "INT to BIGINT token quota migration must succeed")
	assert.Equal(t, "bigint", mysqlTokenQuotaDataType(t, mdb, "remain_quota"))
	assert.Equal(t, "bigint", mysqlTokenQuotaDataType(t, mdb, "used_quota"))

	var afterCount int64
	require.NoError(t, mdb.Table("tokens").Count(&afterCount).Error)
	assert.Equal(t, beforeCount, afterCount, "quota widening must not add or remove token rows")

	var migrated Token
	require.NoError(t, mdb.Unscoped().First(&migrated, legacy.Id).Error)
	assert.Equal(t, legacy.SiteId, migrated.SiteId)
	assert.Equal(t, legacy.UserId, migrated.UserId)
	assert.Equal(t, legacy.Key, migrated.Key)
	assert.Equal(t, legacy.Status, migrated.Status)
	assert.Equal(t, legacy.Name, migrated.Name)
	assert.Equal(t, legacy.CreatedTime, migrated.CreatedTime)
	assert.Equal(t, legacy.AccessedTime, migrated.AccessedTime)
	assert.Equal(t, legacy.ExpiredTime, migrated.ExpiredTime)
	assert.Equal(t, int(legacy.RemainQuota), migrated.RemainQuota)
	assert.Equal(t, int(legacy.UsedQuota), migrated.UsedQuota)
	assert.Equal(t, legacy.UnlimitedQuota, migrated.UnlimitedQuota)
	assert.Equal(t, legacy.ModelLimitsEnabled, migrated.ModelLimitsEnabled)
	assert.Equal(t, legacy.ModelLimits, migrated.ModelLimits)
	assert.Equal(t, legacy.Group, migrated.Group)
	assert.Equal(t, legacy.CrossGroupRetry, migrated.CrossGroupRetry)
	assert.Equal(t, legacy.AutoGroups, migrated.AutoGroups)
	assert.False(t, migrated.DeletedAt.Valid)
	require.NotNil(t, migrated.AllowIps)
	assert.Equal(t, allowIPs, *migrated.AllowIps)

	largeRemain := int64(5_000_000_000)
	largeUsed := int64(4_000_000_000)
	require.NoError(t, mdb.Model(&Token{}).Where("id = ?", legacy.Id).Updates(map[string]interface{}{
		"remain_quota": largeRemain,
		"used_quota":   largeUsed,
	}).Error)

	var widenedValues struct {
		RemainQuota int64 `gorm:"column:remain_quota"`
		UsedQuota   int64 `gorm:"column:used_quota"`
	}
	require.NoError(t, mdb.Table("tokens").Select("remain_quota, used_quota").Where("id = ?", legacy.Id).Scan(&widenedValues).Error)
	assert.Equal(t, largeRemain, widenedValues.RemainQuota)
	assert.Equal(t, largeUsed, widenedValues.UsedQuota)
	var widenedToken Token
	require.NoError(t, mdb.First(&widenedToken, legacy.Id).Error)
	assert.Equal(t, int(largeRemain), widenedToken.RemainQuota)
	assert.Equal(t, int(largeUsed), widenedToken.UsedQuota)

	createBefore := mysqlShowCreate(t, mdb, "tokens")
	require.NoError(t, mdb.AutoMigrate(&Token{}), "token quota migration must be safe to run twice")
	createAfter := mysqlShowCreate(t, mdb, "tokens")
	assert.Equal(t, createBefore, createAfter, "second migration must not churn the tokens schema")

	var finalCount int64
	require.NoError(t, mdb.Table("tokens").Count(&finalCount).Error)
	assert.Equal(t, beforeCount, finalCount)
	require.NoError(t, mdb.Table("tokens").Select("remain_quota, used_quota").Where("id = ?", legacy.Id).Scan(&widenedValues).Error)
	assert.Equal(t, largeRemain, widenedValues.RemainQuota)
	assert.Equal(t, largeUsed, widenedValues.UsedQuota)
}

func mysqlTokenQuotaDataType(t *testing.T, db *gorm.DB, column string) string {
	t.Helper()
	var dataType string
	require.NoError(t, db.Raw(
		"SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tokens' AND COLUMN_NAME = ?",
		column,
	).Scan(&dataType).Error)
	require.NotEmpty(t, dataType, "tokens.%s must exist", column)
	return strings.ToLower(dataType)
}
