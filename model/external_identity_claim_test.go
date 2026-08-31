package model

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type legacyGlobalExternalIdentityClaim struct {
	Id        int64  `gorm:"primaryKey"`
	Provider  string `gorm:"type:varchar(32);not null;uniqueIndex:idx_external_identity_subject,priority:1;uniqueIndex:idx_external_identity_user,priority:1"`
	Subject   string `gorm:"type:varchar(128);not null;uniqueIndex:idx_external_identity_subject,priority:2"`
	UserId    int    `gorm:"not null;index;uniqueIndex:idx_external_identity_user,priority:2"`
	CreatedAt time.Time
}

func (legacyGlobalExternalIdentityClaim) TableName() string {
	return "external_identity_claims"
}

func TestExternalIdentityClaimEnforcesSingleOwnerAtomically(t *testing.T) {
	truncateTables(t)

	first := User{Username: "telegram-owner-one", Password: "password", AffCode: "telegram-owner-one"}
	second := User{Username: "telegram-owner-two", Password: "password", AffCode: "telegram-owner-two"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "telegram-123", first.Id)
	}))
	err := DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "telegram-123", second.Id)
	})
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	err = DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "telegram-456", first.Id)
	})
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	var claims []ExternalIdentityClaim
	require.NoError(t, DB.Find(&claims).Error)
	require.Len(t, claims, 1)
	assert.Equal(t, first.Id, claims[0].UserId)
	assert.Equal(t, "telegram-123", claims[0].Subject)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ReleaseExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, first.Id)
	}))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "telegram-123", second.Id)
	}))
}

func TestExternalIdentityClaimAllowsSameSubjectOnDifferentSites(t *testing.T) {
	truncateTables(t)

	mainSiteUser := User{
		SiteId:   0,
		Username: "telegram-main-site",
		Password: "password",
		AffCode:  "telegram-main-site",
	}
	subSiteUser := User{
		SiteId:   1,
		Username: "telegram-sub-site",
		Password: "password",
		AffCode:  "telegram-sub-site",
	}
	rejectedSameSiteUser := User{
		SiteId:   1,
		Username: "telegram-same-site-conflict",
		Password: "password",
		AffCode:  "telegram-same-site-conflict",
	}
	require.NoError(t, DB.Create(&mainSiteUser).Error)
	require.NoError(t, DB.Create(&subSiteUser).Error)
	require.NoError(t, DB.Create(&rejectedSameSiteUser).Error)

	for _, user := range []User{mainSiteUser, subSiteUser} {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, "shared-telegram-id", user.Id)
		}))
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(
			tx,
			ExternalIdentityProviderTelegram,
			"shared-telegram-id",
			rejectedSameSiteUser.Id,
		)
	})
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	var claims []ExternalIdentityClaim
	require.NoError(t, DB.Order("site_id ASC").Find(&claims).Error)
	require.Len(t, claims, 2)
	assert.Equal(t, 0, claims[0].SiteId)
	assert.Equal(t, mainSiteUser.Id, claims[0].UserId)
	assert.Equal(t, 1, claims[1].SiteId)
	assert.Equal(t, subSiteUser.Id, claims[1].UserId)
}

