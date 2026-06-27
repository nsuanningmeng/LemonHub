package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminAffSeed wires a deterministic referral fixture for the admin global leaderboard:
// three inviters (alice/bob/carol) sharing the "adm_lb_" username prefix so the leaderboard
// keyword filter isolates them from any other rows in the shared in-memory DB, four invitees,
// and a commission ledger spanning the current and previous month. carol has invitees but no
// earnings, which proves the inviter set is "has at least one invitee" rather than "has earned".
//
// It returns (monthStart, alice last_at, bob last_at) so callers can assert the month-window
// split and the per-inviter last-activity enrichment exactly.
func adminAffSeed(t *testing.T) (monthStart, aliceLastAt, bobLastAt int64) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}, &User{}))

	userIds := []int{9301, 9302, 9303, 9311, 9312, 9313, 9314}
	cleanup := func() {
		_ = DB.Unscoped().Where("id IN ?", userIds).Delete(&User{}).Error
		_ = DB.Where("trade_no LIKE ?", "admlb-%").Delete(&AffiliateCommission{}).Error
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now()
	monthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	// Anchor the seed timestamps to a fixed offset INTO the current month rather than relative
	// to "now", so the month-window (created_at >= monthStart) assertions never depend on how
	// close "now" is to midnight on the 1st (which would otherwise flake in a ~30s window).
	lastMonthTs := monthStart - 1
	aliceFirstAt := monthStart + 100
	aliceLastAt = monthStart + 200
	bobLastAt = monthStart + 50

	// Inviters.
	require.NoError(t, DB.Create(&User{Id: 9301, Username: "adm_lb_alice", AffCode: "admlb01",
		Status: common.UserStatusEnabled, AffHistoryQuota: 10000, AffQuota: 3000, AffCount: 2}).Error)
	require.NoError(t, DB.Create(&User{Id: 9302, Username: "adm_lb_bob", AffCode: "admlb02",
		Status: common.UserStatusEnabled, AffHistoryQuota: 5000, AffQuota: 5000, AffCount: 1}).Error)
	require.NoError(t, DB.Create(&User{Id: 9303, Username: "adm_lb_carol", AffCode: "admlb03",
		Status: common.UserStatusEnabled, AffHistoryQuota: 0, AffQuota: 0, AffCount: 0}).Error)
	// Invitees (link to inviters via inviter_id).
	require.NoError(t, DB.Create(&User{Id: 9311, Username: "adm_lb_i1", AffCode: "admlb04", InviterId: 9301}).Error)
	require.NoError(t, DB.Create(&User{Id: 9312, Username: "adm_lb_i2", AffCode: "admlb05", InviterId: 9301}).Error)
	require.NoError(t, DB.Create(&User{Id: 9313, Username: "adm_lb_i3", AffCode: "admlb06", InviterId: 9302}).Error)
	require.NoError(t, DB.Create(&User{Id: 9314, Username: "adm_lb_i4", AffCode: "admlb07", InviterId: 9303}).Error)

	rows := []AffiliateCommission{
		// alice: two this-month rows (6000 + 4000) + one previous-month row (9999, excluded from month).
		{InviterId: 9301, InviteeId: 9311, TradeNo: "admlb-a1", Kind: AffiliateKindRechargeCommission, CommissionQuota: 6000, CreatedAt: aliceFirstAt},
		{InviterId: 9301, InviteeId: 9312, TradeNo: "admlb-a2", Kind: AffiliateKindRechargeCommission, CommissionQuota: 4000, CreatedAt: aliceLastAt},
		{InviterId: 9301, InviteeId: 9311, TradeNo: "admlb-a0", Kind: AffiliateKindRechargeCommission, CommissionQuota: 9999, CreatedAt: lastMonthTs},
		// bob: one this-month row.
		{InviterId: 9302, InviteeId: 9313, TradeNo: "admlb-b1", Kind: AffiliateKindRechargeCommission, CommissionQuota: 5000, CreatedAt: bobLastAt},
		// carol: no ledger rows -> month=0, last_at=0.
	}
	require.NoError(t, DB.Create(&rows).Error)
	return monthStart, aliceLastAt, bobLastAt
}

