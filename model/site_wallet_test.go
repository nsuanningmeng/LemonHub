package model

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

// TestSiteWalletConcurrentDeductNoNegative fires many concurrent wallet debits at a
// balance that can only satisfy some of them, and asserts the core invariants from the
// spec: the balance never goes negative, exactly floor(initial/amount) debits succeed,
// and the ledger stays consistent with the balance.
//
// NOTE on coverage: the shared test harness pins the in-memory SQLite to a single
// connection (MaxOpenConns(1), required because :memory: gives each connection a private
// DB), so these transactions execute serialized rather than truly interleaved. The
// guarantee against negative balances under real cross-connection contention rests on
// DeductSiteWallet's atomic conditional UPDATE (`wallet_balance >= amount`, RowsAffected
// check) — the correct pattern on all three databases — which this test exercises
// functionally; true-contention behavior is a property of that SQL, validated on MySQL/PG.
func TestSiteWalletConcurrentDeductNoNegative(t *testing.T) {
	const siteId = 7701
	const initial = int64(1000)
	const amount = int64(100)
	const goroutines = 40

	if err := DB.AutoMigrate(&Site{}, &SiteWalletLog{}); err != nil {
		t.Fatalf("migrate wallet tables: %v", err)
	}
	DB.Where("id = ?", siteId).Delete(&Site{})
	DB.Where("site_id = ?", siteId).Delete(&SiteWalletLog{})
	defer func() {
		DB.Where("id = ?", siteId).Delete(&Site{})
		DB.Where("site_id = ?", siteId).Delete(&SiteWalletLog{})
	}()

	site := &Site{Id: siteId, Name: "wallet-test", Status: SiteStatusNormal, WalletBalance: initial, DiscountRate: DiscountRateBase}
	if err := DB.Create(site).Error; err != nil {
		t.Fatalf("seed site: %v", err)
	}

	var wg sync.WaitGroup
	var successes int64
	var insufficient int64
	var unexpected int64
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := DB.Transaction(func(tx *gorm.DB) error {
				return DeductSiteWallet(tx, siteId, amount, WalletLogTypeRedemptionGen, "concurrency-test", "", 1)
			})
			switch {
			case err == nil:
				atomic.AddInt64(&successes, 1)
			case errors.Is(err, ErrInsufficientWalletBalance):
				atomic.AddInt64(&insufficient, 1)
			default:
				atomic.AddInt64(&unexpected, 1)
			}
		}()
	}
	wg.Wait()

	if unexpected != 0 {
		t.Fatalf("got %d unexpected (non-insufficient) errors", unexpected)
	}

	finalBalance, err := GetSiteWalletBalance(siteId)
	if err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if finalBalance < 0 {
		t.Fatalf("NEGATIVE BALANCE: %d", finalBalance)
	}
	if want := initial / amount; successes != want {
		t.Fatalf("successes = %d, want %d", successes, want)
	}
	if want := initial - successes*amount; finalBalance != want {
		t.Fatalf("finalBalance = %d, want %d", finalBalance, want)
	}
	if successes+insufficient != goroutines {
		t.Fatalf("accounted %d, want %d", successes+insufficient, goroutines)
	}

	// Ledger consistency: every debit wrote exactly one flow record; the seeded initial
	// balance was set directly (not via the ledger), so initial + Σ(flow) == balance.
	sum, err := SumSiteWalletLogAmount(siteId)
	if err != nil {
		t.Fatalf("sum ledger: %v", err)
	}
	if initial+sum != finalBalance {
		t.Fatalf("ledger inconsistent: initial(%d) + Σflow(%d) = %d != balance(%d)", initial, sum, initial+sum, finalBalance)
	}
	var logCount int64
	DB.Model(&SiteWalletLog{}).Where("site_id = ?", siteId).Count(&logCount)
	if logCount != successes {
		t.Fatalf("flow record count %d != successful debits %d", logCount, successes)
	}
}
