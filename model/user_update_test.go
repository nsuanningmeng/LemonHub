package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserUpdateTestState(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})
}

func createUserBindTestUser(t *testing.T) User {
	t.Helper()
	user := User{
		Username:    "bind-test-user",
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     "bind-test-aff-code",
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func TestUserUpdateDoesNotOverwriteConcurrentAccountingOrTokenChanges(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:              1,
		Username:        "quota-race-user",
		Password:        "password",
		DisplayName:     "before",
		Status:          common.UserStatusEnabled,
		Quota:           1000,
		UsedQuota:       20,
		RequestCount:    3,
		AffCount:        2,
		AffQuota:        800,
		AffHistoryQuota: 1200,
	}
	user.SetAccessToken("old-token")
	require.NoError(t, DB.Create(&user).Error)

	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 400),
		"used_quota":    gorm.Expr("used_quota + ?", 400),
		"request_count": gorm.Expr("request_count + ?", 1),
		"aff_count":     gorm.Expr("aff_count + ?", 1),
		"aff_quota":     gorm.Expr("aff_quota - ?", 500),
		"aff_history":   gorm.Expr("aff_history + ?", 500),
		"access_token":  "rotated-token",
	}).Error)

	staleUser.DisplayName = "after"
	require.NoError(t, staleUser.Update(false))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "after", got.DisplayName)
	assert.Equal(t, 600, got.Quota)
	assert.Equal(t, 420, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
	assert.Equal(t, 3, got.AffCount)
	assert.Equal(t, 300, got.AffQuota)
	assert.Equal(t, 1700, got.AffHistoryQuota)
	assert.Equal(t, "rotated-token", got.GetAccessToken())
}

func TestUsageAccountingSupportsSignedDirectAndBatchDeltas(t *testing.T) {
	setupUserUpdateTestState(t)
	resetBatchUpdateTestState(t)

	user := User{
		Id:           10,
		Username:     "usage-adjustment-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		UsedQuota:    1000,
		RequestCount: 3,
	}
	channel := Channel{
		Id:        10,
		Name:      "usage-adjustment-channel",
		Key:       "sk-test",
		Status:    common.ChannelStatusEnabled,
		UsedQuota: 1000,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&channel).Error)

	UpdateUserUsedQuota(user.Id, -200)
	UpdateUserUsedQuota(user.Id, 50)
	UpdateChannelUsedQuota(channel.Id, -200)
	UpdateChannelUsedQuota(channel.Id, 50)

	var got User
	require.NoError(t, DB.Select("used_quota", "request_count").First(&got, user.Id).Error)
	assert.Equal(t, 850, got.UsedQuota)
	assert.Equal(t, 3, got.RequestCount)
	var gotChannel Channel
	require.NoError(t, DB.Select("used_quota").First(&gotChannel, channel.Id).Error)
	assert.Equal(t, int64(850), gotChannel.UsedQuota)

	common.BatchUpdateEnabled = true
	UpdateUserUsedQuota(user.Id, 400)
	UpdateUserUsedQuota(user.Id, -100)
	UpdateChannelUsedQuota(channel.Id, 400)
	UpdateChannelUsedQuota(channel.Id, -100)

	require.NoError(t, DB.Select("used_quota", "request_count").First(&got, user.Id).Error)
	assert.Equal(t, 850, got.UsedQuota, "batch deltas must remain queued until flush")
	assert.Equal(t, 3, got.RequestCount)
	require.NoError(t, DB.Select("used_quota").First(&gotChannel, channel.Id).Error)
	assert.Equal(t, int64(850), gotChannel.UsedQuota, "batch deltas must remain queued until flush")

	batchUpdate()
	require.NoError(t, DB.Select("used_quota", "request_count").First(&got, user.Id).Error)
	assert.Equal(t, 1150, got.UsedQuota)
	assert.Equal(t, 3, got.RequestCount)
	require.NoError(t, DB.Select("used_quota").First(&gotChannel, channel.Id).Error)
	assert.Equal(t, int64(1150), gotChannel.UsedQuota)
}

func TestUpdateUserAccessTokenOnlyUpdatesAccessToken(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:              2,
		Username:        "token-rotation-user",
		Password:        "password",
		DisplayName:     "before",
		Status:          common.UserStatusEnabled,
		Quota:           1000,
		AffQuota:        800,
		AffHistoryQuota: 1200,
	}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":        gorm.Expr("quota + ?", 500),
		"aff_quota":    gorm.Expr("aff_quota - ?", 500),
		"display_name": "concurrent-update",
	}).Error)

	require.NoError(t, UpdateUserAccessToken(user.Id, "rotated-token"))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "rotated-token", got.GetAccessToken())
	assert.Equal(t, "concurrent-update", got.DisplayName)
	assert.Equal(t, 1500, got.Quota)
	assert.Equal(t, 300, got.AffQuota)
	assert.Equal(t, 1200, got.AffHistoryQuota)
}