// TestGetAffAdminSummary asserts the site-wide overview deltas exactly. Deltas (measured
// before vs after seeding) make the assertion robust to any rows other tests leave in the
// shared in-memory DB, while still proving each aggregate is computed correctly.
func TestGetAffAdminSummary(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}, &User{}))

	before, err := GetAffAdminSummary()
	require.NoError(t, err)

	adminAffSeed(t)

	after, err := GetAffAdminSummary()
	require.NoError(t, err)

	assert.Equal(t, int64(15000), after.TotalCommissionPaid-before.TotalCommissionPaid, "Σaff_history: 10000+5000+0")
	assert.Equal(t, int64(8000), after.TotalPendingQuota-before.TotalPendingQuota, "Σaff_quota: 3000+5000+0")
	assert.Equal(t, int64(3), after.TotalActivated-before.TotalActivated, "Σaff_count: 2+1+0")
	assert.Equal(t, int64(3), after.InviterCount-before.InviterCount, "three new distinct inviters")
	assert.Equal(t, int64(15000), after.MonthCommissionQuota-before.MonthCommissionQuota, "this-month commission only: 6000+4000+5000 (9999 is last month)")
}

// TestGetAffAdminLeaderboard covers the default ranking, real (un-masked) usernames, derived
// column enrichment (total_invited / month / last_at), the zero-earning-but-has-invitee case,
// pagination, keyword filtering, and the whitelisted sort columns.
func TestGetAffAdminLeaderboard(t *testing.T) {
	monthStart, aliceLastAt, bobLastAt := adminAffSeed(t)
	_ = monthStart

	// Default: keyword-isolated to our three inviters, sorted by total earned descending.
	res, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "adm_lb_"})
	require.NoError(t, err)
	require.Equal(t, int64(3), res.Total, "alice + bob + carol")
	require.Len(t, res.Items, 3)
	assert.Equal(t, 1, res.Page)
	assert.Equal(t, 20, res.PageSize, "default page size")

	alice, bob, carol := res.Items[0], res.Items[1], res.Items[2]

	// Ranking order + real usernames (NOT masked).
	assert.Equal(t, "adm_lb_alice", alice.Username)
	assert.Equal(t, "adm_lb_bob", bob.Username)
	assert.Equal(t, "adm_lb_carol", carol.Username)

	// alice row, fully enriched.
	assert.Equal(t, 9301, alice.InviterId)
	assert.Equal(t, int64(10000), alice.TotalEarnedQuota)
	assert.Equal(t, int64(3000), alice.PendingQuota)
	assert.Equal(t, 2, alice.ActivatedCount)
	assert.Equal(t, int64(2), alice.TotalInvited, "9311 + 9312")
	assert.Equal(t, int64(10000), alice.MonthCommissionQuota, "6000 + 4000 (excludes last-month 9999)")
	assert.Equal(t, aliceLastAt, alice.LastAt, "max created_at of this-month rows")

	// bob row.
	assert.Equal(t, int64(5000), bob.TotalEarnedQuota)
	assert.Equal(t, int64(1), bob.TotalInvited)
	assert.Equal(t, int64(5000), bob.MonthCommissionQuota)
	assert.Equal(t, bobLastAt, bob.LastAt)

	// carol: zero earnings but has an invitee -> still listed, with zeroed enrichment.
	assert.Equal(t, int64(0), carol.TotalEarnedQuota)
	assert.Equal(t, int64(1), carol.TotalInvited, "9314")
	assert.Equal(t, int64(0), carol.MonthCommissionQuota)
	assert.Equal(t, int64(0), carol.LastAt)

	// Pagination: 2 per page over 3 inviters.
	p1, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "adm_lb_", Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, p1.Items, 2)
	assert.Equal(t, int64(3), p1.Total)
	assert.Equal(t, "adm_lb_alice", p1.Items[0].Username)
	assert.Equal(t, "adm_lb_bob", p1.Items[1].Username)

	p2, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "adm_lb_", Page: 2, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, p2.Items, 1)
	assert.Equal(t, "adm_lb_carol", p2.Items[0].Username)

	// Narrower keyword.
	one, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "adm_lb_alice"})
	require.NoError(t, err)
	require.Equal(t, int64(1), one.Total)
	require.Len(t, one.Items, 1)
	assert.Equal(t, 9301, one.Items[0].InviterId)

	// Sort by pending ascending.
	byPending, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "adm_lb_", Sort: "pending", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, byPending.Items, 3)
	assert.Equal(t, "adm_lb_carol", byPending.Items[0].Username, "pending 0")
	assert.Equal(t, "adm_lb_alice", byPending.Items[1].Username, "pending 3000")
	assert.Equal(t, "adm_lb_bob", byPending.Items[2].Username, "pending 5000")

	// Sort by activated descending.
	byActivated, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "adm_lb_", Sort: "activated", Order: "desc"})
	require.NoError(t, err)
	require.Len(t, byActivated.Items, 3)
	assert.Equal(t, "adm_lb_alice", byActivated.Items[0].Username, "activated 2")
	assert.Equal(t, "adm_lb_bob", byActivated.Items[1].Username, "activated 1")
	assert.Equal(t, "adm_lb_carol", byActivated.Items[2].Username, "activated 0")
}