func TestClearTelegramBindingReleasesIdentityClaim(t *testing.T) {
	truncateTables(t)

	user := User{Username: "telegram-unbind", Password: "password", TelegramId: "telegram-unbind-id"}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.TelegramId, user.Id)
	}))

	require.NoError(t, user.ClearBinding(ExternalIdentityProviderTelegram))
	assert.Empty(t, user.TelegramId)

	var count int64
	require.NoError(t, DB.Model(&ExternalIdentityClaim{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestInitializeExternalIdentityClaimsIsIdempotent(t *testing.T) {
	truncateTables(t)

	user := User{Username: "telegram-legacy", Password: "password", TelegramId: "telegram-legacy-id"}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, InitializeExternalIdentityClaims())
	require.NoError(t, InitializeExternalIdentityClaims())

	var claim ExternalIdentityClaim
	require.NoError(t, DB.Where("provider = ? AND subject = ?", ExternalIdentityProviderTelegram, user.TelegramId).
		First(&claim).Error)
	assert.Equal(t, user.Id, claim.UserId)
}

func TestInitializeExternalIdentityClaimsRejectsSameSiteAmbiguousLegacyBindings(t *testing.T) {
	truncateTables(t)

	first := User{Username: "telegram-legacy-one", Password: "password", TelegramId: "duplicate-telegram-id", AffCode: "telegram-legacy-one"}
	second := User{Username: "telegram-legacy-two", Password: "password", TelegramId: "duplicate-telegram-id", AffCode: "telegram-legacy-two"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)

	err := InitializeExternalIdentityClaims()
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	var count int64
	require.NoError(t, DB.Model(&ExternalIdentityClaim{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestInitializeExternalIdentityClaimsPreservesCrossSiteLegacyBindings(t *testing.T) {
	truncateTables(t)

	mainSiteUser := User{
		SiteId:     0,
		Username:   "telegram-legacy-main",
		Password:   "password",
		TelegramId: "cross-site-telegram-id",
		AffCode:    "telegram-legacy-main",
	}
	subSiteUser := User{
		SiteId:     1,
		Username:   "telegram-legacy-sub",
		Password:   "password",
		TelegramId: "cross-site-telegram-id",
		AffCode:    "telegram-legacy-sub",
	}
	require.NoError(t, DB.Create(&mainSiteUser).Error)
	require.NoError(t, DB.Create(&subSiteUser).Error)

	require.NoError(t, InitializeExternalIdentityClaims())
	require.NoError(t, InitializeExternalIdentityClaims())

	var claims []ExternalIdentityClaim
	require.NoError(t, DB.Order("site_id ASC").Find(&claims).Error)
	require.Len(t, claims, 2)
	assert.Equal(t, mainSiteUser.Id, claims[0].UserId)
	assert.Equal(t, 0, claims[0].SiteId)
	assert.Equal(t, subSiteUser.Id, claims[1].UserId)
	assert.Equal(t, 1, claims[1].SiteId)
}

func TestPreflightExternalIdentityClaimsRejectsOnlySameSiteDuplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}))

	users := []User{
		{SiteId: 0, Username: "preflight-main", Password: "password", TelegramId: "shared-preflight", AffCode: "preflight-main"},
		{SiteId: 1, Username: "preflight-sub", Password: "password", TelegramId: "shared-preflight", AffCode: "preflight-sub"},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, preflightExternalIdentityClaims(db))

	conflict := User{
		SiteId:     1,
		Username:   "preflight-sub-conflict",
		Password:   "password",
		TelegramId: "shared-preflight",
		AffCode:    "preflight-sub-conflict",
	}
	require.NoError(t, db.Create(&conflict).Error)
	require.ErrorContains(t, preflightExternalIdentityClaims(db), "ambiguous legacy Telegram ownership in site 1")
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}))

	require.NoError(t, db.Model(&conflict).Update("telegram_id", " \t ").Error)
	require.ErrorContains(t, preflightExternalIdentityClaims(db), "external identity subject is empty")
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}))
}

func TestPreflightExternalIdentityClaimsRejectsNonCanonicalWhitespace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))

	user := User{
		Username:    "preflight-whitespace",
		Password:    "password",
		GitHubId:    " github-subject ",
		AffCode:     "preflight-whitespace",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)

	err = preflightExternalIdentityClaims(db)
	require.ErrorContains(t, err, "non-canonical whitespace in legacy GitHub ownership")
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}))
}

func TestPreflightExternalIdentityClaimsUsesDatabaseCollation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE users (
id integer primary key,
site_id integer,
github_id text COLLATE NOCASE
)`).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, site_id, github_id) VALUES (?, ?, ?), (?, ?, ?)",
		1, 3, "Case-Sensitive-In-Go", 2, 4, "case-sensitive-in-go",
	).Error)

	// Cross-site reuse remains valid even when the source column's collation
	// considers the two subjects equivalent.
	require.NoError(t, preflightExternalIdentityClaims(db))
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}))

	require.NoError(t, db.Exec(
		"INSERT INTO users (id, site_id, github_id) VALUES (?, ?, ?)",
		3, 3, "case-sensitive-in-go",
	).Error)
	err = preflightExternalIdentityClaims(db)
	require.ErrorContains(t, err, "ambiguous legacy GitHub ownership in site 3 under database collation")
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}), "preflight must fail before identity DDL")
}

func TestPreflightExternalIdentityClaimsWithoutSiteColumnUsesMainSite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE users (
id integer primary key,
github_id text COLLATE NOCASE
)`).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, github_id) VALUES (?, ?), (?, ?)",
		1, "Legacy-Identity", 2, "legacy-identity",
	).Error)

	err = preflightExternalIdentityClaims(db)
	require.ErrorContains(t, err, "ambiguous legacy GitHub ownership in site 0 under database collation")
	assert.False(t, db.Migrator().HasTable(&ExternalIdentityClaim{}), "preflight must fail before identity DDL")
}

func TestPreflightExternalIdentityClaimsRejectsPersistedClaimMismatchBeforeDDL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &legacyGlobalExternalIdentityClaim{}))
	owner := User{
		Username: "claim-mismatch-owner", Password: "password", AffCode: "claim-mismatch-owner",
		GitHubId: "legacy-github-subject",
	}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&legacyGlobalExternalIdentityClaim{
		Provider: ExternalIdentityProviderGitHub,
		Subject:  "different-claim-subject",
		UserId:   owner.Id,
	}).Error)

	err = preflightExternalIdentityClaims(db)
	require.ErrorContains(t, err, "conflicts with legacy GitHub binding")
	assert.False(t, db.Migrator().HasColumn(&ExternalIdentityClaim{}, "site_id"))
	var persisted legacyGlobalExternalIdentityClaim
	require.NoError(t, db.First(&persisted).Error)
	assert.Equal(t, "different-claim-subject", persisted.Subject)
}

