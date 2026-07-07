package model

import (
	"errors"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// TestSearchRedemptionsNumericKeywordSiteScoped is a security regression for 越权: a
// numeric-keyword search (which ORs `id = ?` into the predicate) must still be confined
// to the caller's site scope, so a sub-site admin cannot find another tenant's code by
// guessing its numeric id. Locks in the explicit OR grouping in SearchRedemptions.
func TestSearchRedemptionsNumericKeywordSiteScoped(t *testing.T) {
	if err := DB.AutoMigrate(&Redemption{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const siteA, siteB = 6701, 6702
	DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&Redemption{})
	defer DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&Redemption{})

	codeA := &Redemption{SiteId: siteA, Name: "acode", Key: common.GetUUID(), Status: common.RedemptionCodeStatusEnabled, Quota: 1, CreatedTime: common.GetTimestamp()}
	if err := DB.Create(codeA).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	idKw := strconv.Itoa(codeA.Id)

	// From site B's scope, searching site A's numeric id must return nothing.
	res, total, err := SearchRedemptions(idKw, "", 0, 10, siteB)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 0 || len(res) != 0 {
		t.Fatalf("越权: numeric id search under siteB leaked a site-A code (total=%d)", total)
	}
	// From its own site, it is found.
	if _, total, _ = SearchRedemptions(idKw, "", 0, 10, siteA); total != 1 {
		t.Fatalf("own-site numeric search should find the code, total=%d", total)
	}
	// SiteScopeAll (main admin) finds it.
	if _, total, _ = SearchRedemptions(idKw, "", 0, 10, SiteScopeAll); total != 1 {
		t.Fatalf("SiteScopeAll numeric search should find the code, total=%d", total)
	}
}

// TestRedemptionWalletIntegration covers the phase-3 wallet↔redemption invariants:
// generation atomically debits the wallet (and fails wholesale when funds are short),
// void refunds exactly the original cost (原路退), redeem is site-isolated, and the
// ledger stays equal to the balance throughout (reconciliation).
func TestRedemptionWalletIntegration(t *testing.T) {
	if err := DB.AutoMigrate(&Site{}, &SiteWalletLog{}, &Redemption{}, &User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const siteA, siteB = 6601, 6602
	cleanup := func() {
		DB.Where("id IN ?", []int{siteA, siteB}).Delete(&Site{})
		DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&SiteWalletLog{})
		DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&Redemption{})
	}
	cleanup()
	defer cleanup()

	// Site A starts with 1000 厘; cost per code = 100 厘.
	if err := DB.Create(&Site{Id: siteA, Name: "A", Status: SiteStatusNormal, WalletBalance: 1000, DiscountRate: DiscountRateBase}).Error; err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if err := DB.Create(&Site{Id: siteB, Name: "B", Status: SiteStatusNormal, WalletBalance: 500, DiscountRate: DiscountRateBase}).Error; err != nil {
		t.Fatalf("seed B: %v", err)
	}
	const cost = int64(100)

	// 1. Generate 6 codes on A → debit 600; balance 1000→400.
	keys, err := GenerateRedemptions(siteA, 1, "batch", 5000, 6, 0, cost)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(keys) != 6 {
		t.Fatalf("expected 6 keys, got %d", len(keys))
	}
	if bal, _ := GetSiteWalletBalance(siteA); bal != 400 {
		t.Fatalf("after generate balance = %d, want 400", bal)
	}

	// 2. Over-spend is rejected wholesale: 5 more codes cost 500 > 400 remaining.
	if _, err := GenerateRedemptions(siteA, 1, "toomany", 5000, 5, 0, cost); !errors.Is(err, ErrInsufficientWalletBalance) {
		t.Fatalf("expected insufficient balance, got %v", err)
	}
	if bal, _ := GetSiteWalletBalance(siteA); bal != 400 {
		t.Fatalf("balance changed on failed generate: %d", bal)
	}
	var aCount int64
	DB.Model(&Redemption{}).Where("site_id = ?", siteA).Count(&aCount)
	if aCount != 6 {
		t.Fatalf("failed batch leaked codes: %d total", aCount)
	}

	// 3. Void one code → refund 100; balance 400→500. Find a code id.
	var code Redemption
	DB.Where("site_id = ?", siteA).First(&code)
	if err := VoidRedemption(code.Id, siteA, 1); err != nil {
		t.Fatalf("void: %v", err)
	}
	if bal, _ := GetSiteWalletBalance(siteA); bal != 500 {
		t.Fatalf("after void balance = %d, want 500", bal)
	}
	// Voiding again (now disabled) must fail.
	if err := VoidRedemption(code.Id, siteA, 1); err == nil {
		t.Fatal("re-voiding a disabled code must fail")
	}
	// Cross-site void is rejected by ownership.
	var codeA2 Redemption
	DB.Where("site_id = ? AND status = ?", siteA, common.RedemptionCodeStatusEnabled).First(&codeA2)
	if err := VoidRedemption(codeA2.Id, siteB, 1); err == nil {
		t.Fatal("voiding site-A code under site-B scope must be rejected")
	}

	// 4. Redeem is site-isolated. Create a user on site A and redeem an A code from A (ok)
	// but the same code is invalid from site B.
	pw, _ := common.Password2Hash("x")
	u := &User{Username: "redeemer", SiteId: siteA, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "redeemaff"}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	defer DB.Where("id = ?", u.Id).Delete(&User{})
	// An enabled code from site A.
	var liveCode Redemption
	DB.Where("site_id = ? AND status = ?", siteA, common.RedemptionCodeStatusEnabled).First(&liveCode)
	if _, err := RedeemForSite(liveCode.Key, u.Id, siteB); err == nil {
		t.Fatal("CROSS-SITE: redeeming a site-A code from site B must be invalid")
	}
	if q, err := RedeemForSite(liveCode.Key, u.Id, siteA); err != nil || q != 5000 {
		t.Fatalf("redeem on own site should grant 5000 quota: q=%d err=%v", q, err)
	}

	// 5. Reconciliation: balance == ledger sum for both sites (initial balances were set
	// directly, so initial + Σledger == balance). Verify ledger-tracked deltas reconcile.
	// Site A ledger deltas: -600 (gen) +100 (void) = -500; 1000 + (-500) = 500 == balance.
	sumA, _ := SumSiteWalletLogAmount(siteA)
	balA, _ := GetSiteWalletBalance(siteA)
	if 1000+sumA != balA {
		t.Fatalf("site A reconcile: 1000 + Σ(%d) = %d != balance %d", sumA, 1000+sumA, balA)
	}
	// ReconcileSiteWallets includes both sites; site B had no wallet activity (sum 0),
	// so its directly-seeded balance won't equal sum — that's expected for this seeded
	// test (in production the initial recharge is itself a ledger entry). Just assert the
	// function runs and returns both sites.
	results, err := ReconcileSiteWallets()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var sawA bool
	for _, r := range results {
		if r.SiteId == siteA {
			sawA = true
			if r.LedgerSum != sumA || r.Balance != balA {
				t.Fatalf("reconcile A mismatch: %+v", r)
			}
		}
	}
	if !sawA {
		t.Fatal("reconcile did not include site A")
	}
}
