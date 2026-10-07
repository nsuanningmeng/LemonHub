package model

import (
	"math"
	"net/url"
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
	"gorm.io/driver/clickhouse"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Keep the pre-input_tokens_total schema independent of Log. This also models
// writes from an old application that does not include the new nullable column.
type legacyInputTokenLog struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	SiteId            int    `json:"site_id" gorm:"type:int;default:0;index"` // white-label sub-site (0 = main site)
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

func (legacyInputTokenLog) TableName() string { return "logs" }

func TestLogInputTokensMigrationPreservesLegacyRows(t *testing.T) {
	for _, backend := range []struct {
		name, env    string
		databaseType common.DatabaseType
	}{
		{name: "sqlite", databaseType: common.DatabaseTypeSQLite},
		{name: "mysql", env: "MYSQL_LOG_INPUT_MIGRATION_DSN", databaseType: common.DatabaseTypeMySQL},
		{name: "postgres", env: "POSTGRES_LOG_INPUT_MIGRATION_DSN", databaseType: common.DatabaseTypePostgreSQL},
		{name: "clickhouse", env: "CLICKHOUSE_LOG_INPUT_MIGRATION_DSN", databaseType: common.DatabaseTypeClickHouse},
	} {
		t.Run(backend.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(backend.env))
			if backend.env != "" && dsn == "" {
				t.Skip("set " + backend.env + " to a fresh lemonhub_log_input_test_* database")
			}
			modes := []string{"main", "separate_log"}
			if backend.databaseType == common.DatabaseTypeClickHouse {
				modes = []string{"separate_log"}
			}
			for _, mode := range modes {
				t.Run(mode, func(t *testing.T) {
					var dialector gorm.Dialector
					var expectedDatabase string
					switch backend.name {
					case "sqlite":
						dialector = sqlite.Open(filepath.Join(t.TempDir(), "logs.db"))
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
					case "clickhouse":
						config, err := url.Parse(dsn)
						require.NoError(t, err)
						expectedDatabase = strings.TrimPrefix(config.Path, "/")
						dialector = clickhouse.Open(dsn)
					}
					if backend.env != "" {
						require.Regexp(t, regexp.MustCompile(`^lemonhub_log_input_test_[a-z0-9_]+$`), expectedDatabase, "refusing fixture outside a disposable database")
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
						if backend.name == "clickhouse" {
							query = "SELECT currentDatabase()"
						}
						var actualDatabase string
						require.NoError(t, db.Raw(query).Scan(&actualDatabase).Error)
						require.Equal(t, expectedDatabase, actualDatabase)
					}
					existingTables, err := logInputMigrationTables(db, backend.databaseType)
					require.NoError(t, err)
					require.Empty(t, existingTables, "refusing to change a nonempty fixture database")
					t.Cleanup(func() {
						tables, err := logInputMigrationTables(db, backend.databaseType)
						require.NoError(t, err)
						for _, table := range tables {
							require.NoError(t, db.Migrator().DropTable(table))
						}
					})
					previousDB, previousLogDB := DB, LOG_DB
					previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
					DB, LOG_DB = db, db
					common.SetDatabaseTypes(backend.databaseType, backend.databaseType)
					if mode == "separate_log" {
						// A different primary connection and dialect detects migrations that
						// accidentally write through DB instead of the configured LOG_DB.
						DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "primary.db")), newGormConfig(false))
						require.NoError(t, err)
						primarySQL, err := DB.DB()
						require.NoError(t, err)
						t.Cleanup(func() { _ = primarySQL.Close() })
						common.SetMainDatabaseType(common.DatabaseTypeSQLite)
					}
					initCol()
					t.Cleanup(func() {
						DB, LOG_DB = previousDB, previousLogDB
						common.SetDatabaseTypes(previousMainType, previousLogType)
						initCol()
					})
					t.Setenv("LOG_SQL_CLICKHOUSE_TTL_DAYS", "0")
					if backend.name == "clickhouse" {
						require.NoError(t, db.Exec(legacyInputTokenClickHouseDDL).Error)
					} else {
						require.NoError(t, db.AutoMigrate(&legacyInputTokenLog{}))
					}
					require.False(t, db.Migrator().HasColumn(&legacyInputTokenLog{}, "input_tokens_total"))
					legacy := []legacyInputTokenLog{
						{Id: 701, SiteId: 7, UserId: 101, CreatedAt: 1_900_000_000, Type: LogTypeConsume, Content: "legacy 中文😀", Username: "log-audit", TokenName: "key", ModelName: "cache-model", Quota: 31, PromptTokens: 11, CompletionTokens: 3, UseTime: 2, IsStream: true, ChannelId: 4, TokenId: 5, Group: "premium", Ip: "127.0.0.1", RequestId: "legacy-a", UpstreamRequestId: "up-a", Other: `{"cache_tokens":7,"cache_creation_tokens":9,"marker":"中文😀"}`},
						{Id: 702, SiteId: 7, UserId: 101, CreatedAt: 1_900_000_001, Type: LogTypeConsume, Content: "raw B", Username: "log-audit", TokenName: "key", ModelName: "cache-model", Quota: 37, PromptTokens: 17, CompletionTokens: 5, RequestId: "legacy-b", Other: `{"cache_tokens":2}`},
					}
					// ClickHouse's native driver sends each batch synchronously.
					require.NoError(t, db.Create(&legacy).Error)
					var legacyRaw []map[string]interface{}
					require.NoError(t, db.Table("logs").Order("id").Find(&legacyRaw).Error)
					for startup := 1; startup <= 2; startup++ {
						if mode == "main" {
							require.NoError(t, migrateDB(), "full main migration %d", startup)
						} else {
							require.NoError(t, migrateLOGDB(), "separate log migration %d", startup)
						}
						var actual []legacyInputTokenLog
						require.NoError(t, db.Order("id").Find(&actual).Error)
						assert.Equal(t, legacy, actual, "all legacy log, cache metadata and quota values survive startup")
						var rows []map[string]interface{}
						require.NoError(t, db.Table("logs").Order("id").Find(&rows).Error)
						for _, row := range rows {
							require.Contains(t, row, "input_tokens_total")
							assert.Nil(t, row["input_tokens_total"], "old rows retain NULL fallback")
							delete(row, "input_tokens_total")
						}
						assert.Equal(t, legacyRaw, rows, "old raw fields remain byte-for-byte stable")
						if mode == "separate_log" {
							assert.False(t, DB.Migrator().HasTable("logs"), "log migration must not create tables in primary DB")
						}
					}
					// Old and new binaries can share a migrated log schema. An omitted field
					// stays NULL; an explicit 0 must override a nonzero raw prompt count.
					oldWriter := legacy[0]
					oldWriter.Id = 703
					oldWriter.RequestId = "old-writer"
					oldWriter.PromptTokens = 29
					oldWriter.CompletionTokens = 13
					require.NoError(t, db.Create(&oldWriter).Error)
					zero, wide := int64(0), int64(5_000_000_000)
					newRows := []Log{
						{Id: 704, SiteId: 7, UserId: 101, CreatedAt: 1_900_000_002, Type: LogTypeConsume, Username: "log-audit", PromptTokens: 19, CompletionTokens: 7, InputTokensTotal: &zero, RequestId: "explicit-zero", Other: `{"cache_tokens":19}`},
						{Id: 705, SiteId: 7, UserId: 101, CreatedAt: 1_900_000_003, Type: LogTypeConsume, Username: "log-audit", PromptTokens: 23, CompletionTokens: 11, InputTokensTotal: &wide, RequestId: "wide-inclusive", Other: `{"cache_creation_tokens":4999999977}`},
					}
					require.NoError(t, db.Create(&newRows).Error)
					var current []Log
					require.NoError(t, db.Order("id").Find(&current).Error)
					require.Len(t, current, 5)
					for i := 0; i < 3; i++ {
						assert.Nil(t, current[i].InputTokensTotal)
					}
					require.NotNil(t, current[3].InputTokensTotal)
					assert.Equal(t, int64(0), *current[3].InputTokensTotal)
					require.NotNil(t, current[4].InputTokensTotal)
					assert.Equal(t, wide, *current[4].InputTokensTotal)
					assert.Equal(t, []int{11, 17, 29, 19, 23}, []int{current[0].PromptTokens, current[1].PromptTokens, current[2].PromptTokens, current[3].PromptTokens, current[4].PromptTokens})
					assert.Equal(t, newRows[0].Other, current[3].Other)
					assert.Equal(t, newRows[1].Other, current[4].Other)
					const expectedTokens = int64(5_000_000_096) // 14 + 22 + 42 + 7 + 5,000,000,011
					stat, err := SumUserUsedQuota(101, LogTypeConsume, 0, 0, "", "", 0, "")
					require.NoError(t, err)
					assert.EqualValues(t, expectedTokens, stat.Tpm)
					assert.Equal(t, 5, stat.Rpm)
					assert.EqualValues(t, expectedTokens, SumUsedToken(LogTypeConsume, 0, 0, "", "log-audit", ""))
					var rawAfterWrites []map[string]interface{}
					require.NoError(t, db.Table("logs").Order("id").Find(&rawAfterWrites).Error)
					if mode == "main" {
						require.NoError(t, migrateDB())
					} else {
						require.NoError(t, migrateLOGDB())
					}
					var rawAfterRestart []map[string]interface{}
					require.NoError(t, db.Table("logs").Order("id").Find(&rawAfterRestart).Error)
					assert.Equal(t, rawAfterWrites, rawAfterRestart, "restart retains NULL, valid zero and wide values together")
					// Exercise the real dialect's aggregate arithmetic, including SQLite's
					// integer SUM and cross-sum overflow paths, without rewriting raw usage.
					maximum, nearMaximum, positive, negative := int64(math.MaxInt64), int64(math.MaxInt64-5), int64(123), int64(-50)
					extremeRows := []Log{
						{Id: 801, UserId: 202, CreatedAt: 1_900_000_004, Type: LogTypeConsume, Username: "sum-overflow", InputTokensTotal: &maximum, RequestId: "sum-overflow-a"},
						{Id: 802, UserId: 202, CreatedAt: 1_900_000_004, Type: LogTypeConsume, Username: "sum-overflow", InputTokensTotal: &maximum, RequestId: "sum-overflow-b"},
						{Id: 803, UserId: 203, CreatedAt: 1_900_000_004, Type: LogTypeConsume, Username: "addition-overflow", InputTokensTotal: &nearMaximum, CompletionTokens: 10, RequestId: "addition-overflow"},
						{Id: 804, UserId: 204, CreatedAt: 1_900_000_004, Type: LogTypeConsume, Username: "negative-components", InputTokensTotal: &positive, CompletionTokens: -200, RequestId: "negative-output"},
						{Id: 805, UserId: 204, CreatedAt: 1_900_000_004, Type: LogTypeConsume, Username: "negative-components", InputTokensTotal: &negative, CompletionTokens: 7, RequestId: "negative-input"},
						{Id: 806, UserId: 204, CreatedAt: 1_900_000_004, Type: LogTypeConsume, Username: "negative-components", PromptTokens: -9, CompletionTokens: 3, RequestId: "negative-legacy-input"},
					}
					require.NoError(t, db.Create(&extremeRows).Error)
					for _, boundary := range []struct {
						user     int
						username string
						requests int
						tokens   int64
					}{
						{202, "sum-overflow", 2, math.MaxInt64},
						{203, "addition-overflow", 1, math.MaxInt64},
						{204, "negative-components", 3, 133},
					} {
						stat, err := SumUserUsedQuota(boundary.user, LogTypeConsume, 0, 0, "", "", 0, "")
						require.NoError(t, err)
						assert.EqualValues(t, boundary.tokens, stat.Tpm, boundary.username)
						assert.Equal(t, boundary.requests, stat.Rpm, boundary.username)
						assert.EqualValues(t, boundary.tokens, SumUsedToken(LogTypeConsume, 0, 0, "", boundary.username, ""), boundary.username)
					}
					var persistedExtreme []Log
					require.NoError(t, db.Where("id >= ?", 801).Order("id").Find(&persistedExtreme).Error)
					assert.Equal(t, extremeRows, persistedExtreme, "aggregate normalization must never mutate raw log usage")

				})
			}
		})
	}
}

