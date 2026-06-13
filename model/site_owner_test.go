package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
