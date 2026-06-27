package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

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

// TestGetAffAdminLeaderboard_CashOwedAndFlag pins the admin cash-settlement report contract. It
// drives settlement through the production path (SettleReferralOnTopUp) — never hand-crafting
// ledger rows — so the per-row cash_settled marker is set exactly as production would, then asserts
// the leaderboard's derived CashCommissionOwed and IsCashSettled per inviter.
//
//   - The cash-settled promoter accrues owed = the SUM of floor(creditedQuota*percent/100) across its
//     recharge top-ups (multiple rows), and its wallet stays 0 (ledger-only).
//   - The normal inviter's commission was wallet-credited, so its owed is 0 even though it earned a
//     real commission — i.e. wallet-credited commission is never reported as cash owed.
//   - W1 regression: first_bonus rows never contribute to owed. The normal inviter carries a non-zero
//     (2000) first_bonus row yet its owed is 0, proving the kind+cash_settled filter excludes
//     first_bonus; the promoter's first_bonus row is asserted to carry cash_settled=false directly.
func TestGetAffAdminLeaderboard_CashOwedAndFlag(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}, &User{}))

	userIds := []int{9401, 9402, 9403, 9404}
	cleanupRows := func() {
		_ = DB.Unscoped().Where("id IN ?", userIds).Delete(&User{}).Error
		_ = DB.Where("inviter_id IN ?", []int{9401, 9403}).Delete(&AffiliateCommission{}).Error
	}
	cleanupRows()

	prevInviter, prevInvitee, prevPercent := common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent
	pay := operation_setting.GetPaymentSetting()
	prevConfirmed, prevVersion := pay.ComplianceConfirmed, pay.ComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent = prevInviter, prevInvitee, prevPercent
		pay.ComplianceConfirmed, pay.ComplianceTermsVersion = prevConfirmed, prevVersion
		cleanupRows()
	})

	common.QuotaForInviter = 2000
	common.QuotaForInvitee = 1000
	common.AffRechargeCommissionPercent = 5
	pay.ComplianceConfirmed = true
	pay.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	// Cash-settled promoter (AffCashSettled=true) + its invitee.
	require.NoError(t, DB.Create(&User{Id: 9401, Username: "admcash_promoter", Status: common.UserStatusEnabled, AffCode: "admcash1", AffCashSettled: true}).Error)
	require.NoError(t, DB.Create(&User{Id: 9402, Username: "admcash_p_invitee", Status: common.UserStatusEnabled, AffCode: "admcash2", InviterId: 9401}).Error)
	// Normal inviter (wallet-credited) + its invitee.
	require.NoError(t, DB.Create(&User{Id: 9403, Username: "admcash_normal", Status: common.UserStatusEnabled, AffCode: "admcash3"}).Error)
	require.NoError(t, DB.Create(&User{Id: 9404, Username: "admcash_n_invitee", Status: common.UserStatusEnabled, AffCode: "admcash4", InviterId: 9403}).Error)

	// Production path settlement so cash_settled is stamped per row.
	// Promoter: two qualifying top-ups (different trade_nos) -> owed = 5000 + 3000 = 8000.
	require.NoError(t, SettleReferralOnTopUp(9402, "admcash-p1", 100000, "stripe")) // floor(100000*5/100)=5000
	require.NoError(t, SettleReferralOnTopUp(9402, "admcash-p2", 60000, "stripe"))  // floor(60000*5/100)=3000
	// Normal inviter: one top-up -> commission 5000 credited to wallet, NOT cash-owed.
	require.NoError(t, SettleReferralOnTopUp(9404, "admcash-n1", 100000, "stripe")) // floor(100000*5/100)=5000

	res, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: "admcash_"})
	require.NoError(t, err)
	require.Equal(t, int64(2), res.Total, "promoter + normal inviter (invitees are not inviters)")

	byId := make(map[int]AffAdminLeaderboardItem, len(res.Items))
	for _, it := range res.Items {
		byId[it.InviterId] = it
	}
	promoter, ok := byId[9401]
	require.True(t, ok, "promoter row present")
	normal, ok := byId[9403]
	require.True(t, ok, "normal inviter row present")

	// Promoter: cash-settled flag + owed is the sum of both recharge commissions, wallet untouched.
	assert.True(t, promoter.IsCashSettled, "promoter flagged cash-settled")
	assert.Equal(t, int64(8000), promoter.CashCommissionOwed, "5000 + 3000 across two recharge rows")
	assert.Equal(t, int64(0), promoter.PendingQuota, "cash-settled wallet stays 0")
	assert.Equal(t, int64(0), promoter.TotalEarnedQuota, "cash-settled wallet stays 0")

	// Normal inviter: not cash-settled; owed 0 despite a real wallet-credited commission.
	assert.False(t, normal.IsCashSettled, "normal inviter not cash-settled")
	assert.Equal(t, int64(0), normal.CashCommissionOwed, "wallet-credited commission is never cash owed")
	assert.Equal(t, int64(7000), normal.TotalEarnedQuota, "fixed 2000 + commission 5000 in wallet")

	// W1 regression (direct): the promoter's first_bonus row is never marked cash_settled, so the
	// kind+flag filter excludes it from owed regardless of its commission amount.
	var promoterFirstBonus AffiliateCommission
	require.NoError(t, DB.Where("invitee_id = ? AND kind = ?", 9402, AffiliateKindFirstBonus).First(&promoterFirstBonus).Error)
	assert.False(t, promoterFirstBonus.CashSettled, "first_bonus rows are never marked cash_settled")

	// W1 regression (witness): the normal inviter has a non-zero (2000) first_bonus row, yet its owed
	// is 0 — confirming first_bonus never leaks into CashCommissionOwed.
	var normalFirstBonus AffiliateCommission
	require.NoError(t, DB.Where("invitee_id = ? AND kind = ?", 9404, AffiliateKindFirstBonus).First(&normalFirstBonus).Error)
	assert.Equal(t, int64(2000), normalFirstBonus.CommissionQuota, "normal first_bonus carries a real amount")
	assert.False(t, normalFirstBonus.CashSettled)
}

