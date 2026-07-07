package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransferAffQuotaToQuotaConditionalUpdate locks in the atomic-transfer contract:
// the aff_quota >= ? guard rejects an over-transfer without touching any balance, a
// valid transfer moves exactly the requested amount, and the targeted two-column
// update never rewrites unrelated usage counters (used_quota / request_count), which
// a stale full-row Save previously could clobber.
func TestTransferAffQuotaToQuotaConditionalUpdate(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&User{}))
	unit := int(common.QuotaPerUnit)

	pw, _ := common.Password2Hash("x")
	u := &User{
		Username: "afftransu", Password: pw, Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AffCode: "afftransaff",
		Quota: 100, AffQuota: unit + 100, UsedQuota: 777, RequestCount: 42,
	}
	require.NoError(t, DB.Create(u).Error)
	defer DB.Where("id = ?", u.Id).Delete(&User{})

	// Below the per-transfer minimum is rejected up front.
	require.Error(t, u.TransferAffQuotaToQuota(unit-1))

	// More than the available aff_quota loses the conditional update.
	require.Error(t, u.TransferAffQuotaToQuota(unit*2))
	var got User
	require.NoError(t, DB.First(&got, u.Id).Error)
	assert.Equal(t, 100, got.Quota, "failed transfer must not change quota")
	assert.Equal(t, unit+100, got.AffQuota, "failed transfer must not change aff_quota")

	// A valid transfer moves exactly `quota` and leaves usage counters alone.
	require.NoError(t, u.TransferAffQuotaToQuota(unit))
	require.NoError(t, DB.First(&got, u.Id).Error)
	assert.Equal(t, 100+unit, got.Quota)
	assert.Equal(t, 100, got.AffQuota)
	assert.Equal(t, 777, got.UsedQuota, "transfer must not rewrite used_quota")
	assert.Equal(t, 42, got.RequestCount, "transfer must not rewrite request_count")
}

// TestResetUserPasswordByEmailRequiresExactlyOneMatch: (site_id, email) has no unique
// index, so duplicates can exist; a reset token holder must not be able to take over
// every duplicate at once, and a reset against a non-existent account must fail loudly
// instead of silently succeeding.
func TestResetUserPasswordByEmailRequiresExactlyOneMatch(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&User{}))
	const siteId = 7801
	cleanup := func() { DB.Unscoped().Where("site_id = ?", siteId).Delete(&User{}) }
	cleanup()
	defer cleanup()

	oldPw, _ := common.Password2Hash("old-password-1")
	for i, email := range []string{"dup@example.com", "dup@example.com", "solo@example.com"} {
		require.NoError(t, DB.Create(&User{
			Username: fmt.Sprintf("resetu%d", i), SiteId: siteId, Email: email,
			Password: oldPw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser,
			AffCode: fmt.Sprintf("resetaff%d", i),
		}).Error)
	}

	// Duplicate same-site emails: reset must refuse and change nothing.
	require.Error(t, ResetUserPasswordByEmail("dup@example.com", "new-password-9", siteId))
	var dups []User
	require.NoError(t, DB.Where("email = ? AND site_id = ?", "dup@example.com", siteId).Find(&dups).Error)
	require.Len(t, dups, 2)
	for _, d := range dups {
		assert.True(t, common.ValidatePasswordAndHash("old-password-1", d.Password), "duplicate account password must be untouched")
	}

	// No matching account: fail instead of silently succeeding.
	require.Error(t, ResetUserPasswordByEmail("missing@example.com", "new-password-9", siteId))

	// Exactly one match: reset succeeds for that account only.
	require.NoError(t, ResetUserPasswordByEmail("solo@example.com", "new-password-9", siteId))
	var solo User
	require.NoError(t, DB.Where("email = ? AND site_id = ?", "solo@example.com", siteId).First(&solo).Error)
	assert.True(t, common.ValidatePasswordAndHash("new-password-9", solo.Password))
}

// TestBindUserEmailEnforcesSameSiteUniqueness: the bind must refuse an email already
// held by another same-site account (checked transactionally, right before the write),
// while the same address on a DIFFERENT site, or re-binding one's own address, is fine.
func TestBindUserEmailEnforcesSameSiteUniqueness(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&User{}))
	const siteA, siteB = 7901, 7902
	cleanup := func() { DB.Unscoped().Where("site_id IN ?", []int{siteA, siteB}).Delete(&User{}) }
	cleanup()
	defer cleanup()

	pw, _ := common.Password2Hash("x")
	holder := &User{Username: "bindholder", SiteId: siteA, Email: "taken@example.com", Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "bindaff1"}
	binder := &User{Username: "binduser", SiteId: siteA, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "bindaff2"}
	other := &User{Username: "bindother", SiteId: siteB, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "bindaff3"}
	for _, u := range []*User{holder, binder, other} {
		require.NoError(t, DB.Create(u).Error)
	}

	// Same site + already held by another account → rejected, email unchanged.
	require.Error(t, BindUserEmail(binder.Id, "taken@example.com"))
	var got User
	require.NoError(t, DB.First(&got, binder.Id).Error)
	assert.Empty(t, got.Email)

	// Different site may hold the same address (per-site uniqueness).
	require.NoError(t, BindUserEmail(other.Id, "taken@example.com"))

	// Fresh address binds; re-binding one's own address is a no-op success.
	require.NoError(t, BindUserEmail(binder.Id, "fresh@example.com"))
	require.NoError(t, BindUserEmail(binder.Id, "fresh@example.com"))
	require.NoError(t, DB.First(&got, binder.Id).Error)
	assert.Equal(t, "fresh@example.com", got.Email)
}
