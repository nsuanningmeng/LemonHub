package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// TestCompleteEpayTopUpSettlement covers the phase-4 online-recharge settlement core:
// a sub-site order credits the user AND debits the agent wallet atomically (flow type=4);
// an insufficient wallet parks the order for manual review and credits NOTHING; settlement
// is idempotent (duplicate callbacks never double-credit); and a main-site order credits
// the user with no wallet involvement.
func TestCompleteEpayTopUpSettlement(t *testing.T) {
	if err := DB.AutoMigrate(&Site{}, &SiteWalletLog{}, &TopUp{}, &User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const siteA, siteB = 5501, 5502
	cleanup := func() {
		DB.Where("id IN ?", []int{siteA, siteB}).Delete(&Site{})
		DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&SiteWalletLog{})
		DB.Where("trade_no LIKE ?", "P4TEST%").Delete(&TopUp{})
	}
	cleanup()
	defer cleanup()

	// QuotaPerUnit = 500000, so Amount=10 → 5,000,000 quota.
	const amount = int64(10)
	wantQuota := int(amount * int64(common.QuotaPerUnit))

	mkUser := func(name string, site int) *User {
		pw, _ := common.Password2Hash("x")
		u := &User{Username: name, SiteId: site, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "p4" + name}
		if err := DB.Create(u).Error; err != nil {
			t.Fatalf("user %s: %v", name, err)
		}
		return u
	}
	mkOrder := func(tradeNo string, site, userId int) {
		o := &TopUp{SiteId: site, UserId: userId, Amount: amount, Money: 100, TradeNo: tradeNo,
			PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
		if err := DB.Create(o).Error; err != nil {
			t.Fatalf("order %s: %v", tradeNo, err)
		}
	}
	userQuota := func(id int) int {
		var u User
		DB.Select("quota").First(&u, id)
		return u.Quota
	}

	// --- Case 1: sub-site, sufficient wallet → settle + atomic wallet debit. ---
	DB.Create(&Site{Id: siteA, Name: "A", Status: SiteStatusNormal, WalletBalance: 100000, DiscountRate: DiscountRateBase})
	uA := mkUser("p4ua", siteA)
	defer DB.Where("id = ?", uA.Id).Delete(&User{})
	mkOrder("P4TESTA", siteA, uA.Id)
	const costA = int64(70000)

	status, added, err := CompleteEpayTopUp("P4TESTA", costA, 1)
	if err != nil {
		t.Fatalf("settle A: %v", err)
	}
	if status != common.TopUpStatusSuccess || added != wantQuota {
		t.Fatalf("settle A status=%s added=%d, want success/%d", status, added, wantQuota)
	}
	if q := userQuota(uA.Id); q != wantQuota {
		t.Fatalf("user A quota=%d, want %d", q, wantQuota)
	}
	if bal, _ := GetSiteWalletBalance(siteA); bal != 100000-costA {
		t.Fatalf("site A wallet=%d, want %d", bal, 100000-costA)
	}
	// Flow record type=4 written.
	var flowCount int64
	DB.Model(&SiteWalletLog{}).Where("site_id = ? AND type = ?", siteA, WalletLogTypeTopupDeduct).Count(&flowCount)
	if flowCount != 1 {
		t.Fatalf("expected 1 type-4 flow, got %d", flowCount)
	}

	// --- Case 1b: idempotency — a duplicate callback must not double-credit. ---
	status, added, err = CompleteEpayTopUp("P4TESTA", costA, 1)
	if err != nil {
		t.Fatalf("settle A dup: %v", err)
	}
	if added != 0 {
		t.Fatalf("duplicate callback credited again: added=%d", added)
	}
	if q := userQuota(uA.Id); q != wantQuota {
		t.Fatalf("user A quota changed on duplicate: %d", q)
	}
	if bal, _ := GetSiteWalletBalance(siteA); bal != 100000-costA {
		t.Fatalf("wallet changed on duplicate: %d", bal)
	}

	// --- Case 2: sub-site, insufficient wallet → manual_review, credit nothing. ---
	DB.Create(&Site{Id: siteB, Name: "B", Status: SiteStatusNormal, WalletBalance: 100, DiscountRate: DiscountRateBase})
	uB := mkUser("p4ub", siteB)
	defer DB.Where("id = ?", uB.Id).Delete(&User{})
	mkOrder("P4TESTB", siteB, uB.Id)

	status, added, err = CompleteEpayTopUp("P4TESTB", costA, 1) // cost 70000 > balance 100
	if err != nil {
		t.Fatalf("settle B: %v", err)
	}
	if status != TopUpStatusManualReview || added != 0 {
		t.Fatalf("settle B status=%s added=%d, want manual_review/0", status, added)
	}
	if q := userQuota(uB.Id); q != 0 {
		t.Fatalf("manual-review user must not be credited, quota=%d", q)
	}
	if bal, _ := GetSiteWalletBalance(siteB); bal != 100 {
		t.Fatalf("manual-review must not touch wallet, bal=%d", bal)
	}
	var orderB TopUp
	DB.Where("trade_no = ?", "P4TESTB").First(&orderB)
	if orderB.Status != TopUpStatusManualReview {
		t.Fatalf("order B status=%s, want manual_review", orderB.Status)
	}

	// --- Case 3: main-site order (site_id=0) → credit, no wallet. ---
	uM := mkUser("p4um", 0)
	defer DB.Where("id = ?", uM.Id).Delete(&User{})
	mkOrder("P4TESTM", 0, uM.Id)
	status, added, err = CompleteEpayTopUp("P4TESTM", 0, 1)
	if err != nil {
		t.Fatalf("settle M: %v", err)
	}
	if status != common.TopUpStatusSuccess || added != wantQuota {
		t.Fatalf("settle M status=%s added=%d", status, added)
	}
	if q := userQuota(uM.Id); q != wantQuota {
		t.Fatalf("main-site user quota=%d, want %d", q, wantQuota)
	}
}