// cashPayoutFixture seeds one inviter + invitee with the standard reward config and confirmed
// payment compliance, then restores globals and removes the seeded rows on cleanup. cashSettled
// selects whether the inviter is a cash-settled promoter (commission recorded as cash owed) or a
// normal inviter (commission wallet-credited, nothing owed as cash). affPrefix must be unique per
// test and short enough for the varchar(32) aff_code unique index.
func cashPayoutFixture(t *testing.T, inviterId, inviteeId int, affPrefix string, cashSettled bool) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}, &AffiliateCashPayout{}, &User{}))

	cleanup := func() {
		_ = DB.Unscoped().Where("id IN ?", []int{inviterId, inviteeId}).Delete(&User{}).Error
		_ = DB.Where("inviter_id = ?", inviterId).Delete(&AffiliateCommission{}).Error
		_ = DB.Where("inviter_id = ?", inviterId).Delete(&AffiliateCashPayout{}).Error
	}
	cleanup()

	prevInviter, prevInvitee, prevPercent := common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent
	pay := operation_setting.GetPaymentSetting()
	prevConfirmed, prevVersion := pay.ComplianceConfirmed, pay.ComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent = prevInviter, prevInvitee, prevPercent
		pay.ComplianceConfirmed, pay.ComplianceTermsVersion = prevConfirmed, prevVersion
		cleanup()
	})

	common.QuotaForInviter = 2000
	common.QuotaForInvitee = 1000
	common.AffRechargeCommissionPercent = 5
	pay.ComplianceConfirmed = true
	pay.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, DB.Create(&User{Id: inviterId, Username: affPrefix + "_inv", Status: common.UserStatusEnabled, AffCode: affPrefix + "i", AffCashSettled: cashSettled}).Error)
	require.NoError(t, DB.Create(&User{Id: inviteeId, Username: affPrefix + "_ee", Status: common.UserStatusEnabled, AffCode: affPrefix + "e", InviterId: inviterId}).Error)
}