var mysqlIdentityMigrationDatabaseName = regexp.MustCompile(`^lemonhub_identity_migration_test_[a-z0-9_]+$`)

// TestPreflightExternalIdentityClaimsMySQLCollation is intentionally guarded
// by a dedicated DSN and a strict disposable-database name. It proves that
// MySQL's real case/accent-insensitive comparison is applied before any
// claim-table DDL.
func TestPreflightExternalIdentityClaimsMySQLCollation(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("TEST_MYSQL_IDENTITY_MIGRATION_DSN"))
	if rawDSN == "" {
		t.Skip("set TEST_MYSQL_IDENTITY_MIGRATION_DSN to a dedicated disposable database named lemonhub_identity_migration_test_*")
	}
	dsnConfig, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexpf(t, mysqlIdentityMigrationDatabaseName, strings.ToLower(dsnConfig.DBName),
		"refusing destructive identity migration test against database %q: its name must match lemonhub_identity_migration_test_*", dsnConfig.DBName)
	dsnConfig.ParseTime = true

	db, err := gorm.Open(mysql.Open(dsnConfig.FormatDSN()), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Regexpf(t, mysqlIdentityMigrationDatabaseName, strings.ToLower(selectedDatabase),
		"refusing destructive identity migration test against selected database %q", selectedDatabase)
	t.Cleanup(func() {
		_ = db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error
		_ = db.Exec("DROP TABLE IF EXISTS user_oauth_bindings").Error
		_ = db.Exec("DROP TABLE IF EXISTS users").Error
		_ = sqlDB.Close()
	})

	tests := []struct {
		name            string
		sourceCollation string
		targetCollation string
		tableCollation  string
		wantError       string
	}{
		{
			"matching case insensitive columns reject equivalent identities",
			"utf8mb4_unicode_ci", "utf8mb4_unicode_ci", "utf8mb4_unicode_ci",
			"ambiguous legacy GitHub ownership in site 7 under database collation (source)",
		},
		{
			"binary source and case insensitive claim fail closed",
			"utf8mb4_bin", "utf8mb4_unicode_ci", "utf8mb4_unicode_ci",
			"must use identical comparison semantics",
		},
		{
			"case insensitive source and binary claim fail closed",
			"utf8mb4_unicode_ci", "utf8mb4_bin", "utf8mb4_bin",
			"must use identical comparison semantics",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
			require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
			require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TABLE users (
id bigint NOT NULL AUTO_INCREMENT,
site_id int NULL DEFAULT 0,
github_id varchar(191) COLLATE %s NULL,
telegram_id varchar(191) COLLATE %s NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, test.sourceCollation, test.targetCollation)).Error)
			require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) NOT NULL,
