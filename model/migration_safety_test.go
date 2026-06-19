package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openMigSafetyDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:migsafety_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// TestBackfillNullSiteIds proves the defensive NULL site_id backfill: rows left
// with a NULL site_id by a non-standard / interrupted historical schema are
// repaired to 0 (main site) so `WHERE site_id = 0` queries keep seeing them.
// It also asserts idempotency and that absent tables are skipped safely.
func TestBackfillNullSiteIds(t *testing.T) {
	db := openMigSafetyDB(t)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("automigrate users: %v", err)
	}

	// Simulate the non-standard schema: existing rows whose site_id is NULL.
	if err := db.Exec(
		"INSERT INTO users (id, username, password, aff_code, site_id) VALUES (1,'u1','p','AFF1',NULL),(2,'u2','p','AFF2',NULL)",
	).Error; err != nil {
		t.Fatalf("seed NULL site_id rows: %v", err)
	}

	var nullCount int64
	db.Model(&User{}).Where("site_id IS NULL").Count(&nullCount)
	if nullCount != 2 {
		t.Fatalf("setup expected 2 NULL site_id rows, got %d", nullCount)
	}

	// backfillNullSiteIds iterates all site-scoped models; only `users` exists
	// here, the rest must be skipped without error.
	if err := backfillNullSiteIds(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	db.Model(&User{}).Where("site_id IS NULL").Count(&nullCount)
	if nullCount != 0 {
		t.Fatalf("expected 0 NULL site_id rows after backfill, got %d", nullCount)
	}
	var zeroCount int64
	db.Model(&User{}).Where("site_id = ?", 0).Count(&zeroCount)
	if zeroCount != 2 {
		t.Fatalf("expected 2 rows with site_id=0 after backfill, got %d", zeroCount)
	}

	// Idempotent: a second pass is a no-op and does not error.
	if err := backfillNullSiteIds(db); err != nil {
		t.Fatalf("idempotent backfill: %v", err)
	}
	db.Model(&User{}).Where("site_id = ?", 0).Count(&zeroCount)
	if zeroCount != 2 {
		t.Fatalf("expected 2 rows with site_id=0 after second backfill, got %d", zeroCount)
	}
}

// TestCountOutOfRangePriceAmounts proves the fail-closed pre-flight gate used by
// migrateSubscriptionPlanPriceAmount: it counts exactly the subscription_plans
// rows whose price_amount cannot be stored losslessly in decimal(10,6) (|value|
// >= 10000), which would otherwise be silently truncated by MySQL (non-STRICT
// mode) or rejected by PostgreSQL.
func TestCountOutOfRangePriceAmounts(t *testing.T) {
	db := openMigSafetyDB(t)
	if err := db.Exec(
		"CREATE TABLE subscription_plans (id integer primary key, price_amount real)",
	).Error; err != nil {
		t.Fatalf("create subscription_plans: %v", err)
	}
	// 9.99 and 9999.0 are in-range; 25000.0 and -10001.5 are out of decimal(10,6).
	if err := db.Exec(
		"INSERT INTO subscription_plans (id, price_amount) VALUES (1, 9.99),(2, 9999.0),(3, 25000.0),(4, -10001.5)",
	).Error; err != nil {
		t.Fatalf("seed prices: %v", err)
	}

	n, err := countOutOfRangePriceAmounts(db)
	if err != nil {
		t.Fatalf("count out of range: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 out-of-range price rows (25000, -10001.5), got %d", n)
	}
}

// TestCountOutOfRangePriceAmountsNoTable proves the pre-flight is a safe no-op on
// a fresh install where subscription_plans does not exist yet.
func TestCountOutOfRangePriceAmountsNoTable(t *testing.T) {
	db := openMigSafetyDB(t)
	n, err := countOutOfRangePriceAmounts(db)
	if err != nil {
		t.Fatalf("count on missing table should not error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 on missing table, got %d", n)
	}
}

// TestCountPrecisionLossPriceAmounts proves the second fail-closed pre-flight gate
// used by migrateSubscriptionPlanPriceAmount: it counts subscription_plans rows
// whose price_amount needs more than 6 fractional digits and would be silently
// rounded (not range-rejected) when MySQL/PostgreSQL narrow the column to
// decimal(10,6). These values pass the magnitude check, so a dedicated precision
// gate is required to avoid quietly mutating stored monetary data.
func TestCountPrecisionLossPriceAmounts(t *testing.T) {
	db := openMigSafetyDB(t)
	require.NoError(t, db.Exec(
		"CREATE TABLE subscription_plans (id integer primary key, price_amount real)",
	).Error)
	// 9.99 (2 dp) and 1.123456 (exactly 6 dp) fit decimal(10,6) losslessly;
	// 1.1234567 (7 dp) and 0.0000001 (7 dp) would be rounded.
	require.NoError(t, db.Exec(
		"INSERT INTO subscription_plans (id, price_amount) VALUES (1, 9.99),(2, 1.123456),(3, 1.1234567),(4, 0.0000001)",
	).Error)

	n, err := countPrecisionLossPriceAmounts(db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "only the two >6-fractional-digit rows must be flagged")
}

// TestCountPrecisionLossPriceAmountsNoTable proves the precision pre-flight is a
// safe no-op when subscription_plans does not exist yet.
func TestCountPrecisionLossPriceAmountsNoTable(t *testing.T) {
	db := openMigSafetyDB(t)
	n, err := countPrecisionLossPriceAmounts(db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}
