package common

import "strings"

type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

var mainDatabaseType = DatabaseTypeSQLite
var logDatabaseType = DatabaseTypeSQLite

func MainDatabaseType() DatabaseType {
	return mainDatabaseType
}

func LogDatabaseType() DatabaseType {
	return logDatabaseType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	mainDatabaseType = databaseType
}

func SetLogDatabaseType(databaseType DatabaseType) {
	logDatabaseType = databaseType
}

func SetDatabaseTypes(mainType DatabaseType, logType DatabaseType) {
	mainDatabaseType = mainType
	logDatabaseType = logType
}

func UsingMainDatabase(databaseType DatabaseType) bool {
	return mainDatabaseType == databaseType
}

func UsingLogDatabase(databaseType DatabaseType) bool {
	return logDatabaseType == databaseType
}

var SQLitePath = sqliteDSN("one-api.db")

// sqliteDSN preserves an explicitly configured DSN. Bare paths use the busy
// timeout syntax supported by our pure-Go SQLite driver without changing the
// database's journal mode or transaction locking policy.
func sqliteDSN(path string) string {
	if strings.Contains(path, "?") {
		return path
	}
	return path + "?_pragma=busy_timeout(30000)"
}
