package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