subject varchar(128) COLLATE %s NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject (provider, subject),
UNIQUE KEY idx_external_identity_user (provider, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=%s`, test.targetCollation, test.tableCollation)).Error)
			require.NoError(t, db.Exec(
				"INSERT INTO users (site_id, github_id, telegram_id) VALUES (?, ?, ?), (?, ?, NULL)",
				7, "Case-Identity", "preserved-subject", 7, "case-identity",
			).Error)
			require.NoError(t, db.Exec(
				"INSERT INTO external_identity_claims (provider, subject, user_id) VALUES (?, ?, ?)",
				ExternalIdentityProviderTelegram, "preserved-subject", 1,
			).Error)

			claimDDLBefore := mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName())
			var claimCountBefore int64
			require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCountBefore).Error)

			err = preflightExternalIdentityClaims(db)
			require.ErrorContains(t, err, test.wantError)
			assert.Equal(t, claimDDLBefore, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
			assert.False(t, db.Migrator().HasColumn(&ExternalIdentityClaim{}, "site_id"), "preflight must not begin identity DDL")
			var claimCountAfter int64
			require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCountAfter).Error)
			assert.Equal(t, claimCountBefore, claimCountAfter)
			var preserved ExternalIdentityClaim
			require.NoError(t, db.Where("provider = ?", ExternalIdentityProviderTelegram).First(&preserved).Error)
			assert.Equal(t, "preserved-subject", preserved.Subject)
		})
	}

	t.Run("widening preserves explicit collation and non-strict data", func(t *testing.T) {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL AUTO_INCREMENT,
site_id int NULL DEFAULT 0,
github_id varchar(191) COLLATE utf8mb4_bin NULL,
telegram_id varchar(191) COLLATE utf8mb4_bin NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin ROW_FORMAT=COMPACT`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
site_id int NOT NULL DEFAULT 0,
subject varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject_site (provider, site_id, subject),
UNIQUE KEY idx_external_identity_user (provider, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_general_ci ROW_FORMAT=COMPACT`).Error)
		var initialRowFormat string
		require.NoError(t, db.Raw(
			"SELECT ROW_FORMAT FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
			ExternalIdentityClaim{}.TableName(),
		).Scan(&initialRowFormat).Error)
		assert.Equal(t, "compact", strings.ToLower(initialRowFormat))
		firstSubject := "Case-Persisted-\U0001F600"
		secondSubject := "case-persisted-\U0001F600"
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, site_id, github_id, telegram_id) VALUES (?, ?, NULL, ?), (?, ?, NULL, ?)",
			1, 7, firstSubject, 2, 7, secondSubject,
		).Error)
		require.NoError(t, db.Exec(
			"INSERT INTO external_identity_claims (provider, site_id, subject, user_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
			ExternalIdentityProviderTelegram, 7, firstSubject, 1,
			ExternalIdentityProviderTelegram, 7, secondSubject, 2,
		).Error)

		var originalSQLMode string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSQLMode).Error)
		require.NoError(t, db.Exec("SET SESSION sql_mode = ''").Error)
		defer func() { require.NoError(t, db.Exec("SET SESSION sql_mode = ?", originalSQLMode).Error) }()

		require.NoError(t, preflightExternalIdentityClaims(db))
		require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
		require.NoError(t, db.AutoMigrate(&ExternalIdentityClaim{}))
		require.NoError(t, finalizeExternalIdentityClaimsSiteScope(db))

		var subjects []string
		require.NoError(t, db.Model(&ExternalIdentityClaim{}).Order("id").Pluck("subject", &subjects).Error)
		assert.Equal(t, []string{firstSubject, secondSubject}, subjects)
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
			ExternalIdentityClaim{}.TableName(), "subject").Scan(&definition).Error)
		assert.Equal(t, "utf8mb4", strings.ToLower(definition.CharacterSet))
		assert.Equal(t, "utf8mb4_bin", strings.ToLower(definition.Collation))
		assert.Equal(t, int64(externalIdentitySubjectMaxLength), definition.MaximumLength)
		assert.Equal(t, "dynamic", strings.ToLower(definition.RowFormat))
	})

	t.Run("persisted claims without user site column use main site", func(t *testing.T) {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL AUTO_INCREMENT,
github_id varchar(191) COLLATE utf8mb4_bin NULL,
telegram_id varchar(191) COLLATE utf8mb4_bin NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id bigint NOT NULL AUTO_INCREMENT,
provider varchar(32) COLLATE utf8mb4_bin NOT NULL,
subject varchar(128) COLLATE utf8mb4_bin NOT NULL,
user_id bigint NOT NULL,
created_at datetime(3) NULL,
PRIMARY KEY (id),
UNIQUE KEY idx_external_identity_subject (provider, subject),
UNIQUE KEY idx_external_identity_user (provider, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error)
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, github_id, telegram_id) VALUES (?, NULL, ?), (?, NULL, ?)",
			1, "Case-Main-Site", 2, "case-main-site",
		).Error)
		require.NoError(t, db.Exec(
			"INSERT INTO external_identity_claims (provider, subject, user_id) VALUES (?, ?, ?), (?, ?, ?)",
			ExternalIdentityProviderTelegram, "Case-Main-Site", 1,
			ExternalIdentityProviderTelegram, "case-main-site", 2,
		).Error)

		claimDDLBefore := mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName())
		var claimCountBefore int64
		require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCountBefore).Error)

		err = preflightExternalIdentityClaims(db)
		require.NoError(t, err)
		assert.Equal(t, claimDDLBefore, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
		var claimCountAfter int64
		require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCountAfter).Error)
		assert.Equal(t, claimCountBefore, claimCountAfter)
	})

	t.Run("narrow existing claim charset rejects unrepresentable legacy subject before DDL", func(t *testing.T) {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
github_id varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COLLATE=utf8_bin`).Error)
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
		subject := "legacy-\U0001F600"
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, site_id, github_id) VALUES (?, ?, ?)",
			1, 7, subject,
		).Error)
		claimDDLBefore := mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName())

		err = preflightExternalIdentityClaims(db)
		require.ErrorContains(t, err, "cannot be represented")
		assert.Equal(t, claimDDLBefore, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
		var persistedSubject string
		require.NoError(t, db.Table("users").Where("id = ?", 1).Pluck("github_id", &persistedSubject).Error)
		assert.Equal(t, subject, persistedSubject)
	})

	t.Run("narrow database default preserves legacy comparison and dynamic storage", func(t *testing.T) {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS user_oauth_bindings").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
		var originalSchema mysqlIdentityComparison
		require.NoError(t, db.Raw(`SELECT DEFAULT_CHARACTER_SET_NAME AS character_set_name,
DEFAULT_COLLATION_NAME AS collation_name
FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()`).Scan(&originalSchema).Error)
		originalSchema.Name = "test schema"
		require.NoError(t, validateMySQLIdentityComparison(originalSchema))
		require.NoError(t, db.Exec(fmt.Sprintf(
			"ALTER DATABASE `%s` CHARACTER SET utf8 COLLATE utf8_general_ci",
			selectedDatabase,
		)).Error)
		defer func() {
			require.NoError(t, db.Exec(fmt.Sprintf(
				"ALTER DATABASE `%s` CHARACTER SET %s COLLATE %s",
				selectedDatabase,
				originalSchema.CharacterSet,
				originalSchema.Collation,
			)).Error)
		}()
		var narrowSchema mysqlIdentityComparison
		require.NoError(t, db.Raw(`SELECT DEFAULT_CHARACTER_SET_NAME AS character_set_name,
DEFAULT_COLLATION_NAME AS collation_name
FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()`).Scan(&narrowSchema).Error)
		narrowSchema.Name = "narrow test schema"
		require.NoError(t, validateMySQLIdentityComparison(narrowSchema))
		var originalSQLMode string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSQLMode).Error)
		require.NoError(t, db.Exec("SET SESSION sql_mode = ''").Error)
		defer func() { require.NoError(t, db.Exec("SET SESSION sql_mode = ?", originalSQLMode).Error) }()

		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