func TestUpdateUserAccessTokenRejectsSoftDeletedUser(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:       3,
		Username: "deleted-token-rotation-user",
		Password: "password",
		Status:   common.UserStatusEnabled,
	}
	user.SetAccessToken("old-token")
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Delete(&user).Error)

	err := UpdateUserAccessToken(user.Id, "orphaned-token")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	var got User
	require.NoError(t, DB.Unscoped().First(&got, user.Id).Error)
	assert.Equal(t, "old-token", got.GetAccessToken())
}

func TestUpdateUserSettingOnlyUpdatesSetting(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:           2,
		Username:     "setting-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 250),
		"used_quota":    gorm.Expr("used_quota + ?", 250),
		"request_count": gorm.Expr("request_count + ?", 1),
	}).Error)

	require.NoError(t, UpdateUserSetting(user.Id, dto.UserSetting{Language: "zh"}))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 750, got.Quota)
	assert.Equal(t, 270, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
	assert.Equal(t, "zh", got.GetSetting().Language)
}

func TestEnsureEmailAvailableIsCaseInsensitiveWithinSite(t *testing.T) {
	setupUserUpdateTestState(t)
	const siteA, siteB = 31, 32

	user := User{
		Username: "existing",
		Password: "old-password",
		Email:    "Taken@Example.com",
		SiteId:   siteA,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(&user).Error)

	require.ErrorIs(t, EnsureEmailAvailable(" taken@example.COM ", siteA, 0), ErrEmailAlreadyTaken)
	require.NoError(t, EnsureEmailAvailable("taken@example.com", siteA, user.Id))
	require.NoError(t, EnsureEmailAvailable("taken@example.com", siteB, 0))
}

func TestInsertRejectsCaseInsensitiveDuplicateEmailWithinSite(t *testing.T) {
	setupUserUpdateTestState(t)
	const siteId = 41

	require.NoError(t, DB.Create(&User{
		Username: "existing",
		Password: "old-password",
		Email:    "taken@example.com",
		SiteId:   siteId,
		Status:   common.UserStatusEnabled,
	}).Error)

	user := &User{
		Username: "oauth-user",
		Email:    "TAKEN@example.com",
		SiteId:   siteId,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}

	require.ErrorIs(t, user.Insert(0), ErrEmailAlreadyTaken)
	var count int64
	require.NoError(t, DB.Model(&User{}).Where("username = ? AND site_id = ?", "oauth-user", siteId).Count(&count).Error)
	assert.Zero(t, count)
}

func TestInsertAllowsSameEmailOnDifferentSiteAndNormalizesIt(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "site-a-user",
		Password: "old-password",
		Email:    "same@example.com",
		SiteId:   51,
		Status:   common.UserStatusEnabled,
	}).Error)

	user := &User{
		Username: "site-b-user",
		Email:    " SAME@Example.COM ",
		SiteId:   52,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, "same@example.com", stored.Email)
}

func TestInsertKeepsBlankPasswordForPasswordlessUser(t *testing.T) {
	setupUserUpdateTestState(t)

	user := &User{
		Username: "passwordless-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}

	require.NoError(t, user.Insert(0))

	var stored User
	require.NoError(t, DB.Where("username = ?", user.Username).First(&stored).Error)
	assert.Empty(t, stored.Password)
}

func TestUpdateUserBindColumnOnlyTouchesTheBindingColumn(t *testing.T) {
	truncateTables(t)

	user := createUserBindTestUser(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"role":   common.RoleAdminUser,
		"status": common.UserStatusEnabled,
		"group":  "vip",
	}).Error)

	require.NoError(t, UpdateUserBindColumn(user.Id, "github_id", "gh-12345"))

	reloaded, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "gh-12345", reloaded.GitHubId)
	assert.Equal(t, common.RoleAdminUser, reloaded.Role)
	assert.Equal(t, common.UserStatusEnabled, reloaded.Status)
	assert.Equal(t, "vip", reloaded.Group)
}

func TestUpdateUserBindColumnPreservesRestrictiveChange(t *testing.T) {
	truncateTables(t)

	user := createUserBindTestUser(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).
		Update("status", common.UserStatusDisabled).Error)
	require.NoError(t, UpdateUserBindColumn(user.Id, "wechat_id", "wx-open-id"))

	reloaded, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "wx-open-id", reloaded.WeChatId)
	assert.Equal(t, common.UserStatusDisabled, reloaded.Status)
}

