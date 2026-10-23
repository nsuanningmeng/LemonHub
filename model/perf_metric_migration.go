package model

import (
	"fmt"

	"gorm.io/gorm"
)

// Run before AutoMigrate: SQLite can otherwise rebuild the table while changing
// index tags and discard the old constraint before its rows are backfilled.
// Add a nullable hash and its unique index without rewriting the table, then
// remove the old constraint only after every legacy row has an exact identity.
func migratePerfMetricIdentities(db *gorm.DB) error {
	if !db.Migrator().HasTable(&PerfMetric{}) || !db.Migrator().HasIndex(&PerfMetric{}, "idx_channel_perf_bucket") {
		// A legacy MySQL node can still append NULL-hash rows after the upgrade.
		// Keep those independent historical rows; re-keying them could collide
		// with new writers' aggregates. All new writes run BeforeCreate.
		return nil
	}
	if !db.Migrator().HasColumn(&PerfMetric{}, "IdentityHash") {
		if err := db.Migrator().AddColumn(&PerfMetric{}, "IdentityHash"); err != nil {
			return fmt.Errorf("add performance metric identity column: %w", err)
		}
	}
	if !db.Migrator().HasIndex(&PerfMetric{}, "idx_channel_perf_identity_bucket") {
		if err := db.Migrator().CreateIndex(&PerfMetric{}, "idx_channel_perf_identity_bucket"); err != nil {
			return fmt.Errorf("create performance metric identity index: %w", err)
		}
	}
	lastID := 0
	for {
		var metrics []PerfMetric
		err := db.Select("id, model_name, "+commonGroupCol).
			Where("id > ? AND identity_hash IS NULL", lastID).
			Order("id ASC").Limit(200).Find(&metrics).Error
		if err != nil {
			return fmt.Errorf("read legacy performance metric identities: %w", err)
		}
		if len(metrics) == 0 {
			break
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, metric := range metrics {
				identityHash, err := perfMetricIdentityHash(metric.ModelName, metric.Group)
				if err != nil {
					return err
				}
				if err := tx.Model(&PerfMetric{}).Where("id = ? AND identity_hash IS NULL", metric.Id).
					Update("identity_hash", identityHash).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("backfill performance metric identities: %w", err)
		}
		lastID = metrics[len(metrics)-1].Id
	}
	if err := db.Migrator().DropIndex(&PerfMetric{}, "idx_channel_perf_bucket"); err != nil {
		return fmt.Errorf("remove collation-dependent performance metric index: %w", err)
	}
	return nil
}
