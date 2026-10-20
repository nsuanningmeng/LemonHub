package model

import (
	"maps"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Uses two real connections: the first represents another application node
// holding a configuration transaction; the second exercises UpdateOption.
// This bypasses the local writer mutex without weakening production locking.
func TestMySQLNotificationConfigSerializesAcrossNodes(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_NOTIFICATION_CONFIG_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_NOTIFICATION_CONFIG_DSN to an empty lemonhub_notification_config_test_* database")
	}
	config, err := mysqlDriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^lemonhub_notification_config_test_[a-z0-9_]+$`), config.DBName)
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(3)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var selected string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selected).Error)
	require.Equal(t, config.DBName, selected)
	var tables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error)
	require.Zero(t, tables, "refusing to modify a nonempty schema")
	require.NoError(t, db.AutoMigrate(&Option{}))

	oldDB, oldType, oldOptions := DB, common.MainDatabaseType(), common.OptionMap
	oldUnsubscribe, oldMedia := common.UnsubscribeSecret, common.MediaSigningSecret
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeMySQL)
	t.Cleanup(func() {
		DB, common.OptionMap = oldDB, oldOptions
		common.SetMainDatabaseType(oldType)
		common.UnsubscribeSecret, common.MediaSigningSecret = oldUnsubscribe, oldMedia
	})
	require.NoError(t, InitOptionMap())
	for _, existing := range []bool{false, true} {
		name := "new bundle"
		if existing {
			name = "existing bundle"
		}
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.Where(map[string]any{"key": []string{"EmailProvider", "ResendApiKey", "ResendFrom"}}).Delete(&Option{}).Error)
			require.NoError(t, InitOptionMap())
			if existing {
				require.NoError(t, UpdateOptionsBulk(map[string]string{
					"EmailProvider": "smtp", "ResendApiKey": "re_old", "ResendFrom": "old@example.com",
				}))
			}
			staleNode := maps.Clone(common.OptionMap)

			otherNode := db.Begin()
			require.NoError(t, otherNode.Error)
			t.Cleanup(func() { _ = otherNode.Rollback() })
			for _, option := range []Option{
				{Key: "EmailProvider", Value: "resend"},
				{Key: "ResendApiKey", Value: "re_other_node"},
				{Key: "ResendFrom", Value: "other@example.com"},
			} {
				if existing {
					require.NoError(t, otherNode.Model(&Option{}).Where(&Option{Key: option.Key}).Update("value", option.Value).Error)
				} else {
					require.NoError(t, otherNode.Create(&option).Error)
				}
			}
			// An independent connection must wait for the other node's row
			// lock, even though this process still has SMTP in its cache.
			// The server's lock timeout proves the DB-level serialization.
			require.NoError(t, db.Connection(func(conn *gorm.DB) error {
				require.NoError(t, conn.Exec("SET SESSION innodb_lock_wait_timeout = 1").Error)
				DB = conn
				defer func() {
					DB = db
					require.NoError(t, conn.Exec("SET SESSION innodb_lock_wait_timeout = DEFAULT").Error)
				}()
				err := UpdateOption("ResendApiKey", "")
				var lockErr *mysqlDriver.MySQLError
				require.ErrorAs(t, err, &lockErr)
				assert.EqualValues(t, 1205, lockErr.Number)
				return nil
			}))
			assert.Equal(t, staleNode, common.OptionMap, "a rolled-back write must not publish")
			require.NoError(t, otherNode.Commit().Error)
			require.ErrorContains(t, UpdateOption("ResendApiKey", ""), "Resend API key")
			assert.Equal(t, "re_other_node", requireOptionValue(t, db, "ResendApiKey"))
			assert.Equal(t, "resend", requireOptionValue(t, db, "EmailProvider"))
			require.NoError(t, InitOptionMap(), "the committed bundle must survive restart")
		})
	}

	t.Run("activation with stale cache", func(t *testing.T) {
		require.NoError(t, UpdateOptionsBulk(map[string]string{
			"EmailProvider": "smtp", "ResendApiKey": "re_existing", "ResendFrom": "existing@example.com",
		}))
		common.OptionMap = map[string]string{"EmailProvider": "smtp", "ResendApiKey": "", "ResendFrom": ""}
		require.NoError(t, UpdateOption("EmailProvider", "resend"))
		assert.Equal(t, "resend", common.OptionMap["EmailProvider"])
		assert.Equal(t, "re_existing", common.OptionMap["ResendApiKey"])
		assert.Equal(t, "existing@example.com", common.OptionMap["ResendFrom"])
		require.NoError(t, InitOptionMap())
	})
}