// affPayoutBalances reads (total, paid, owed) cash figures for one inviter off the admin leaderboard,
// keyword-scoped to that inviter's unique prefix so it never collides with rows other tests leave in
// the shared in-memory DB.
func affPayoutBalances(t *testing.T, inviterId int, keyword string) (total, paid, owed int64) {
	t.Helper()
	res, err := GetAffAdminLeaderboard(AffAdminLeaderboardQuery{Keyword: keyword})
	require.NoError(t, err)
	for _, it := range res.Items {
		if it.InviterId == inviterId {
			return it.CashCommissionTotal, it.CashCommissionPaid, it.CashCommissionOwed
		}
	}
	t.Fatalf("inviter %d not found on leaderboard for keyword %q", inviterId, keyword)
	return 0, 0, 0
}

func payoutCount(t *testing.T, inviterId int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, DB.Model(&AffiliateCashPayout{}).Where("inviter_id = ?", inviterId).Count(&n).Error)
	return n
}

// TestRecordAffiliateCashPayout_PartialThenFullThenOverpay walks the full settlement lifecycle for a
// cash-settled promoter with a known gross cash commission T: a partial payout leaves owed=T-p, the
// remaining payout drives owed to 0, and any further payout is rejected without writing a row or
// moving balances. The total/paid/owed triple is read off GetAffAdminLeaderboard at each step.
func TestRecordAffiliateCashPayout_PartialThenFullThenOverpay(t *testing.T) {
	const inviterId, inviteeId = 9501, 9502
	cashPayoutFixture(t, inviterId, inviteeId, "cpayA", true)

	// Drive gross cash commission T = floor(100000*5%) + floor(80000*5%) = 5000 + 4000 = 9000.
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayA-1", 100000, "stripe"))
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayA-2", 80000, "stripe"))
	const total = int64(9000)

	total0, paid0, owed0 := affPayoutBalances(t, inviterId, "cpayA")
	require.Equal(t, total, total0)
	require.Equal(t, int64(0), paid0)
	require.Equal(t, total, owed0)

	// Partial payout p = 4000.
	const p = int64(4000)
	row, err := RecordAffiliateCashPayout(inviterId, p, "batch-1", 777)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Greater(t, row.Id, 0)
	assert.Equal(t, inviterId, row.InviterId)
	assert.Equal(t, p, row.Amount)
	assert.Equal(t, "batch-1", row.Note)
	assert.Equal(t, 777, row.OperatorId)

	tt, pp, oo := affPayoutBalances(t, inviterId, "cpayA")
	assert.Equal(t, total, tt, "gross total unchanged by payouts")
	assert.Equal(t, p, pp, "paid == partial")
	assert.Equal(t, total-p, oo, "owed == T - p")

	// Settle the remainder -> owed 0, paid == total.
	rem, err := RecordAffiliateCashPayout(inviterId, total-p, "batch-2", 777)
	require.NoError(t, err)
	require.NotNil(t, rem)

	tt, pp, oo = affPayoutBalances(t, inviterId, "cpayA")
	assert.Equal(t, total, tt)
	assert.Equal(t, total, pp, "fully paid")
	assert.Equal(t, int64(0), oo, "owed cleared")
	require.Equal(t, int64(2), payoutCount(t, inviterId))

	// One more unit must be rejected, write no row, and leave balances untouched.
	over, err := RecordAffiliateCashPayout(inviterId, 1, "over", 777)
	require.Error(t, err, "payout beyond outstanding must be rejected")
	assert.Nil(t, over)
	assert.Equal(t, int64(2), payoutCount(t, inviterId), "no row written on rejection")

	tt, pp, oo = affPayoutBalances(t, inviterId, "cpayA")
	assert.Equal(t, total, tt)
	assert.Equal(t, total, pp, "paid unchanged after rejection")
	assert.Equal(t, int64(0), oo, "owed unchanged after rejection")
}

