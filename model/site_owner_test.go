package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateSiteOwnerPromotionAndWallet verifies that creating a sub-site promotes its
// main-site owner to a sub-site admin bound to the new site, rejects platform-admin
// owners, and that main-admin wallet ops keep the ledger exactly equal to the balance
// (the production reconciliation invariant, since every change is ledger-backed).
func TestCreateSiteOwnerPromotionAndWallet(t *testing.T) {
	if err := DB.AutoMigrate(&Site{}, &SiteDomain{}, &SiteWalletLog{}, &User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pw, _ := common.Password2Hash("x")
	owner := &User{Username: "agent_owner", SiteId: 0, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Password: pw, AffCode: "owneraff1"}
	if err := DB.Create(owner).Error; err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	defer DB.Where("id = ?", owner.Id).Delete(&User{})

	site := &Site{Name: "promo", OwnerUsername: "agent_owner", Domains: []string{"promo.example.com"}, DiscountRate: 7000}
	if err := CreateSite(site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	defer func() {
		DB.Where("id = ?", site.Id).Delete(&Site{})
		DB.Where("site_id = ?", site.Id).Delete(&SiteDomain{})
		DB.Where("site_id = ?", site.Id).Delete(&SiteWalletLog{})
	}()

	// Owner promoted to sub-site admin and bound to the new site.
	var promoted User
	DB.First(&promoted, owner.Id)
	if promoted.Role != common.RoleSubSiteAdmin {
		t.Fatalf("owner role = %d, want RoleSubSiteAdmin(%d)", promoted.Role, common.RoleSubSiteAdmin)
	}
	if promoted.SiteId != site.Id {
		t.Fatalf("owner site_id = %d, want %d", promoted.SiteId, site.Id)
	}

	// A platform admin cannot be a sub-site owner.
	pw2, _ := common.Password2Hash("y")
	adminU := &User{Username: "admin_owner", SiteId: 0, Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Password: pw2, AffCode: "owneraff2"}
	DB.Create(adminU)
	defer DB.Where("id = ?", adminU.Id).Delete(&User{})
	if err := CreateSite(&Site{Name: "x", OwnerUsername: "admin_owner", Domains: []string{"x.example.com"}}); err == nil {
		t.Fatal("platform-admin owner must be rejected")
	}

	// Wallet ops: recharge +10000, adjust -3000, adjust +500 → 7500; ledger == balance.
	if err := RechargeSiteWallet(site.Id, 10000, "进货", owner.Id); err != nil {
		t.Fatalf("recharge: %v", err)
	}
	if err := AdjustSiteWallet(site.Id, -3000, "扣减", owner.Id); err != nil {
		t.Fatalf("adjust down: %v", err)
	}
	if err := AdjustSiteWallet(site.Id, 500, "补偿", owner.Id); err != nil {
		t.Fatalf("adjust up: %v", err)
	}
	bal, _ := GetSiteWalletBalance(site.Id)
	if bal != 7500 {
		t.Fatalf("balance = %d, want 7500", bal)
	}
	sum, _ := SumSiteWalletLogAmount(site.Id)
	if sum != bal {
		t.Fatalf("reconciliation: ledger %d != balance %d", sum, bal)
	}

	// Manual adjust requires a remark; over-deduction fails closed.
	if err := AdjustSiteWallet(site.Id, 100, "", owner.Id); err == nil {
		t.Fatal("adjust without remark must fail")
	}
	if err := AdjustSiteWallet(site.Id, -999999, "big", owner.Id); !errors.Is(err, ErrInsufficientWalletBalance) {
		t.Fatalf("over-deduct should be insufficient, got %v", err)
	}
}

func TestCreateSiteMovesOwnerIdentityScopeAndAuthVersion(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Site{},
		&SiteDomain{},
		&User{},
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&ExternalIdentityClaim{},
	))
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	owner := User{
		Username:    "identity_site_owner",
		Password:    "password",
		AffCode:     "identity-site-owner",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		AuthVersion: 3,
	}
	require.NoError(t, DB.Create(&owner).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, owner.Id) })
	require.NoError(t, UpdateUserBindColumn(owner.Id, "github_id", "site-owner-github"))
	provider := createCustomOAuthProviderForBindingTest(t, DB)
	require.NoError(t, CreateUserOAuthBinding(&UserOAuthBinding{
		UserId: owner.Id, ProviderId: provider.Id, ProviderUserId: "site-owner-custom",
	}))
	t.Cleanup(func() {
		DB.Where("user_id = ?", owner.Id).Delete(&UserOAuthBinding{})
		DB.Where("user_id = ?", owner.Id).Delete(&ExternalIdentityClaim{})
	})

	site := Site{
		Name:          "identity-owner-site",
		OwnerUsername: owner.Username,
		Domains:       []string{"identity-owner.example.com"},
	}
	require.NoError(t, CreateSite(&site))
	t.Cleanup(func() {
		DB.Where("site_id = ?", site.Id).Delete(&SiteDomain{})
		DB.Delete(&Site{}, site.Id)
	})

	var moved User
	require.NoError(t, DB.First(&moved, owner.Id).Error)
	assert.Equal(t, site.Id, moved.SiteId)
	assert.Equal(t, int64(4), moved.AuthVersion)

	var claims []ExternalIdentityClaim
	require.NoError(t, DB.Where("user_id = ?", owner.Id).Order("provider").Find(&claims).Error)
	require.Len(t, claims, 2)
	for _, claim := range claims {
		assert.Equal(t, site.Id, claim.SiteId)
	}
	var binding UserOAuthBinding
	require.NoError(t, DB.Where("user_id = ? AND provider_id = ?", owner.Id, provider.Id).First(&binding).Error)
	assert.Equal(t, site.Id, binding.SiteId)
}
