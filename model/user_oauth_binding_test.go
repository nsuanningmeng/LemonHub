package model

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type legacyUserOAuthBinding struct {
	Id             int    `gorm:"primaryKey"`
	UserId         int    `gorm:"not null;uniqueIndex:ux_user_provider,priority:1"`
	ProviderId     int    `gorm:"not null;uniqueIndex:ux_user_provider,priority:2;uniqueIndex:ux_provider_userid,priority:1"`
	ProviderUserId string `gorm:"type:varchar(256);not null;uniqueIndex:ux_provider_userid,priority:2"`
}

func (legacyUserOAuthBinding) TableName() string { return "user_oauth_bindings" }

func createCustomOAuthProviderForBindingTest(t *testing.T, db *gorm.DB) CustomOAuthProvider {
	t.Helper()
	slug := strings.NewReplacer("/", "-", "_", "-").Replace(strings.ToLower(t.Name()))
	provider := CustomOAuthProvider{Name: t.Name(), Slug: slug}
	require.NoError(t, db.Create(&provider).Error)
	if db == DB {
		t.Cleanup(func() { DB.Delete(&CustomOAuthProvider{}, provider.Id) })
	}
	return provider
}

func TestCustomOAuthBindingAllowsCrossSiteReuseAndRejectsSameSiteReuse(t *testing.T) {
	truncateTables(t)
	provider := createCustomOAuthProviderForBindingTest(t, DB)

	users := []User{
		{SiteId: 0, Username: "custom-main", Password: "password", AffCode: "custom-main"},
		{SiteId: 3, Username: "custom-sub", Password: "password", AffCode: "custom-sub"},
		{SiteId: 3, Username: "custom-conflict", Password: "password", AffCode: "custom-conflict"},
	}
	require.NoError(t, DB.Create(&users).Error)
	require.NoError(t, CreateUserOAuthBinding(&UserOAuthBinding{UserId: users[0].Id, ProviderId: provider.Id, ProviderUserId: "shared-subject"}))
	require.NoError(t, CreateUserOAuthBinding(&UserOAuthBinding{UserId: users[1].Id, ProviderId: provider.Id, ProviderUserId: "shared-subject"}))
	err := CreateUserOAuthBinding(&UserOAuthBinding{UserId: users[2].Id, ProviderId: provider.Id, ProviderUserId: "shared-subject"})
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	found, err := GetUserByOAuthBinding(provider.Id, "shared-subject", 0)
	require.NoError(t, err)
	assert.Equal(t, users[0].Id, found.Id)
	found, err = GetUserByOAuthBinding(provider.Id, "shared-subject", 3)
	require.NoError(t, err)
	assert.Equal(t, users[1].Id, found.Id)
}

func TestCustomOAuthBindingMigrationBackfillsSiteAndClaim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &CustomOAuthProvider{}, &legacyUserOAuthBinding{}, &ExternalIdentityClaim{}))
	provider := createCustomOAuthProviderForBindingTest(t, db)

	owner := User{SiteId: 9, Username: "legacy-custom-owner", Password: "password", AffCode: "legacy-custom-owner"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&legacyUserOAuthBinding{UserId: owner.Id, ProviderId: provider.Id, ProviderUserId: "legacy-subject"}).Error)
	require.NoError(t, db.Exec(
		"CREATE UNIQUE INDEX ux_custom_oauth_legacy_subject_alt ON user_oauth_bindings (provider_id, provider_user_id)",
	).Error)

	require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
	require.NoError(t, prepareUserOAuthBindingsSiteScope(db))
	require.NoError(t, db.AutoMigrate(&UserOAuthBinding{}))
	require.NoError(t, finalizeUserOAuthBindingsSiteScope(db))
	var binding UserOAuthBinding
	require.NoError(t, db.First(&binding).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, customOAuthExternalIdentityProvider(provider.Id), binding.ProviderUserId, binding.UserId)
	}))

	assert.Equal(t, 9, binding.SiteId)
	assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
	assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex))
	assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, "ux_custom_oauth_legacy_subject_alt"))
}