github_id varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`).Error)
		subject := "new-claim-\U0001F600"
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, site_id, github_id) VALUES (?, ?, ?)",
			1, 7, subject,
		).Error)

		require.NoError(t, preflightExternalIdentityClaims(db))
		var legacySubject string
		require.NoError(t, db.Table("users").Where("id = ?", 1).Pluck("github_id", &legacySubject).Error)
		assert.Equal(t, subject, legacySubject)

		require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
		require.NoError(t, prepareUserOAuthBindingsSiteScope(db))

		columns := map[string]struct {
			column     string
			comparison mysqlIdentityComparison
		}{
			ExternalIdentityClaim{}.TableName(): {column: "subject", comparison: mysqlIdentityComparison{
				CharacterSet: "utf8mb4", Collation: "utf8mb4_bin",
			}},
			UserOAuthBinding{}.TableName(): {column: "provider_user_id", comparison: narrowSchema},
		}
		for table, expected := range columns {
			var definition struct {
				CharacterSet string `gorm:"column:character_set_name"`
				Collation    string `gorm:"column:collation_name"`
				RowFormat    string `gorm:"column:row_format"`
			}
			require.NoError(t, db.Raw(
				`SELECT c.CHARACTER_SET_NAME AS character_set_name, c.COLLATION_NAME AS collation_name,
