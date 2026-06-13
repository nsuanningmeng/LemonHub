package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// legacyUsersDDL is the exact SQLite schema produced by the pre-multi-tenant User
// struct (username carried an inline column UNIQUE from the `unique` tag). It is used
// to reproduce a real upgrade scenario and prove the migration preserves data.
const legacyUsersDDL = "CREATE TABLE `users` (`id` integer,`username` text UNIQUE,`password` text NOT NULL,`display_name` text,`role` integer DEFAULT 1,`status` integer DEFAULT 1,`email` text,`github_id` text,`discord_id` text,`oidc_id` text,`wechat_id` text,`telegram_id` text,`access_token` char(32),`quota` integer DEFAULT 0,`used_quota` integer DEFAULT 0,`request_count` integer DEFAULT 0,`group` varchar(64) DEFAULT \"default\",`aff_code` varchar(32),`aff_count` integer DEFAULT 0,`aff_quota` integer DEFAULT 0,`aff_history` integer DEFAULT 0,`inviter_id` integer,`deleted_at` datetime,`linux_do_id` text,`setting` text,`remark` varchar(255),`stripe_customer` varchar(64),`created_at` integer,`last_login_at` integer DEFAULT 0,PRIMARY KEY (`id`))"

func openLegacyUsersDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:usermig_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(legacyUsersDDL).Error; err != nil {
		t.Fatalf("create legacy users: %v", err)
	}
	// The legacy schema also has standalone unique indexes (from uniqueIndex tags).
	if err := db.Exec("CREATE UNIQUE INDEX `idx_users_access_token` ON `users`(`access_token`)").Error; err != nil {
		t.Fatalf("create access_token index: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX `idx_users_aff_code` ON `users`(`aff_code`)").Error; err != nil {
		t.Fatalf("create aff_code index: %v", err)
	}
	return db
}

// TestRelaxUsernameUniqueSQLite proves the SQLite username-uniqueness migration:
// data is preserved, site_id backfills to 0, the same username becomes allowed across
// sites but rejected within a site, and the unrelated global-unique access_token index
// survives. It also asserts idempotency.
func TestRelaxUsernameUniqueSQLite(t *testing.T) {
	db := openLegacyUsersDB(t)

	// Seed legacy rows (globally-unique usernames, as the old constraint guaranteed).
	if err := db.Exec("INSERT INTO users (id, username, password, email, aff_code) VALUES (1,'alice','h','a@x.com','AFF1'),(2,'bob','h','b@x.com','AFF2')").Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Run the migration sequence exactly as production does: relax legacy unique, then AutoMigrate.
	if err := relaxUsernameUniqueSQLite(db); err != nil {
		t.Fatalf("relax: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// (a) Data preserved.
	var count int64
	db.Model(&User{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows preserved, got %d", count)
	}
	// (b) site_id backfilled to 0.
	var alice User
	if err := db.First(&alice, "username = ?", "alice").Error; err != nil {
		t.Fatalf("read alice: %v", err)
	}
	if alice.SiteId != 0 {
		t.Fatalf("expected alice.site_id=0, got %d", alice.SiteId)
	}

	mk := func(username string, site int, aff string) *User {
		return &User{Username: username, SiteId: site, Password: "hashedpwd", AffCode: aff}
	}

	// (c) Same username on a DIFFERENT site is now allowed.
	if err := db.Create(mk("alice", 5, "AFF3")).Error; err != nil {
		t.Fatalf("cross-site same username should be allowed: %v", err)
	}
	// (d) Same username on the SAME site is rejected by the composite unique index.
	if err := db.Create(mk("alice", 0, "AFF4")).Error; err == nil {
		t.Fatalf("same-site duplicate username must be rejected")
	}
	// (d2) Same username on the same NEW site is also rejected.
	if err := db.Create(mk("alice", 5, "AFF5")).Error; err == nil {
		t.Fatalf("duplicate username within site 5 must be rejected")
	}

	// (e) The unrelated global-unique access_token index still enforces uniqueness.
	tok := "tok-shared-0000000000000000000000"
	u1 := mk("carol", 1, "AFF6")
	u1.AccessToken = &tok
	if err := db.Create(u1).Error; err != nil {
		t.Fatalf("create carol: %v", err)
	}
	u2 := mk("dave", 2, "AFF7")
	u2.AccessToken = &tok
	if err := db.Create(u2).Error; err == nil {
		t.Fatalf("access_token must remain globally unique across sites")
	}

	// (f) Idempotent: running the SQLite relax again is a no-op and preserves data.
	if err := relaxUsernameUniqueSQLite(db); err != nil {
		t.Fatalf("idempotent relax: %v", err)
	}
	// Rows present now: alice@0, bob@0, alice@5, carol@1 = 4.
	var finalCount int64
	db.Model(&User{}).Count(&finalCount)
	if finalCount != 4 {
		t.Fatalf("expected 4 rows after migration+inserts, got %d", finalCount)
	}
}
