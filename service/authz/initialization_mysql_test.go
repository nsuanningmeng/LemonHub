package authz

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This opt-in proof may only run against a newly created, empty owned schema.
// The independent database owner provisions and removes that schema/server.
func TestAuthzInitializationAtomicMySQL(t *testing.T) {
	dsn := os.Getenv("B23_AUTHZ_MYSQL_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL schema was not supplied")
	}
	require.Equal(t, "1", os.Getenv("B23_AUTHZ_MYSQL_ALLOW_SCHEMA"))
	config, err := mysqlDriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(config.DBName, "b23_authz_"), "refuse an unowned schema")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var actual string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&actual).Error)
	require.Equal(t, config.DBName, actual)
	var tables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error)
	require.Zero(t, tables, "schema must be empty before any migration or mutation")
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}))
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = wasMaster })
	runAuthzAtomicFailures(t, func(t *testing.T) *gorm.DB {
		require.NoError(t, db.Where("1 = 1").Delete(&model.CasbinRule{}).Error)
		require.NoError(t, db.Where("1 = 1").Delete(&model.AuthzRole{}).Error)
		return db
	})
	require.NoError(t, SetUserPermissions(43, PermissionsMap{ResourceChannel: {ActionSensitiveWrite: true}}))
	require.NoError(t, ReloadPolicy())
	require.True(t, Can(43, common.RoleAdminUser, ChannelSensitiveWrite))
}