t.ROW_FORMAT AS row_format
FROM information_schema.COLUMNS c
JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.TABLE_NAME = c.TABLE_NAME
WHERE c.TABLE_SCHEMA = DATABASE() AND c.TABLE_NAME = ? AND c.COLUMN_NAME = ?`,
				table, expected.column,
			).Scan(&definition).Error)
			assert.Equal(t, strings.ToLower(expected.comparison.CharacterSet), strings.ToLower(definition.CharacterSet), table)
			assert.Equal(t, strings.ToLower(expected.comparison.Collation), strings.ToLower(definition.Collation), table)
			assert.Equal(t, "dynamic", strings.ToLower(definition.RowFormat), table)
		}

		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderGitHub, subject, 1)
		}))
		var claimCount int64
		require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCount).Error)
		assert.EqualValues(t, 1, claimCount)
		err = validateMySQLIdentitySubjectStorage(
			db,
			&UserOAuthBinding{},
			UserOAuthBinding{}.TableName(),
			"provider_user_id",
			"custom OAuth binding subject",
			subject,
		)
		require.ErrorContains(t, err, "cannot be represented")

		require.NoError(t, preflightExternalIdentityClaims(db))
		require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
		require.NoError(t, prepareUserOAuthBindingsSiteScope(db))
	})

	t.Run("completed wide indexes remain idempotent after large prefix is disabled", func(t *testing.T) {
		if os.Getenv("TEST_MYSQL_DISPOSABLE_INSTANCE") != "1" {
			t.Skip("set TEST_MYSQL_DISPOSABLE_INSTANCE=1 only for an isolated disposable MySQL instance; this test changes a GLOBAL variable")
		}
		type mysqlTestVariable struct {
			Name  string `gorm:"column:Variable_name"`
			Value string `gorm:"column:Value"`
		}
		var largePrefixVariables []mysqlTestVariable
		require.NoError(t, db.Raw("SHOW GLOBAL VARIABLES LIKE 'innodb_large_prefix'").Scan(&largePrefixVariables).Error)
		if len(largePrefixVariables) == 0 {
			t.Skip("MySQL version does not expose innodb_large_prefix")
		}
		originalLargePrefix := strings.ToUpper(largePrefixVariables[0].Value)
		if originalLargePrefix != "OFF" && originalLargePrefix != "ON" {
			t.Fatalf("unexpected innodb_large_prefix value %q", originalLargePrefix)
		}
		require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = ON").Error)
		defer func() {
			value := "OFF"
			if originalLargePrefix == "ON" {
				value = "ON"
			}
			require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = "+value).Error)
		}()

		require.NoError(t, db.Exec("DROP TABLE IF EXISTS user_oauth_bindings").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
		require.NoError(t, db.Set(
			"gorm:table_options",
			"ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin",
		).AutoMigrate(&ExternalIdentityClaim{}, &UserOAuthBinding{}))
		claimComplete, err := mysqlIdentityIndexStorageAlreadyPrepared(db, ExternalIdentityClaim{}.TableName())
		require.NoError(t, err)
		require.True(t, claimComplete, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
		bindingComplete, err := mysqlIdentityIndexStorageAlreadyPrepared(db, UserOAuthBinding{}.TableName())
		require.NoError(t, err)
		require.True(t, bindingComplete, mysqlShowCreate(t, db, UserOAuthBinding{}.TableName()))

		require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = OFF").Error)
		largePrefixVariables = nil
		require.NoError(t, db.Raw("SHOW GLOBAL VARIABLES LIKE 'innodb_large_prefix'").Scan(&largePrefixVariables).Error)
		require.Len(t, largePrefixVariables, 1)
		require.Equal(t, "OFF", strings.ToUpper(largePrefixVariables[0].Value))

		claimDDL := mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName())
		bindingDDL := mysqlShowCreate(t, db, UserOAuthBinding{}.TableName())
		require.NoError(t, ensureMySQLIdentityIndexStorage(
			db, &ExternalIdentityClaim{}, ExternalIdentityClaim{}.TableName(),
		))
		require.NoError(t, ensureMySQLIdentityIndexStorage(
			db, &UserOAuthBinding{}, UserOAuthBinding{}.TableName(),
		))
		require.NoError(t, db.AutoMigrate(&ExternalIdentityClaim{}, &UserOAuthBinding{}))
		assert.Equal(t, claimDDL, mysqlShowCreate(t, db, ExternalIdentityClaim{}.TableName()))
		assert.Equal(t, bindingDDL, mysqlShowCreate(t, db, UserOAuthBinding{}.TableName()))

		require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = ON").Error)
		require.NoError(t, db.Exec(
			"ALTER TABLE external_identity_claims ADD UNIQUE INDEX ux_external_identity_provider_only (provider)",
		).Error)
		require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = OFF").Error)
		err = ensureMySQLIdentityIndexStorage(db, &ExternalIdentityClaim{}, ExternalIdentityClaim{}.TableName())
		require.ErrorContains(t, err, "require large prefixes")

		require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = ON").Error)
		require.NoError(t, db.Exec("ALTER TABLE external_identity_claims DROP INDEX ux_external_identity_provider_only").Error)
		require.NoError(t, db.Exec("ALTER TABLE external_identity_claims DROP INDEX idx_external_identity_claims_user_id").Error)
		require.NoError(t, db.Exec("SET GLOBAL innodb_large_prefix = OFF").Error)
		err = ensureMySQLIdentityIndexStorage(db, &ExternalIdentityClaim{}, ExternalIdentityClaim{}.TableName())
		require.ErrorContains(t, err, "require large prefixes")
	})

	t.Run("non-strict runtime rejects built-in subjects beyond MySQL column capacity", func(t *testing.T) {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS user_oauth_bindings").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS external_identity_claims").Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
		require.NoError(t, db.Exec(`CREATE TABLE users (
id bigint NOT NULL,
site_id int NULL DEFAULT 0,
github_id varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
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
		require.NoError(t, db.Exec("INSERT INTO users (id, site_id, github_id) VALUES (1, 7, NULL)").Error)
		var originalSQLMode string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSQLMode).Error)
		require.NoError(t, db.Exec("SET SESSION sql_mode = ''").Error)
		defer func() { require.NoError(t, db.Exec("SET SESSION sql_mode = ?", originalSQLMode).Error) }()

		tooLong := strings.Repeat("s", builtInExternalIdentitySubjectMaxLength+1)
		err = db.Transaction(func(tx *gorm.DB) error {
			return BindUserExternalIdentityWithTx(tx, 1, "github_id", tooLong)
		})
		require.ErrorContains(t, err, "exceeds 191 characters")
		var githubIdIsNull bool
		require.NoError(t, db.Raw("SELECT github_id IS NULL FROM users WHERE id = ?", 1).Scan(&githubIdIsNull).Error)
		assert.True(t, githubIdIsNull)
		var claimCount int64
		require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&claimCount).Error)
		assert.Zero(t, claimCount)

		newUser := User{Username: "long-built-in-new-user", Password: "password", OidcId: tooLong}
		require.ErrorContains(t, newUser.prepareForInsert(db), "exceeds 191 characters")
	})
}

