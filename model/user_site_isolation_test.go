package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// TestUserSiteIsolation verifies cross-site user-identity isolation at the model layer:
// same username/email/oauth-id on different sites are independent accounts, and every
// identity lookup is scoped so site A's data is invisible to a site B query. Uses the
// package-global DB set up by TestMain (User is migrated with the (site_id, username)
// composite unique index). Dedicated high site ids keep it independent of other tests.
func TestUserSiteIsolation(t *testing.T) {
	const siteA, siteB = 8801, 8802
	DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&User{})
	defer DB.Where("site_id IN ?", []int{siteA, siteB}).Delete(&User{})

	pwHash, err := common.Password2Hash("Secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	affSeq := 0
	mk := func(username, email, github string, site int) *User {
		affSeq++
		u := &User{
			Username: username, Email: email, GitHubId: github, SiteId: site,
			Password: pwHash, Status: common.UserStatusEnabled, Role: common.RoleCommonUser,
			// aff_code carries a global unique index; give each test user a distinct one.
			AffCode: "isoaff" + strconv.Itoa(affSeq),
		}
		if err := DB.Create(u).Error; err != nil {
			t.Fatalf("create %s@%d: %v", username, site, err)
		}
		return u
	}

	// Same username "iso_alice" registered independently on both sites (only possible
	// because the global-unique username was relaxed to per-site composite unique).
	mk("iso_alice", "alice@a.com", "gh_A", siteA)
	mk("iso_alice", "alice@b.com", "gh_B", siteB)
	mk("iso_bob", "bob@a.com", "", siteA)

	// 1. CheckUserExistOrDeleted is site-scoped (no cross-site existence leak).
	if exist, _ := CheckUserExistOrDeleted("iso_bob", "", siteA); !exist {
		t.Fatal("iso_bob should exist on siteA")
	}
	if exist, _ := CheckUserExistOrDeleted("iso_bob", "", siteB); exist {
		t.Fatal("CROSS-SITE LEAK: iso_bob must not be visible on siteB")
	}

	// 2. GetAllUsers is site-scoped.
	usersA, totalA, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 100}, siteA)
	if err != nil {
		t.Fatalf("GetAllUsers(siteA): %v", err)
	}
	if totalA != 2 {
		t.Fatalf("GetAllUsers(siteA) total = %d, want 2", totalA)
	}
	for _, u := range usersA {
		if u.SiteId != siteA {
			t.Fatalf("GetAllUsers(siteA) leaked a site %d user", u.SiteId)
		}
	}
	// SiteScopeAll sees both sites' users.
	_, totalAll, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 100}, SiteScopeAll)
	if err != nil {
		t.Fatalf("GetAllUsers(all): %v", err)
	}
	if totalAll < 3 {
		t.Fatalf("GetAllUsers(SiteScopeAll) total = %d, want >= 3", totalAll)
	}

	// 3. SearchUsers is site-scoped: "iso_alice" exists on both sites but a siteA search
	// returns only siteA's account.
	resA, cntA, err := SearchUsers("iso_alice", "", nil, nil, 0, 100, siteA)
	if err != nil {
		t.Fatalf("SearchUsers(siteA): %v", err)
	}
	if cntA != 1 {
		t.Fatalf("SearchUsers(iso_alice, siteA) = %d, want 1", cntA)
	}
	if len(resA) == 1 && resA[0].SiteId != siteA {
		t.Fatalf("SearchUsers leaked a foreign-site user")
	}

	// 4. ValidateAndFill (password login) is site-scoped.
	loginA := &User{Username: "iso_alice", Password: "Secret123", SiteId: siteA}
	if err := loginA.ValidateAndFill(); err != nil {
		t.Fatalf("login iso_alice on siteA should succeed: %v", err)
	}
	if loginA.Email != "alice@a.com" {
		t.Fatalf("login resolved the wrong site's account: %s", loginA.Email)
	}
	// A user that only exists on siteA cannot log in on a site where they have no account.
	loginBob := &User{Username: "iso_bob", Password: "Secret123", SiteId: siteB}
	if err := loginBob.ValidateAndFill(); err == nil {
		t.Fatal("CROSS-SITE LEAK: iso_bob logged in on siteB")
	}

	// 5. FillUserByGitHubId is site-scoped (oauth identity does not cross sites).
	var ghMiss User
	ghMiss.GitHubId = "gh_A"
	_ = ghMiss.FillUserByGitHubId(siteB)
	if ghMiss.Id != 0 {
		t.Fatal("CROSS-SITE LEAK: gh_A resolved on siteB")
	}
	ghHit := User{GitHubId: "gh_A"}
	_ = ghHit.FillUserByGitHubId(siteA)
	if ghHit.Id == 0 || ghHit.SiteId != siteA {
		t.Fatal("FillUserByGitHubId(siteA) should resolve gh_A")
	}
}