func TestCustomOAuthBindingMigrationRejectsPartialOwnershipIndexes(t *testing.T) {
	createTable := func(t *testing.T) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id integer primary key, user_id integer not null, provider_id integer not null,
site_id integer not null default 0, provider_user_id text not null
)`).Error)
		return db
	}

	t.Run("partial canonical site index", func(t *testing.T) {
		db := createTable(t)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX ux_provider_userid ON user_oauth_bindings (provider_id, provider_user_id)",
		).Error)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX ux_provider_site_userid ON user_oauth_bindings (provider_id, site_id, provider_user_id) WHERE site_id <> 0",
		).Error)

		err := prepareUserOAuthBindingsSiteScope(db)
		require.ErrorContains(t, err, "unexpected definition")
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex))
	})

	t.Run("partial canonical legacy index", func(t *testing.T) {
		db := createTable(t)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX ux_provider_site_userid ON user_oauth_bindings (provider_id, site_id, provider_user_id)",
		).Error)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX ux_provider_userid ON user_oauth_bindings (provider_id, provider_user_id) WHERE site_id = 0",
		).Error)

		err := prepareUserOAuthBindingsSiteScope(db)
		require.ErrorContains(t, err, "unexpected definition")
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex))
	})
}

func TestCustomOAuthBindingMigrationResumesBeforeUserSiteColumnExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE users (id integer primary key)").Error)
	require.NoError(t, db.AutoMigrate(&CustomOAuthProvider{}, &legacyUserOAuthBinding{}))
	provider := createCustomOAuthProviderForBindingTest(t, db)
	require.NoError(t, db.Exec("INSERT INTO users (id) VALUES (1)").Error)
	require.NoError(t, db.Create(&legacyUserOAuthBinding{
		UserId: 1, ProviderId: provider.Id, ProviderUserId: "interrupted-custom-subject",
	}).Error)

	assert.False(t, db.Migrator().HasColumn(&User{}, "site_id"))
	require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
	require.NoError(t, prepareUserOAuthBindingsSiteScope(db))

	var binding UserOAuthBinding
	require.NoError(t, db.First(&binding).Error)
	assert.Zero(t, binding.SiteId)
	assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
}

func TestCustomOAuthBindingPreflightResumesEmptyTableBeforeUsersExist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyUserOAuthBinding{}))

	assert.False(t, db.Migrator().HasTable(&User{}))
	require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
	assert.False(t, db.Migrator().HasTable(&User{}), "preflight must not create missing migration tables")
}

func TestCustomOAuthBindingPreflightRejectsAmbiguousLegacyRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &CustomOAuthProvider{}))
	provider := createCustomOAuthProviderForBindingTest(t, db)
	require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id integer primary key, user_id integer not null, provider_id integer not null, provider_user_id varchar(128) not null
)`).Error)

	users := []User{
		{SiteId: 5, Username: "legacy-ambiguous-one", Password: "password", AffCode: "legacy-ambiguous-one"},
		{SiteId: 5, Username: "legacy-ambiguous-two", Password: "password", AffCode: "legacy-ambiguous-two"},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Exec("INSERT INTO user_oauth_bindings (user_id, provider_id, provider_user_id) VALUES (?, ?, ?), (?, ?, ?)",
		users[0].Id, provider.Id, "ambiguous-subject", users[1].Id, provider.Id, "ambiguous-subject").Error)

	err = preflightUserOAuthBindingsSiteScope(db)
	assert.ErrorContains(t, err, "ambiguous custom OAuth provider")
	assert.False(t, db.Migrator().HasColumn(&UserOAuthBinding{}, "site_id"), "preflight must fail before destructive DDL")
}

