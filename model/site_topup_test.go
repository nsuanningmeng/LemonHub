package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopUpSettlementFailClosedAndRetry covers the codex-flagged robustness paths:
// a sub-site order whose wholesale cost cannot be resolved (costMilli<=0) must FAIL CLOSED
// (never free-credit the user, order stays pending for retry); an insufficient wallet parks
// it for manual review; and a platform admin can re-settle it after the agent funds the
// wallet (RetryManualReviewTopUp).
func TestTopUpSettlementFailClosedAndRetry(t *testing.T) {
	if err := DB.AutoMigrate(&Site{}, &SiteWalletLog{}, &TopUp{}, &User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const siteC = 5503
	cleanup := func() {
		DB.Where("id = ?", siteC).Delete(&Site{})
		DB.Where("site_id = ?", siteC).Delete(&SiteWalletLog{})
		DB.Where("trade_no = ?", "P4RETRY").Delete(&TopUp{})
	}
	cleanup()
	defer cleanup()

	const amount = int64(10)
	wantQuota := int(amount * int64(common.QuotaPerUnit))
	pw, _ := common.Password2Hash("x")
	uC := &User{Username: "p4retryu", SiteId: siteC, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "p4retryaff"}
	if err := DB.Create(uC).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	defer DB.Where("id = ?", uC.Id).Delete(&User{})
	DB.Create(&Site{Id: siteC, Name: "C", Status: SiteStatusNormal, WalletBalance: 0, DiscountRate: DiscountRateBase})
	DB.Create(&TopUp{SiteId: siteC, UserId: uC.Id, Amount: amount, Money: 100, TradeNo: "P4RETRY",
		PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()})
	uQuota := func() int {
		var u User
		DB.Select("quota").First(&u, uC.Id)
		return u.Quota
	}
	orderStatus := func() string {
		var o TopUp
		DB.Where("trade_no = ?", "P4RETRY").First(&o)
		return o.Status
	}

	// 1. Fail-closed: sub-site order settled with costMilli=0 must NOT credit the user.
	if _, added, err := CompleteEpayTopUp("P4RETRY", 0, 1); !errors.Is(err, ErrSiteTopUpUnresolved) || added != 0 {
		t.Fatalf("cost=0 sub-site settle must fail closed, got added=%d err=%v", added, err)
	}
	if uQuota() != 0 {
		t.Fatalf("fail-closed must not credit, quota=%d", uQuota())
	}
	if orderStatus() != common.TopUpStatusPending {
		t.Fatalf("fail-closed must keep order pending, got %s", orderStatus())
	}

	// 2. Insufficient wallet (0 < 70000) → manual_review, credit nothing.
	if status, added, err := CompleteEpayTopUp("P4RETRY", 70000, 1); err != nil || status != TopUpStatusManualReview || added != 0 {
		t.Fatalf("insufficient settle want manual_review/0, got %s/%d err=%v", status, added, err)
	}
	if uQuota() != 0 {
		t.Fatalf("manual_review must not credit, quota=%d", uQuota())
	}

	// 3. Admin funds the wallet and retries → settles (credit + debit).
	if err := RechargeSiteWallet(siteC, 100000, "fund", 1); err != nil {
		t.Fatalf("fund: %v", err)
	}
	if status, added, err := RetryManualReviewTopUp("P4RETRY", 70000, 1); err != nil || status != common.TopUpStatusSuccess || added != wantQuota {
		t.Fatalf("retry want success/%d, got %s/%d err=%v", wantQuota, status, added, err)
	}
	if uQuota() != wantQuota {
		t.Fatalf("retry must credit, quota=%d want %d", uQuota(), wantQuota)
	}
	if bal, _ := GetSiteWalletBalance(siteC); bal != 100000-70000 {
		t.Fatalf("retry must debit wallet, bal=%d want %d", bal, 100000-70000)
	}
	// 4. Retrying a settled order fails (not manual_review anymore).
	if _, _, err := RetryManualReviewTopUp("P4RETRY", 70000, 1); err == nil {
		t.Fatal("retrying a settled order must fail")
	}
}

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

// TestCompleteEpayTopUpConcurrentIdempotent proves the settlement claim credits a paid order
// EXACTLY ONCE under CONCURRENT duplicate callbacks. This invariant matters more now that the
// epay notify route is no longer behind the global rate limit (concurrent duplicate callbacks
// from EasyPay's retry + shared exit IP are more likely): N goroutines settling the SAME paid
// main-site order must credit the user exactly once, never N times, and leave the order success.
func TestCompleteEpayTopUpConcurrentIdempotent(t *testing.T) {
	if err := DB.AutoMigrate(&TopUp{}, &User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const tradeNo = "P4CONC"
	cleanup := func() {
		DB.Unscoped().Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Unscoped().Where("username = ? AND site_id = ?", "p4concu", 0).Delete(&User{})
	}
	cleanup()
	defer cleanup()

	const amount = int64(10)
	wantQuota := int(amount * int64(common.QuotaPerUnit))
	pw, _ := common.Password2Hash("x")
	u := &User{Username: "p4concu", SiteId: 0, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "p4concaff"}
	require.NoError(t, DB.Create(u).Error)

	require.NoError(t, DB.Create(&TopUp{SiteId: 0, UserId: u.Id, Amount: amount, Money: 100, TradeNo: tradeNo,
		PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}).Error)

	const goroutines = 8
	added := make([]int, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, q, err := CompleteEpayTopUp(tradeNo, 0, 1)
			added[idx], errs[idx] = q, err
		}(i)
	}
	wg.Wait()

	creditCount, totalCredited := 0, 0
	for i := 0; i < goroutines; i++ {
		require.NoErrorf(t, errs[i], "goroutine %d must not error (idempotent no-op expected)", i)
		if added[i] > 0 {
			creditCount++
			totalCredited += added[i]
		}
	}
	assert.Equal(t, 1, creditCount, "exactly one concurrent callback must credit the order")
	assert.Equal(t, wantQuota, totalCredited, "total credited must equal the order quota (credited once)")

	var got User
	require.NoError(t, DB.Select("quota").First(&got, u.Id).Error)
	assert.Equal(t, wantQuota, got.Quota, "user must be credited exactly once under concurrency")

	var order TopUp
	require.NoError(t, DB.Where("trade_no = ?", tradeNo).First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status, "order must end success")
}