// TestRecordAffiliateCashPayout_OverpayRejectedUpFront verifies a single payout exceeding the
// outstanding owed is rejected up front and writes nothing.
func TestRecordAffiliateCashPayout_OverpayRejectedUpFront(t *testing.T) {
	const inviterId, inviteeId = 9511, 9512
	cashPayoutFixture(t, inviterId, inviteeId, "cpayB", true)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayB-1", 100000, "stripe")) // T = 5000

	row, err := RecordAffiliateCashPayout(inviterId, 5001, "too much", 1)
	require.Error(t, err)
	assert.Nil(t, row)
	assert.Equal(t, int64(0), payoutCount(t, inviterId), "rejected overpay writes no row")

	tt, pp, oo := affPayoutBalances(t, inviterId, "cpayB")
	assert.Equal(t, int64(5000), tt)
	assert.Equal(t, int64(0), pp)
	assert.Equal(t, int64(5000), oo, "owed still full")
}

// TestRecordAffiliateCashPayout_Validation covers the input guards: non-positive inviter id,
// non-positive amount, and a missing inviter. None of these write a payout row.
func TestRecordAffiliateCashPayout_Validation(t *testing.T) {
	const inviterId, inviteeId = 9521, 9522
	cashPayoutFixture(t, inviterId, inviteeId, "cpayC", true)
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayC-1", 100000, "stripe")) // outstanding 5000

	// inviterId <= 0
	row, err := RecordAffiliateCashPayout(0, 100, "", 1)
	require.Error(t, err)
	assert.Nil(t, row)
	row, err = RecordAffiliateCashPayout(-3, 100, "", 1)
	require.Error(t, err)
	assert.Nil(t, row)

	// amount <= 0 (valid inviter, but bad amount)
	row, err = RecordAffiliateCashPayout(inviterId, 0, "", 1)
	require.Error(t, err)
	assert.Nil(t, row)
	row, err = RecordAffiliateCashPayout(inviterId, -50, "", 1)
	require.Error(t, err)
	assert.Nil(t, row)

	// inviter not found
	row, err = RecordAffiliateCashPayout(987654, 100, "", 1)
	require.Error(t, err)
	assert.Nil(t, row)

	assert.Equal(t, int64(0), payoutCount(t, inviterId), "no rejected call wrote a row")
}

// TestGetAffiliateCashPayouts_NewestFirst verifies the read path returns an inviter's recorded cash
// settlements newest-first with the correct amounts and notes. created_at is set to explicit,
// distinct values after recording so the ordering assertion is fully deterministic.
func TestGetAffiliateCashPayouts_NewestFirst(t *testing.T) {
	const inviterId, inviteeId = 9531, 9532
	cashPayoutFixture(t, inviterId, inviteeId, "cpayD", true)
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayD-1", 200000, "stripe")) // outstanding 10000

	// Each payout is within the shrinking outstanding (10000 -> 9000 -> 7000 -> 4000).
	_, err := RecordAffiliateCashPayout(inviterId, 1000, "first", 1)
	require.NoError(t, err)
	_, err = RecordAffiliateCashPayout(inviterId, 2000, "second", 1)
	require.NoError(t, err)
	_, err = RecordAffiliateCashPayout(inviterId, 3000, "third", 1)
	require.NoError(t, err)

	// Pin distinct created_at so newest-first is deterministic regardless of timestamp resolution.
	require.NoError(t, DB.Model(&AffiliateCashPayout{}).Where("inviter_id = ? AND note = ?", inviterId, "first").Update("created_at", 1000).Error)
	require.NoError(t, DB.Model(&AffiliateCashPayout{}).Where("inviter_id = ? AND note = ?", inviterId, "second").Update("created_at", 2000).Error)
	require.NoError(t, DB.Model(&AffiliateCashPayout{}).Where("inviter_id = ? AND note = ?", inviterId, "third").Update("created_at", 3000).Error)

	rows, err := GetAffiliateCashPayouts(inviterId, 50)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "third", rows[0].Note)
	assert.Equal(t, int64(3000), rows[0].Amount)
	assert.Equal(t, "second", rows[1].Note)
	assert.Equal(t, int64(2000), rows[1].Amount)
	assert.Equal(t, "first", rows[2].Note)
	assert.Equal(t, int64(1000), rows[2].Amount)

	// limit <= 0 falls back to the default (50) and still returns all three rows.
	rowsDefault, err := GetAffiliateCashPayouts(inviterId, 0)
	require.NoError(t, err)
	require.Len(t, rowsDefault, 3)
}