func TestMigrateExternalIdentityClaimsSiteScopePreservesExistingClaims(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &legacyGlobalExternalIdentityClaim{}))

	mainSiteUser := User{
		SiteId:   0,
		Username: "identity-migration-main",
		Password: "password",
		AffCode:  "identity-migration-main",
	}
	subSiteUser := User{
		SiteId:     7,
		Username:   "identity-migration-sub",
		Password:   "password",
		AffCode:    "identity-migration-sub",
		TelegramId: "shared-after-migration",
	}
	require.NoError(t, db.Create(&mainSiteUser).Error)
	require.NoError(t, db.Create(&subSiteUser).Error)
	require.NoError(t, db.Create(&legacyGlobalExternalIdentityClaim{
		Provider: ExternalIdentityProviderTelegram,
		Subject:  "shared-after-migration",
		UserId:   subSiteUser.Id,
	}).Error)

	require.NoError(t, preflightExternalIdentityClaims(db))
	require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
	// SQLite swaps both indexes in one DDL transaction, so every committed
	// schema has either the legacy or the site-scoped uniqueness constraint.
	assert.False(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
	assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))

	require.NoError(t, db.AutoMigrate(&ExternalIdentityClaim{}))
	assert.False(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
	assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
	require.NoError(t, finalizeExternalIdentityClaimsSiteScope(db))
	assert.False(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
	assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
	require.NoError(t, finalizeExternalIdentityClaimsSiteScope(db))

	var preserved ExternalIdentityClaim
	require.NoError(t, db.First(&preserved).Error)
	assert.Equal(t, subSiteUser.Id, preserved.UserId)
	assert.Equal(t, 7, preserved.SiteId)
	assert.Equal(t, "shared-after-migration", preserved.Subject)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(
			tx,
			ExternalIdentityProviderTelegram,
			"shared-after-migration",
			mainSiteUser.Id,
		)
	}))

	var count int64
	require.NoError(t, db.Model(&ExternalIdentityClaim{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestExternalIdentityMigrationResumesBeforeUserSiteColumnExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE users (id integer primary key, github_id text)").Error)
	require.NoError(t, db.AutoMigrate(&legacyGlobalExternalIdentityClaim{}))
	require.NoError(t, db.Exec("INSERT INTO users (id, github_id) VALUES (1, ?)", "interrupted-migration-subject").Error)
	require.NoError(t, db.Create(&legacyGlobalExternalIdentityClaim{
		Provider: ExternalIdentityProviderGitHub,
		Subject:  "interrupted-migration-subject",
		UserId:   1,
	}).Error)

	assert.False(t, db.Migrator().HasColumn(&User{}, "site_id"))
	require.NoError(t, preflightExternalIdentityClaims(db))
	require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))

	var claim ExternalIdentityClaim
	require.NoError(t, db.First(&claim).Error)
	assert.Zero(t, claim.SiteId)
	assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
}

func TestExternalIdentityMigrationBackfillsNullMainSiteScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &legacyGlobalExternalIdentityClaim{}))
	require.NoError(t, db.Exec("ALTER TABLE external_identity_claims ADD COLUMN site_id integer NULL").Error)
	owner := User{
		Username: "null-site-owner", Password: "password", AffCode: "null-site-owner",
		TelegramId: "null-site-subject",
	}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO external_identity_claims (provider, subject, user_id, site_id) VALUES (?, ?, ?, NULL)",
		ExternalIdentityProviderTelegram,
		owner.TelegramId,
		owner.Id,
	).Error)
	var nullCount int64
	require.NoError(t, db.Table(ExternalIdentityClaim{}.TableName()).Where("site_id IS NULL").Count(&nullCount).Error)
	assert.EqualValues(t, 1, nullCount)

	require.NoError(t, preflightExternalIdentityClaims(db))
	require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
	require.NoError(t, db.Table(ExternalIdentityClaim{}.TableName()).Where("site_id IS NULL").Count(&nullCount).Error)
	assert.Zero(t, nullCount)
	var siteId int
	require.NoError(t, db.Table(ExternalIdentityClaim{}.TableName()).Pluck("site_id", &siteId).Error)
	assert.Zero(t, siteId)
}

func TestExternalIdentityMigrationPreservesUnrelatedUniqueIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &legacyGlobalExternalIdentityClaim{}))
	require.NoError(t, db.Exec(
		"CREATE UNIQUE INDEX ux_external_identity_audit ON external_identity_claims (provider, subject, user_id)",
	).Error)

	user := User{Username: "index-owner", Password: "password", AffCode: "index-owner", AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&legacyGlobalExternalIdentityClaim{
		Provider: ExternalIdentityProviderGitHub,
		Subject:  "index-subject",
		UserId:   user.Id,
	}).Error)

	require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
	assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, "ux_external_identity_audit"))
	assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
	assert.False(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
}

