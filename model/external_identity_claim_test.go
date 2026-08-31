package model

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		SiteId:   7,
		Username: "identity-migration-sub",
		Password: "password",
		AffCode:  "identity-migration-sub",
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
	require.NoError(t, db.Exec("CREATE TABLE users (id integer primary key)").Error)
	require.NoError(t, db.AutoMigrate(&legacyGlobalExternalIdentityClaim{}))
	require.NoError(t, db.Exec("INSERT INTO users (id) VALUES (1)").Error)
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
