package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The real PostgreSQL catalog must distinguish an exact site-scoped identity
// index from a partial, expression, reordered, or covering index. In particular,
// GORM's aggregate column order does not necessarily match index key order.
func TestPostgresIdentityIndexesPreserveKeyOrder(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_IDENTITY_INDEX_DSN"))
	if dsn == "" {
		t.Skip("set POSTGRES_IDENTITY_INDEX_DSN to an empty lemonhub_pg_identity_index_test_* database")
	}
	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^lemonhub_pg_identity_index_test_[a-z0-9_]+$`), config.Database,
		"refusing identity index fixtures outside a dedicated disposable database")
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&selectedDatabase).Error)
	require.Equal(t, config.Database, selectedDatabase)
	var tableCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')").Scan(&tableCount).Error)
	require.Zero(t, tableCount, "refusing to modify a nonempty fixture database")
	var serverVersion int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&serverVersion).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS user_oauth_bindings, external_identity_claims").Error)
	})
	require.NoError(t, db.Exec("CREATE TABLE user_oauth_bindings (provider_id BIGINT, provider_user_id TEXT, site_id BIGINT, user_id BIGINT)").Error)
	require.NoError(t, db.Exec("CREATE TABLE external_identity_claims (provider TEXT, subject TEXT, site_id BIGINT, user_id BIGINT)").Error)

	for _, identity := range []struct {
		name, table, provider, subject string
		inspect                        func(*gorm.DB, string) (bool, error)
	}{
		{name: "custom OAuth", table: "user_oauth_bindings", provider: "provider_id", subject: "provider_user_id", inspect: userOAuthBindingSubjectIndexIsSiteScoped},
		{name: "built-in identity", table: "external_identity_claims", provider: "provider", subject: "subject", inspect: externalIdentitySubjectIndexIsSiteScoped},
	} {
		t.Run(identity.name, func(t *testing.T) {
			const indexName = "test_identity_subject_index"
			found, err := identity.inspect(db, indexName)
			require.NoError(t, err)
			assert.False(t, found, "a missing index cannot satisfy migration validation")
			for _, test := range []struct {
				name, unique, definition string
				want                     bool
				minimumVersion           int
			}{
				{name: "exact keys in catalog order", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id, %s)", identity.provider, identity.subject), want: true},
				{name: "reverse keys", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id, %s)", identity.subject, identity.provider)},
				{name: "same keys in another order", unique: "UNIQUE", definition: fmt.Sprintf("(%s, %s, site_id)", identity.provider, identity.subject)},
				{name: "partial unique", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id, %s) WHERE site_id > 0", identity.provider, identity.subject)},
				{name: "expression unique", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id, lower(%s))", identity.provider, identity.subject)},
				{name: "non unique", definition: fmt.Sprintf("(%s, site_id, %s)", identity.provider, identity.subject)},
				{name: "missing site key", unique: "UNIQUE", definition: fmt.Sprintf("(%s, %s)", identity.provider, identity.subject)},
				{name: "extra key", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id, %s, user_id)", identity.provider, identity.subject)},
				{name: "extra included column", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id, %s) INCLUDE (user_id)", identity.provider, identity.subject), minimumVersion: 110000},
				{name: "included subject is not a unique key", unique: "UNIQUE", definition: fmt.Sprintf("(%s, site_id) INCLUDE (%s)", identity.provider, identity.subject), minimumVersion: 110000},
			} {
				t.Run(test.name, func(t *testing.T) {
					if serverVersion < test.minimumVersion {
						t.Skip("INCLUDE indexes require PostgreSQL 11 or later")
					}
					// Every SQL identifier and definition comes from the closed table
					// above, never from the DSN or any external input.
					require.NoError(t, db.Exec(fmt.Sprintf("CREATE %s INDEX %s ON %s %s", test.unique, indexName, identity.table, test.definition)).Error)
					t.Cleanup(func() { require.NoError(t, db.Exec("DROP INDEX "+indexName).Error) })
					found, err := identity.inspect(db, indexName)
					require.NoError(t, err)
					assert.Equal(t, test.want, found, "migration must inspect actual uniqueness and ordered key columns")
				})
			}
		})
	}
}