// TestRecordAffiliateCashPayout_NormalInviterHasNoOutstanding verifies a normal (non-cash-settled)
// inviter has zero cash outstanding — its commission was wallet-credited, not recorded as cash — so
// any positive payout is rejected.
func TestRecordAffiliateCashPayout_NormalInviterHasNoOutstanding(t *testing.T) {
	const inviterId, inviteeId = 9541, 9542
	cashPayoutFixture(t, inviterId, inviteeId, "cpayE", false) // normal inviter
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayE-1", 100000, "stripe"))

	tt, pp, oo := affPayoutBalances(t, inviterId, "cpayE")
	assert.Equal(t, int64(0), tt, "normal inviter: no cash-settled commission")
	assert.Equal(t, int64(0), pp)
	assert.Equal(t, int64(0), oo, "nothing owed as cash")

	row, err := RecordAffiliateCashPayout(inviterId, 1, "nope", 1)
	require.Error(t, err, "positive payout against 0 outstanding must be rejected")
	assert.Nil(t, row)
	assert.Equal(t, int64(0), payoutCount(t, inviterId))
}

// TestCashOwed_LegacyNullCashSettledExcluded proves the cash-owed math is NULL-safe for MySQL/PG
// databases upgraded from before this feature: pre-existing recharge_commission rows have
// cash_settled = SQL NULL (the column is nullable with no default), and the owed filter
// (kind = recharge_commission AND cash_settled = <true>) must treat NULL as NOT cash-settled
// (NULL = true is unknown -> excluded) so a legacy promoter's already-wallet-credited commission is
// never double-counted as cash owed.
//
// The same recharge_commission row is used for both checks: forced to true it IS counted (positive
// control), then forced to NULL it is excluded — isolating the flag as the only difference.
func TestCashOwed_LegacyNullCashSettledExcluded(t *testing.T) {
	const inviterId, inviteeId = 9551, 9552
	cashPayoutFixture(t, inviterId, inviteeId, "cpayF", false) // normal inviter (row written cash_settled=false)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "cpayF-1", 100000, "stripe")) // commission = 5000

	// Positive control: with the row explicitly cash_settled=true, the owed math counts it.
	require.NoError(t, DB.Exec(
		"UPDATE affiliate_commissions SET cash_settled = "+commonTrueVal+" WHERE inviter_id = ? AND kind = ?",
		inviterId, AffiliateKindRechargeCommission).Error)
	tt, _, oo := affPayoutBalances(t, inviterId, "cpayF")
	require.Equal(t, int64(5000), tt, "explicit cash_settled=true row is counted (positive control)")
	require.Equal(t, int64(5000), oo)

	// Legacy upgrade simulation: force the column to SQL NULL on the same row.
	require.NoError(t, DB.Exec(
		"UPDATE affiliate_commissions SET cash_settled = NULL WHERE inviter_id = ? AND kind = ?",
		inviterId, AffiliateKindRechargeCommission).Error)

	// Confirm the row really is NULL now (not coerced to a falsey value) so the assertion below is
	// genuinely exercising NULL-handling.
	var nullCount int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).
		Where("inviter_id = ? AND cash_settled IS NULL", inviterId).Count(&nullCount).Error)
	require.Equal(t, int64(1), nullCount, "the recharge row is now SQL NULL")

	// NULL-safety: the legacy NULL row is excluded from cash owed.
	tt, _, oo = affPayoutBalances(t, inviterId, "cpayF")
	assert.Equal(t, int64(0), tt, "legacy NULL cash_settled is treated as NOT cash-settled (total 0)")
	assert.Equal(t, int64(0), oo, "legacy NULL cash_settled contributes nothing to cash owed")
}