func TestCustomOAuthBindingPreflightRejectsPersistedClaimMismatchBeforeDDL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&User{},
		&CustomOAuthProvider{},
		&legacyUserOAuthBinding{},
		&legacyGlobalExternalIdentityClaim{},
	))
	provider := createCustomOAuthProviderForBindingTest(t, db)
	owner := User{Username: "custom-claim-mismatch", Password: "password", AffCode: "custom-claim-mismatch"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&legacyUserOAuthBinding{
		UserId: owner.Id, ProviderId: provider.Id, ProviderUserId: "binding-subject",
	}).Error)
	require.NoError(t, db.Create(&legacyGlobalExternalIdentityClaim{
		Provider: customOAuthExternalIdentityProvider(provider.Id),
		Subject:  "different-claim-subject",
		UserId:   owner.Id,
	}).Error)

	err = preflightUserOAuthBindingsSiteScope(db)
	require.ErrorContains(t, err, "conflicts with provider")
	assert.False(t, db.Migrator().HasColumn(&UserOAuthBinding{}, "site_id"))
	var binding legacyUserOAuthBinding
	require.NoError(t, db.First(&binding).Error)
	assert.Equal(t, "binding-subject", binding.ProviderUserId)
}

func TestCustomOAuthBindingPreflightUsesClaimCollationMySQL(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("TEST_MYSQL_IDENTITY_MIGRATION_DSN"))
	if rawDSN == "" {
		t.Skip("set TEST_MYSQL_IDENTITY_MIGRATION_DSN to a dedicated disposable database named lemonhub_identity_migration_test_*")
	}
	dsnConfig, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexpf(t, mysqlIdentityMigrationDatabaseName, strings.ToLower(dsnConfig.DBName),
		"refusing destructive identity migration test against database %q", dsnConfig.DBName)
	dsnConfig.ParseTime = true

	db, err := gorm.Open(gormmysql.Open(dsnConfig.FormatDSN()), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Regexpf(t, mysqlIdentityMigrationDatabaseName, strings.ToLower(selectedDatabase),
		"refusing destructive identity migration test against selected database %q", selectedDatabase)
	dropTables := func() {
		_ = db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error
		_ = db.Exec("DROP TABLE IF EXISTS user_oauth_bindings").Error
		_ = db.Exec("DROP TABLE IF EXISTS custom_oauth_providers").Error
		_ = db.Exec("DROP TABLE IF EXISTS users").Error
	}
	t.Cleanup(func() {
		dropTables()
		_ = sqlDB.Close()
	})

	tests := []struct {
		name            string
		targetCollation string
		tableCollation  string
		wantComparison  string
	}{
		{"current claim is case insensitive", "utf8mb4_unicode_ci", "utf8mb4_unicode_ci", "current claim"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dropTables()
			require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error)
			require.NoError(t, db.Exec(`CREATE TABLE custom_oauth_providers (
id bigint NOT NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error)
			require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
provider_user_id varchar(256) COLLATE utf8mb4_bin NOT NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_user_provider (user_id, provider_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error)
			require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) NOT NULL,
site_id int NOT NULL DEFAULT 0,
subject varchar(128) COLLATE %s NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject_site (provider, site_id, subject),
UNIQUE KEY idx_external_identity_user (provider, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=%s`, test.targetCollation, test.tableCollation)).Error)
			require.NoError(t, db.Exec("INSERT INTO users (id, site_id) VALUES (1, 7), (2, 7)").Error)
			require.NoError(t, db.Exec("INSERT INTO custom_oauth_providers (id) VALUES (9)").Error)
			require.NoError(t, db.Exec(
				"INSERT INTO user_oauth_bindings (id, user_id, provider_id, provider_user_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
				11, 1, 9, "Case-Identity", 12, 2, 9, "case-identity",
			).Error)

			claimDDLBefore := mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName())
			bindingDDLBefore := mysqlShowCreate(t, db, UserOAuthBinding{}.TableName())
			err = preflightUserOAuthBindingsSiteScope(db)
			require.ErrorContains(t, err, "ambiguous custom OAuth provider 9 ownership in site 7 under database collation")
			require.ErrorContains(t, err, "("+test.wantComparison+")")
			assert.Equal(t, claimDDLBefore, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
			assert.Equal(t, bindingDDLBefore, mysqlShowCreate(t, db, UserOAuthBinding{}.TableName()))
			assert.False(t, db.Migrator().HasColumn(&UserOAuthBinding{}, "site_id"), "preflight must not begin identity DDL")
		})
	}

	t.Run("empty interrupted binding table before users exists", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
provider_user_id varchar(128) COLLATE utf8mb4_bin NOT NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_user_provider (user_id, provider_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=COMPACT`).Error)

		assert.False(t, db.Migrator().HasTable(&User{}))
		require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
		assert.False(t, db.Migrator().HasTable(&User{}), "preflight must not create missing migration tables")
		var initialRowFormat string
		require.NoError(t, db.Raw(
			"SELECT ROW_FORMAT FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
			UserOAuthBinding{}.TableName(),
		).Scan(&initialRowFormat).Error)
		assert.Equal(t, "compact", strings.ToLower(initialRowFormat))
		require.NoError(t, prepareUserOAuthBindingsSiteScope(db))
		var rowFormat string
		require.NoError(t, db.Raw(
			"SELECT ROW_FORMAT FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
			UserOAuthBinding{}.TableName(),
		).Scan(&rowFormat).Error)
		assert.Equal(t, "dynamic", strings.ToLower(rowFormat))
		definition, err := mysqlIdentityStringColumnDefinition(
			db,
			UserOAuthBinding{}.TableName(),
			"provider_user_id",
			"custom OAuth binding subject",
		)
		require.NoError(t, err)
		assert.Equal(t, int64(externalIdentitySubjectMaxLength), definition.MaximumLength)
	})

	t.Run("widening preserves binding collation and non-strict data", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE custom_oauth_providers (
id bigint NOT NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
provider_user_id varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_user_provider (user_id, provider_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_general_ci ROW_FORMAT=COMPACT`).Error)
		require.NoError(t, db.Exec("INSERT INTO users (id, site_id) VALUES (1, 7)").Error)
		require.NoError(t, db.Exec("INSERT INTO custom_oauth_providers (id) VALUES (9)").Error)
		subject := "binding-\U0001F600"
		require.NoError(t, db.Exec(
			"INSERT INTO user_oauth_bindings (id, user_id, provider_id, provider_user_id) VALUES (?, ?, ?, ?)",
			11, 1, 9, subject,
		).Error)

		var originalSQLMode string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSQLMode).Error)
		require.NoError(t, db.Exec("SET SESSION sql_mode = ''").Error)
		defer func() { require.NoError(t, db.Exec("SET SESSION sql_mode = ?", originalSQLMode).Error) }()

		require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
		require.NoError(t, prepareUserOAuthBindingsSiteScope(db))
		require.NoError(t, db.AutoMigrate(&UserOAuthBinding{}))
		require.NoError(t, finalizeUserOAuthBindingsSiteScope(db))

		var persistedSubject string
		require.NoError(t, db.Model(&UserOAuthBinding{}).Where("id = ?", 11).
			Pluck("provider_user_id", &persistedSubject).Error)
		assert.Equal(t, subject, persistedSubject)
		var definition struct {
			CharacterSet  string `gorm:"column:character_set_name"`
			Collation     string `gorm:"column:collation_name"`
			MaximumLength int64  `gorm:"column:character_maximum_length"`
			RowFormat     string `gorm:"column:row_format"`
		}
		require.NoError(t, db.Raw(`SELECT c.CHARACTER_SET_NAME AS character_set_name,
c.COLLATION_NAME AS collation_name, c.CHARACTER_MAXIMUM_LENGTH AS character_maximum_length,
t.ROW_FORMAT AS row_format
FROM information_schema.COLUMNS c
JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.TABLE_NAME = c.TABLE_NAME
WHERE c.TABLE_SCHEMA = DATABASE() AND c.TABLE_NAME = ? AND c.COLUMN_NAME = ?`,
			UserOAuthBinding{}.TableName(), "provider_user_id").Scan(&definition).Error)
		assert.Equal(t, "utf8mb4", strings.ToLower(definition.CharacterSet))
		assert.Equal(t, "utf8mb4_bin", strings.ToLower(definition.Collation))
		assert.Equal(t, int64(externalIdentitySubjectMaxLength), definition.MaximumLength)
		assert.Equal(t, "dynamic", strings.ToLower(definition.RowFormat))
	})

	t.Run("single binding rejects an unrepresentable existing claim target before DDL", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE custom_oauth_providers (
id bigint NOT NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
provider_user_id varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_user_provider (user_id, provider_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) NOT NULL,
site_id int NOT NULL DEFAULT 0,
subject varchar(128) CHARACTER SET utf8 COLLATE utf8_bin NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject_site (provider, site_id, subject),
UNIQUE KEY idx_external_identity_user (provider, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_general_ci ROW_FORMAT=COMPACT`).Error)
		require.NoError(t, db.Exec("INSERT INTO users (id, site_id) VALUES (1, 7)").Error)
		require.NoError(t, db.Exec("INSERT INTO custom_oauth_providers (id) VALUES (9)").Error)
		subject := "binding-target-\U0001F600"
		require.NoError(t, db.Exec(
			"INSERT INTO user_oauth_bindings (id, user_id, provider_id, provider_user_id) VALUES (?, ?, ?, ?)",
			11, 1, 9, subject,
		).Error)
		bindingDDLBefore := mysqlShowCreate(t, db, UserOAuthBinding{}.TableName())
		claimDDLBefore := mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName())

		err = preflightUserOAuthBindingsSiteScope(db)
		require.ErrorContains(t, err, "cannot be represented")
		assert.Equal(t, bindingDDLBefore, mysqlShowCreate(t, db, UserOAuthBinding{}.TableName()))
		assert.Equal(t, claimDDLBefore, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
		var persistedSubject string
		require.NoError(t, db.Model(&UserOAuthBinding{}).Where("id = ?", 11).
			Pluck("provider_user_id", &persistedSubject).Error)
		assert.Equal(t, subject, persistedSubject)
	})

	t.Run("exact length nullable binding is normalized before AutoMigrate", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE custom_oauth_providers (
id bigint NOT NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_general_ci`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
provider_user_id varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_user_provider (user_id, provider_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_general_ci ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec("INSERT INTO users (id, site_id) VALUES (1, 7)").Error)
		require.NoError(t, db.Exec("INSERT INTO custom_oauth_providers (id) VALUES (9)").Error)
		subject := "nullable-binding-\U0001F600"
		require.NoError(t, db.Exec(
			"INSERT INTO user_oauth_bindings (id, user_id, provider_id, provider_user_id) VALUES (?, ?, ?, ?)",
			11, 1, 9, subject,
		).Error)

		var originalSQLMode string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSQLMode).Error)
		require.NoError(t, db.Exec("SET SESSION sql_mode = ''").Error)
		defer func() { require.NoError(t, db.Exec("SET SESSION sql_mode = ?", originalSQLMode).Error) }()

		require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
		require.NoError(t, prepareUserOAuthBindingsSiteScope(db))
		require.NoError(t, db.AutoMigrate(&UserOAuthBinding{}))
		require.NoError(t, finalizeUserOAuthBindingsSiteScope(db))

		definition, err := mysqlIdentityStringColumnDefinition(
			db,
			UserOAuthBinding{}.TableName(),
			"provider_user_id",
			"custom OAuth binding subject",
		)
		require.NoError(t, err)
		assert.Equal(t, "varchar", strings.ToLower(definition.DataType))
		assert.Equal(t, int64(externalIdentitySubjectMaxLength), definition.MaximumLength)
		assert.Equal(t, "no", strings.ToLower(definition.IsNullable))
		assert.False(t, definition.HasDefault)
		assert.Empty(t, definition.Comment)
		assert.Empty(t, definition.Extra)
		assert.Equal(t, "utf8mb4", strings.ToLower(definition.CharacterSet))
		assert.Equal(t, "utf8mb4_bin", strings.ToLower(definition.Collation))
		var persistedSubject string
		require.NoError(t, db.Model(&UserOAuthBinding{}).Where("id = ?", 11).
			Pluck("provider_user_id", &persistedSubject).Error)
		assert.Equal(t, subject, persistedSubject)
	})

	t.Run("non-strict runtime rejects binding truncation and charset loss", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE custom_oauth_providers (
id bigint NOT NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
site_id int NOT NULL DEFAULT 0,
subject varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject_site (provider, site_id, subject),
UNIQUE KEY idx_external_identity_user (provider, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL AUTO_INCREMENT,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
site_id int NOT NULL DEFAULT 0,
provider_user_id varchar(128) CHARACTER SET utf8 COLLATE utf8_bin NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_bin ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec("INSERT INTO users (id, site_id) VALUES (1, 7)").Error)
		require.NoError(t, db.Exec("INSERT INTO custom_oauth_providers (id) VALUES (9)").Error)
		var originalSQLMode string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSQLMode).Error)
		require.NoError(t, db.Exec("SET SESSION sql_mode = ''").Error)
		defer func() { require.NoError(t, db.Exec("SET SESSION sql_mode = ?", originalSQLMode).Error) }()

		for _, test := range []struct {
			name    string
			subject string
			want    string
		}{
			{name: "column capacity", subject: strings.Repeat("s", 129), want: "exceeds configured MySQL column capacity"},
			{name: "character set", subject: "custom-subject-\U0001F600", want: "cannot be represented"},
		} {
			t.Run(test.name, func(t *testing.T) {
				err = db.Transaction(func(tx *gorm.DB) error {
					return CreateUserOAuthBindingWithTx(tx, &UserOAuthBinding{
						UserId: 1, ProviderId: 9, ProviderUserId: test.subject,
					})
				})
				require.ErrorContains(t, err, test.want)
				var bindingCount int64
				require.NoError(t, db.Model(&UserOAuthBinding{}).Count(&bindingCount).Error)
				assert.Zero(t, bindingCount)
				var claimCount int64
				require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCount).Error)
				assert.Zero(t, claimCount)
			})
		}
	})

	t.Run("unrelated functional indexes are ignored", func(t *testing.T) {
		var version struct {
			Major int `gorm:"column:major"`
			Minor int `gorm:"column:minor"`
			Patch int `gorm:"column:patch"`
		}
		require.NoError(t, db.Raw(`SELECT
CAST(SUBSTRING_INDEX(VERSION(), '.', 1) AS UNSIGNED) AS major,
CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(VERSION(), '.', 2), '.', -1) AS UNSIGNED) AS minor,
CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(VERSION(), '.', 3), '.', -1) AS UNSIGNED) AS patch`).Scan(&version).Error)
		if version.Major < 8 || (version.Major == 8 && version.Minor == 0 && version.Patch < 13) {
			t.Skip("functional key parts require MySQL 8.0.13+")
		}

		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
site_id bigint NOT NULL DEFAULT 0,
subject varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject_site (provider, site_id, subject)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL AUTO_INCREMENT,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
site_id bigint NOT NULL DEFAULT 0,
provider_user_id varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_provider_site_userid (provider_id, site_id, provider_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec(
			"CREATE INDEX idx_external_identity_expression ON external_identity_claims ((user_id + 1))",
		).Error)
		require.NoError(t, db.Exec(
			"CREATE INDEX idx_oauth_binding_expression ON user_oauth_bindings ((provider_id + 1))",
		).Error)

		claimIndexes, err := legacyGlobalExternalIdentitySubjectIndexes(db)
		require.NoError(t, err)
		assert.NotContains(t, claimIndexes, "idx_external_identity_expression")
		bindingIndexes, err := legacyGlobalUserOAuthBindingSubjectIndexes(db)
		require.NoError(t, err)
		assert.NotContains(t, bindingIndexes, "idx_oauth_binding_expression")
	})

	t.Run("legacy prefix ownership indexes are removed only after full site indexes exist", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
site_id bigint NOT NULL DEFAULT 0,
subject varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject_site (provider, site_id, subject),
UNIQUE KEY idx_external_identity_subject (provider, subject(64))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL AUTO_INCREMENT,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
site_id bigint NOT NULL DEFAULT 0,
provider_user_id varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_provider_site_userid (provider_id, site_id, provider_user_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id(64))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)

		require.NoError(t, finalizeExternalIdentityClaimsSiteScope(db))
		require.NoError(t, finalizeUserOAuthBindingsSiteScope(db))
		assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
		assert.False(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
		assert.False(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex))
	})

	t.Run("prefix site index is rejected without dropping legacy ownership index", func(t *testing.T) {
		dropTables()
		require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id bigint NOT NULL,
user_id bigint NOT NULL,
provider_id bigint NOT NULL,
site_id int NOT NULL DEFAULT 0,
provider_user_id varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
PRIMARY KEY (id),
UNIQUE KEY ux_user_provider (user_id, provider_id),
UNIQUE KEY ux_provider_userid (provider_id, provider_user_id),
UNIQUE KEY ux_provider_site_userid (provider_id, site_id, provider_user_id(64))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=DYNAMIC`).Error)

		err = prepareUserOAuthBindingsSiteScope(db)
		require.ErrorContains(t, err, "unexpected definition")
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex))
		assert.True(t, db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex))
	})
}

func TestCustomOAuthBindingPreflightRejectsNonCanonicalAndOrphanedRows(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		withOwner    bool
		withProvider bool
		subject      string
		want         string
	}{
		{name: "whitespace", withOwner: true, withProvider: true, subject: " subject ", want: "non-canonical whitespace"},
		{name: "missing-provider", withOwner: true, subject: "subject", want: "resolve provider"},
		{name: "missing-owner", withProvider: true, subject: "subject", want: "resolve owner"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&User{}, &CustomOAuthProvider{}))
			require.NoError(t, db.Exec(`CREATE TABLE user_oauth_bindings (
id integer primary key, user_id integer not null, provider_id integer not null, provider_user_id varchar(256) not null
)`).Error)

			userId := 999
			if testCase.withOwner {
				owner := User{Username: "preflight-owner", Password: "password", AffCode: "preflight-owner", AuthVersion: 1}
				require.NoError(t, db.Create(&owner).Error)
				userId = owner.Id
			}
			providerId := 999
			if testCase.withProvider {
				provider := createCustomOAuthProviderForBindingTest(t, db)
				providerId = provider.Id
			}
			require.NoError(t, db.Exec(
				"INSERT INTO user_oauth_bindings (user_id, provider_id, provider_user_id) VALUES (?, ?, ?)",
				userId, providerId, testCase.subject,
			).Error)

			err = preflightUserOAuthBindingsSiteScope(db)
			require.ErrorContains(t, err, testCase.want)
			assert.False(t, db.Migrator().HasColumn(&UserOAuthBinding{}, "site_id"))
		})
	}
}

func TestCustomOAuthBindingPreservesLongLegacySubject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &CustomOAuthProvider{}, &legacyUserOAuthBinding{}, &ExternalIdentityClaim{}))
	provider := createCustomOAuthProviderForBindingTest(t, db)
	owner := User{SiteId: 4, Username: "long-custom-owner", Password: "password", AffCode: "long-custom-owner", AuthVersion: 1}
	require.NoError(t, db.Create(&owner).Error)
	subject := strings.Repeat("x", 200)
	require.NoError(t, db.Create(&legacyUserOAuthBinding{UserId: owner.Id, ProviderId: provider.Id, ProviderUserId: subject}).Error)

	require.NoError(t, preflightUserOAuthBindingsSiteScope(db))
	require.NoError(t, prepareUserOAuthBindingsSiteScope(db))
	require.NoError(t, db.AutoMigrate(&UserOAuthBinding{}))
	require.NoError(t, finalizeUserOAuthBindingsSiteScope(db))

	var binding UserOAuthBinding
	require.NoError(t, db.First(&binding).Error)
	assert.Equal(t, subject, binding.ProviderUserId)
	assert.Equal(t, owner.SiteId, binding.SiteId)
}

func TestDeleteCustomOAuthProviderRefusesBindingsAndPreservesClaims(t *testing.T) {
	truncateTables(t)
	provider := createCustomOAuthProviderForBindingTest(t, DB)
	owner := User{Username: "provider-delete-owner", Password: "password", AffCode: "provider-delete-owner", AuthVersion: 1}
	require.NoError(t, DB.Create(&owner).Error)
	require.NoError(t, CreateUserOAuthBinding(&UserOAuthBinding{
		UserId: owner.Id, ProviderId: provider.Id, ProviderUserId: "provider-delete-subject",
	}))

	err := DeleteCustomOAuthProvider(provider.Id)
	require.ErrorIs(t, err, ErrCustomOAuthProviderHasBindings)
	assert.NoError(t, DB.First(&CustomOAuthProvider{}, provider.Id).Error)
	assert.NoError(t, DB.Where("provider_id = ?", provider.Id).First(&UserOAuthBinding{}).Error)
	assert.NoError(t, DB.Where("provider = ?", customOAuthExternalIdentityProvider(provider.Id)).First(&ExternalIdentityClaim{}).Error)

	require.NoError(t, DeleteUserOAuthBinding(owner.Id, provider.Id))
	require.NoError(t, DeleteCustomOAuthProvider(provider.Id))
	assert.ErrorIs(t, DB.First(&CustomOAuthProvider{}, provider.Id).Error, gorm.ErrRecordNotFound)
}

func TestUpdateCustomOAuthProviderDoesNotRecreateDeletedRow(t *testing.T) {
	truncateTables(t)
	provider := CustomOAuthProvider{
		Name:                  "stale-provider-update",
		Slug:                  "stale-provider-update",
		ClientId:              "client-id",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		UserInfoEndpoint:      "https://issuer.example/userinfo",
	}
	require.NoError(t, CreateCustomOAuthProvider(&provider))
	require.NoError(t, DeleteCustomOAuthProvider(provider.Id))

	provider.Name = "must-not-reappear"
	assert.ErrorIs(t, UpdateCustomOAuthProvider(&provider), gorm.ErrRecordNotFound)
	var count int64
	require.NoError(t, DB.Model(&CustomOAuthProvider{}).Where("id = ?", provider.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateCustomOAuthBindingRejectsMissingProviderWithoutClaim(t *testing.T) {
	truncateTables(t)
	owner := User{Username: "missing-provider-owner", Password: "password", AffCode: "missing-provider-owner", AuthVersion: 1}
	require.NoError(t, DB.Create(&owner).Error)

	err := CreateUserOAuthBinding(&UserOAuthBinding{UserId: owner.Id, ProviderId: 987654, ProviderUserId: "orphan-subject"})
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
	var count int64
	require.NoError(t, DB.Model(&ExternalIdentityClaim{}).Where("user_id = ?", owner.Id).Count(&count).Error)
	assert.Zero(t, count)
}
