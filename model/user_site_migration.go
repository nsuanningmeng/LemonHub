package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// migrateRelaxLegacyUsernameUnique removes the legacy GLOBAL unique constraint on
// users.username so it can be replaced by the per-site composite unique index
// (site_id, username) that AutoMigrate creates from the updated struct tags.
//
// It MUST run BEFORE AutoMigrate(&User{}) so that:
//   - on SQLite the table is rebuilt without the inline column UNIQUE, and the
//     subsequent AutoMigrate re-creates all struct indexes (incl. the composite one);
//   - on MySQL/PostgreSQL the legacy single-column unique is dropped first and the
//     composite unique is then created by AutoMigrate.
//
// It is idempotent and data-preserving across SQLite/MySQL/PostgreSQL. On a fresh
// install (no users table yet) it is a no-op — AutoMigrate creates the correct
// schema directly.
func migrateRelaxLegacyUsernameUnique() error {
	return relaxLegacyUsernameUnique(DB)
}

func relaxLegacyUsernameUnique(db *gorm.DB) error {
	if !db.Migrator().HasTable("users") {
		return nil
	}
	switch {
	case common.UsingMainDatabase(common.DatabaseTypeSQLite):
		return relaxUsernameUniqueSQLite(db)
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		return relaxUsernameUniquePostgres(db)
	default:
		return relaxUsernameUniqueMySQL(db)
	}
}

// sqliteUsernameInlineUniqueRe matches an inline `username <type> ... UNIQUE` column
// definition in a SQLite CREATE TABLE statement (the legacy `unique` tag form).
var sqliteUsernameInlineUniqueRe = regexp.MustCompile("(?i)[`\"'\\[ ]username[`\"'\\] ][^,]*\\bunique\\b")

// relaxUsernameUniqueSQLite rebuilds the users table without the inline UNIQUE on
// username. AlterColumn uses the driver's tested table-recreation (copy-rename),
// preserving all rows; dropped secondary indexes are re-created by the following
// AutoMigrate. Guarded so it only runs while the legacy inline UNIQUE is still present.
func relaxUsernameUniqueSQLite(db *gorm.DB) error {
	var ddl string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='users'").Scan(&ddl).Error; err != nil {
		return err
	}
	if ddl == "" || !sqliteUsernameInlineUniqueRe.MatchString(ddl) {
		return nil // fresh install or already migrated
	}
	common.SysLog("migrating users.username from global-unique to per-site composite unique (SQLite table rebuild)")
	return db.Migrator().AlterColumn(&User{}, "username")
}

// relaxUsernameUniqueMySQL drops any single-column unique index on users.username.
func relaxUsernameUniqueMySQL(db *gorm.DB) error {
	name, ok, err := findSingleColumnUniqueIndex(db, "username")
	if err != nil || !ok {
		return err
	}
	common.SysLog("dropping legacy global-unique index on users.username: " + name)
	return db.Migrator().DropIndex(&User{}, name)
}

// relaxUsernameUniquePostgres drops the legacy global-unique on users.username. On
// PostgreSQL a column UNIQUE is backed by a constraint whose name equals the index
// name, so we drop the constraint (which removes its index); if it is a standalone
// unique index instead, we drop the index.
func relaxUsernameUniquePostgres(db *gorm.DB) error {
	name, ok, err := findSingleColumnUniqueIndex(db, "username")
	if err != nil || !ok {
		return err
	}
	common.SysLog("dropping legacy global-unique on users.username: " + name)
	quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	if err := db.Exec("ALTER TABLE users DROP CONSTRAINT IF EXISTS " + quoted).Error; err != nil {
		return err
	}
	if db.Migrator().HasIndex(&User{}, name) {
		return db.Migrator().DropIndex(&User{}, name)
	}
	return nil
}

// findSingleColumnUniqueIndex returns the name of a unique index covering exactly the
// given single column, using the DB-agnostic GORM Migrator.GetIndexes (implemented by
// the MySQL and PostgreSQL drivers). The composite (site_id, username) index is skipped
// because it covers two columns.
func findSingleColumnUniqueIndex(db *gorm.DB, column string) (string, bool, error) {
	indexes, err := db.Migrator().GetIndexes(&User{})
	if err != nil {
		return "", false, fmt.Errorf("get indexes for users: %w", err)
	}
	for _, idx := range indexes {
		cols := idx.Columns()
		unique, ok := idx.Unique()
		if ok && unique && len(cols) == 1 && cols[0] == column {
			return idx.Name(), true, nil
		}
	}
	return "", false, nil
}
