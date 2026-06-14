package model

import (
	"fmt"

	"gorm.io/gorm"
)

// siteScopedModels are the tables that carry a white-label site_id column. A NULL
// site_id in any of them would hide legacy rows from the `WHERE site_id = 0`
// (main-site) queries used throughout the codebase, so the upgrade path must
// guarantee every existing row has site_id = 0.
func siteScopedModels() []interface{} {
	return []interface{}{&User{}, &Token{}, &Redemption{}, &Log{}, &TopUp{}}
}

// backfillNullSiteIds defensively sets site_id = 0 for any row whose site_id is
// NULL.
//
// GORM's `default:0` tag makes a normal `ALTER TABLE ... ADD COLUMN site_id ...
// DEFAULT 0` backfill existing rows to 0 on SQLite, MySQL and PostgreSQL, so the
// standard "old schema without the column -> HEAD" upgrade is already safe.
// However a non-standard or interrupted historical schema (e.g. a nullable
// site_id column added by hand or by a half-finished migration) may still hold
// NULLs. This pass closes that gap. It is idempotent, cheap, and cross-DB.
func backfillNullSiteIds(db *gorm.DB) error {
	for _, m := range siteScopedModels() {
		if err := backfillModelNullSiteId(db, m); err != nil {
			return err
		}
	}
	return nil
}

// backfillModelNullSiteId backfills a single model's NULL site_id rows to 0.
func backfillModelNullSiteId(db *gorm.DB, model interface{}) error {
	if !db.Migrator().HasTable(model) || !db.Migrator().HasColumn(model, "site_id") {
		return nil
	}
	// Update(column, value) sets the column even when value is the zero value,
	// avoiding GORM's struct-based zero-value omission.
	if err := db.Model(model).Where("site_id IS NULL").Update("site_id", 0).Error; err != nil {
		return fmt.Errorf("backfill NULL site_id for %T failed: %w", model, err)
	}
	return nil
}

// priceAmountDecimalMax is the largest magnitude that fits losslessly in
// decimal(10,6) (10 total digits, 6 fractional => max integer part 9999).
const priceAmountDecimalMax = 9999.999999

// countOutOfRangePriceAmounts returns how many subscription_plans rows hold a
// price_amount whose magnitude does not fit in decimal(10,6). Such values would
// be rejected by PostgreSQL (numeric field overflow) or, worse, silently
// truncated by MySQL when STRICT mode is off — so the migration uses this as a
// fail-closed preflight before narrowing the column type.
func countOutOfRangePriceAmounts(db *gorm.DB) (int64, error) {
	if !db.Migrator().HasTable(&SubscriptionPlan{}) ||
		!db.Migrator().HasColumn(&SubscriptionPlan{}, "price_amount") {
		return 0, nil
	}
	var n int64
	err := db.Model(&SubscriptionPlan{}).
		Where("price_amount > ? OR price_amount < ?", priceAmountDecimalMax, -priceAmountDecimalMax).
		Count(&n).Error
	return n, err
}