// This is the existing ClickHouse schema before input_tokens_total was added.
// Do not derive it from the current CREATE helper: this must exercise ALTER.
const legacyInputTokenClickHouseDDL = `
CREATE TABLE logs (
 id Int64 DEFAULT 0, user_id Int32 DEFAULT 0, created_at Int64 DEFAULT 0,
 type Int32 DEFAULT 0, content String DEFAULT '', username String DEFAULT '',
 token_name String DEFAULT '', model_name String DEFAULT '', quota Int32 DEFAULT 0,
 prompt_tokens Int32 DEFAULT 0, completion_tokens Int32 DEFAULT 0,
 use_time Int32 DEFAULT 0, is_stream UInt8 DEFAULT 0, channel_id Int32 DEFAULT 0,
 token_id Int32 DEFAULT 0, site_id Int32 DEFAULT 0, ` + "`group`" + ` String DEFAULT '',
 ip String DEFAULT '', request_id String DEFAULT '', upstream_request_id String DEFAULT '',
 other String DEFAULT ''
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id)`

// ClickHouse's GORM table enumeration uses an incompatible information_schema
// table_type comparison on current servers. Inspect its native catalog so the
// fresh-database guard remains effective before this fixture changes anything.
func logInputMigrationTables(db *gorm.DB, databaseType common.DatabaseType) ([]string, error) {
	if databaseType != common.DatabaseTypeClickHouse {
		return db.Migrator().GetTables()
	}
	var tables []string
	err := db.Raw("SELECT name FROM system.tables WHERE database = currentDatabase()").Scan(&tables).Error
	return tables, err
}