func TestExternalIdentityMigrationRejectsPartialCanonicalIndexAndPreservesPartialLegacyIndex(t *testing.T) {
	createTable := func(t *testing.T) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.Exec(`CREATE TABLE external_identity_claims (
id integer primary key, provider text not null, site_id integer not null default 0,
subject text not null, user_id integer not null
)`).Error)
		return db
	}

	t.Run("partial canonical site index", func(t *testing.T) {
		db := createTable(t)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX idx_external_identity_subject ON external_identity_claims (provider, subject)",
		).Error)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX idx_external_identity_subject_site ON external_identity_claims (provider, site_id, subject) WHERE site_id <> 0",
		).Error)

		err := prepareExternalIdentityClaimsSiteScope(db)
		require.ErrorContains(t, err, "unexpected definition")
		assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
		assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
	})

	t.Run("partial legacy index", func(t *testing.T) {
		db := createTable(t)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX idx_external_identity_subject_site ON external_identity_claims (provider, site_id, subject)",
		).Error)
		require.NoError(t, db.Exec(
			"CREATE UNIQUE INDEX idx_external_identity_subject ON external_identity_claims (provider, subject) WHERE site_id = 0",
		).Error)

		require.NoError(t, prepareExternalIdentityClaimsSiteScope(db))
		assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex))
		assert.True(t, db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex))
	})
}

func TestExternalIdentityClaimSupportsLegacyCustomOAuthSubjectLength(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &ExternalIdentityClaim{}))

	user := User{Username: "long-subject-owner", Password: "password", AffCode: "long-subject-owner", AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	subject := strings.Repeat("s", 200)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, customOAuthExternalIdentityProvider(99), subject, user.Id)
	}))

	var claim ExternalIdentityClaim
	require.NoError(t, db.First(&claim).Error)
	assert.Equal(t, subject, claim.Subject)
	assert.Error(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, customOAuthExternalIdentityProvider(100), strings.Repeat("s", 257), user.Id)
	}))
}

func TestBuiltInIdentityPreservesExistingNonMySQLSubjectCapacity(t *testing.T) {
	truncateTables(t)

	user := User{Username: "long-built-in-subject", Password: "password", AffCode: "long-built-in-subject"}
	require.NoError(t, DB.Create(&user).Error)
	subject := strings.Repeat("s", builtInExternalIdentitySubjectMaxLength+1)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return BindUserExternalIdentityWithTx(tx, user.Id, "oidc_id", subject)
	}))

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, subject, reloaded.OidcId)
	var claim ExternalIdentityClaim
	require.NoError(t, DB.Where("provider = ? AND user_id = ?", ExternalIdentityProviderOIDC, user.Id).First(&claim).Error)
	assert.Equal(t, subject, claim.Subject)
}

func TestBuiltInIdentityBindingIsSiteScopedAndAtomic(t *testing.T) {
	truncateTables(t)

	mainUser := User{SiteId: 0, Username: "github-main", Password: "password", AffCode: "github-main"}
	subUser := User{SiteId: 2, Username: "github-sub", Password: "password", AffCode: "github-sub"}
	conflict := User{SiteId: 2, Username: "github-conflict", Password: "password", AffCode: "github-conflict"}
	require.NoError(t, DB.Create(&mainUser).Error)
	require.NoError(t, DB.Create(&subUser).Error)
	require.NoError(t, DB.Create(&conflict).Error)

	require.NoError(t, UpdateUserBindColumn(mainUser.Id, "github_id", "github-shared"))
	require.NoError(t, UpdateUserBindColumn(subUser.Id, "github_id", "github-shared"))
	err := UpdateUserBindColumn(conflict.Id, "github_id", "github-shared")
	assert.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)

	var unchanged User
	require.NoError(t, DB.First(&unchanged, conflict.Id).Error)
	assert.Empty(t, unchanged.GitHubId, "a failed claim must roll back the users-table binding")
}

func TestConcurrentBuiltInIdentityBindingHasOneWinner(t *testing.T) {
	truncateTables(t)

	first := User{Username: "github-race-one", Password: "password", AffCode: "github-race-one"}
	second := User{Username: "github-race-two", Password: "password", AffCode: "github-race-two"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, userId := range []int{first.Id, second.Id} {
		go func(id int) {
			ready.Done()
			<-start
			errs <- UpdateUserBindColumn(id, "discord_id", "discord-race")
		}(userId)
	}
	ready.Wait()
	close(start)
	firstErr, secondErr := <-errs, <-errs

	successes := 0
	conflicts := 0
	for _, bindErr := range []error{firstErr, secondErr} {
		switch {
		case bindErr == nil:
			successes++
		case errors.Is(bindErr, ErrExternalIdentityAlreadyClaimed):
			conflicts++
		default:
			require.NoError(t, bindErr)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)

	var claims int64
	require.NoError(t, DB.Model(&ExternalIdentityClaim{}).
		Where("provider = ? AND site_id = ? AND subject = ?", ExternalIdentityProviderDiscord, 0, "discord-race").
		Count(&claims).Error)
	assert.EqualValues(t, 1, claims)
	var boundUsers int64
	require.NoError(t, DB.Model(&User{}).Where("discord_id = ?", "discord-race").Count(&boundUsers).Error)
	assert.EqualValues(t, 1, boundUsers)
}
