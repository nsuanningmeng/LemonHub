package common

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteDSNPreservesExplicitConfiguration(t *testing.T) {
	for _, path := range []string{
		"/data/new-api.db?_pragma=busy_timeout(5000)",
		"file:custom?mode=memory&cache=shared",
		"custom.db?",
	} {
		assert.Equal(t, path, sqliteDSN(path))
	}
}

func TestSQLiteBarePathKeepsDataAndBusyTimeoutOnEveryConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	old, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = old.Exec("CREATE TABLE saved (value TEXT); INSERT INTO saved VALUES ('历史数据')")
	require.NoError(t, err)
	require.NoError(t, old.Close())

	db, err := sql.Open("sqlite", sqliteDSN(path))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	// Holding both connections ensures the setting is applied to new pooled
	// connections too, rather than only the first connection opened at startup.
	first, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer first.Close()
	second, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer second.Close()
	for _, conn := range []*sql.Conn{first, second} {
		var timeout int
		require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout))
		assert.Equal(t, 30000, timeout)
		var journal, value string
		require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journal))
		assert.Equal(t, "delete", journal)
		require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT value FROM saved").Scan(&value))
		assert.Equal(t, "历史数据", value)
	}
}