func TestUserUpdateDoesNotRestoreReleasedExternalIdentity(t *testing.T) {
	truncateTables(t)

	user := createUserBindTestUser(t)
	require.NoError(t, UpdateUserBindColumn(user.Id, "github_id", "github-old"))
	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)

	require.NoError(t, UpdateUserBindColumn(user.Id, "github_id", "github-new"))
	staleUser.DisplayName = "profile-update"
	require.NoError(t, staleUser.Update(false))

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, "github-new", reloaded.GitHubId)
	assert.Equal(t, "profile-update", reloaded.DisplayName)
	var claims []ExternalIdentityClaim
	require.NoError(t, DB.Where("provider = ? AND user_id = ?", ExternalIdentityProviderGitHub, user.Id).Find(&claims).Error)
	require.Len(t, claims, 1)
	assert.Equal(t, "github-new", claims[0].Subject)
}

func TestUserUpdateDoesNotMoveAccountBackToStaleSite(t *testing.T) {
	truncateTables(t)

	user := createUserBindTestUser(t)
	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("site_id", 77).Error)

	staleUser.DisplayName = "profile-update"
	require.NoError(t, staleUser.Update(false))

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 77, reloaded.SiteId)
	assert.Equal(t, "profile-update", reloaded.DisplayName)
}

func TestUserUpdateRejectsSnapshotOlderThanAuthorizationChange(t *testing.T) {
	setupUserUpdateTestState(t)

	user := createUserBindTestUser(t)
	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		if _, err := IncrementUserAuthVersionWithTx(tx, user.Id); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error
	}))

	staleUser.DisplayName = "must-not-commit"
	assert.ErrorIs(t, staleUser.Update(false), ErrStaleUserUpdate)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, reloaded.Status)
	assert.NotEqual(t, "must-not-commit", reloaded.DisplayName)
}

func TestUpdateUserBindColumnRejectsSoftDeletedUser(t *testing.T) {
	truncateTables(t)

	user := createUserBindTestUser(t)
	require.NoError(t, DB.Delete(&user).Error)

	err := UpdateUserBindColumn(user.Id, "github_id", "gh-after-delete")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var deleted User
	require.NoError(t, DB.Unscoped().First(&deleted, user.Id).Error)
	assert.Empty(t, deleted.GitHubId)
}

func TestUpdateUserBindColumnRejectsNonWhitelistedColumns(t *testing.T) {
	truncateTables(t)

	user := createUserBindTestUser(t)
	for _, column := range []string{"role", "status", "group", "quota", "username", "password", "id"} {
		assert.Error(t, UpdateUserBindColumn(user.Id, column, "1"), "column %s must be rejected", column)
	}
	assert.Error(t, UpdateUserBindColumn(user.Id, "github_id; DROP TABLE users", "x"))
	assert.Error(t, UpdateUserBindColumn(0, "github_id", "x"))
}

func TestValidateAndFillRejectsPasswordlessUser(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "passwordless-user",
		Password: "",
		Status:   common.UserStatusEnabled,
	}).Error)

	loginUser := User{
		Username: "passwordless-user",
		Password: "NewPassword123",
	}
	require.ErrorIs(t, loginUser.ValidateAndFill(), ErrInvalidCredentials)

	var stored User
	require.NoError(t, DB.Where("username = ?", "passwordless-user").First(&stored).Error)
	assert.Empty(t, stored.Password)
}

func TestResetUserPasswordByEmailUsesCaseInsensitiveSameSiteMatch(t *testing.T) {
	setupUserUpdateTestState(t)
	const siteId = 61

	require.NoError(t, DB.Create(&User{
		Username: "duplicate-1",
		Password: "old-1",
		Email:    "legacy@example.com",
		AffCode:  "dupe1",
		SiteId:   siteId,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Username: "duplicate-2",
		Password: "old-2",
		Email:    "LEGACY@example.com",
		AffCode:  "dupe2",
		SiteId:   siteId,
		Status:   common.UserStatusEnabled,
	}).Error)

	require.ErrorIs(t, ResetUserPasswordByEmail("legacy@example.com", "NewPassword123", siteId), ErrEmailAmbiguous)

	var duplicates []User
	require.NoError(t, DB.Where("LOWER(email) = ? AND site_id = ?", "legacy@example.com", siteId).Order("username asc").Find(&duplicates).Error)
	require.Len(t, duplicates, 2)
	assert.Equal(t, "old-1", duplicates[0].Password)
	assert.Equal(t, "old-2", duplicates[1].Password)

	require.NoError(t, DB.Create(&User{
		Username: "unique",
		Password: "old",
		Email:    "unique@example.com",
		AffCode:  "unique",
		SiteId:   siteId,
		Status:   common.UserStatusEnabled,
	}).Error)

	require.NoError(t, ResetUserPasswordByEmail(" UNIQUE@example.com ", "NewPassword123", siteId))
	var unique User
	require.NoError(t, DB.Where("username = ? AND site_id = ?", "unique", siteId).First(&unique).Error)
	assert.True(t, common.ValidatePasswordAndHash("NewPassword123", unique.Password))
	require.ErrorIs(t, ResetUserPasswordByEmail("missing@example.com", "NewPassword123", siteId), ErrEmailNotFound)
}
