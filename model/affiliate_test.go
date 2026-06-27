package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// affiliateTestSetup migrates the ledger table, configures the reward amounts/percent and
// the compliance gate, seeds an inviter + invitee, and restores global state on cleanup.
// It returns the inviter and invitee ids.
func affiliateTestSetup(t *testing.T, inviterReward, inviteeReward int, percent float64) (int, int) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}))

	require.NoError(t, DB.Exec("DELETE FROM affiliate_commissions").Error)
	require.NoError(t, DB.Unscoped().Where("id IN ?", []int{8101, 8102, 8103}).Delete(&User{}).Error)

	prevInviter, prevInvitee, prevPercent := common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent
	pay := operation_setting.GetPaymentSetting()
	prevConfirmed, prevVersion := pay.ComplianceConfirmed, pay.ComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent = prevInviter, prevInvitee, prevPercent
		pay.ComplianceConfirmed, pay.ComplianceTermsVersion = prevConfirmed, prevVersion
		_ = DB.Exec("DELETE FROM affiliate_commissions").Error
		_ = DB.Unscoped().Where("id IN ?", []int{8101, 8102, 8103}).Delete(&User{}).Error
	})

	common.QuotaForInviter = inviterReward
	common.QuotaForInvitee = inviteeReward
	common.AffRechargeCommissionPercent = percent
	pay.ComplianceConfirmed = true
	pay.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, DB.Create(&User{Id: 8101, Username: "inviter1", Status: common.UserStatusEnabled, AffCode: "afftst01"}).Error)
	require.NoError(t, DB.Create(&User{Id: 8102, Username: "invitee2", Status: common.UserStatusEnabled, AffCode: "afftst02", InviterId: 8101}).Error)
	return 8101, 8102
}

func reloadUser(t *testing.T, id int) User {
	t.Helper()
	var u User
	require.NoError(t, DB.Where("id = ?", id).First(&u).Error)
	return u
}

func ledgerCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&n).Error)
	return n
}

// ledgerRow fetches the single ledger row matching the where clause (fails if absent or
// non-unique callers should scope tightly). Used to assert the recorded commission_quota.
func ledgerRow(t *testing.T, query string, args ...interface{}) AffiliateCommission {
	t.Helper()
	var row AffiliateCommission
	require.NoError(t, DB.Where(query, args...).First(&row).Error)
	return row
}

// TestSettleReferralOnTopUp_FirstRechargeGrantsBonusAndCommission verifies the first
// qualifying top-up grants BOTH the one-time fixed bonus (inviter + invitee) and the
// percentage commission, and that the invitee's real quota is credited.
func TestSettleReferralOnTopUp_FirstRechargeGrantsBonusAndCommission(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-1", 100000, "stripe"))

	inviter := reloadUser(t, inviterId)
	// fixed 2000 + 5% of 100000 (=5000)
	assert.Equal(t, 7000, inviter.AffQuota)
	assert.Equal(t, 7000, inviter.AffHistoryQuota)
	assert.Equal(t, 1, inviter.AffCount)

	invitee := reloadUser(t, inviteeId)
	assert.Equal(t, 1000, invitee.Quota)

	assert.Equal(t, int64(2), ledgerCount(t))
}

// TestSettleReferralOnTopUp_IdempotentOnRetry verifies a duplicate webhook for the same
// trade_no never double-pays.
func TestSettleReferralOnTopUp_IdempotentOnRetry(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-1", 100000, "stripe"))
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-1", 100000, "stripe"))

	inviter := reloadUser(t, inviterId)
	assert.Equal(t, 7000, inviter.AffQuota)
	assert.Equal(t, 1, inviter.AffCount)
	invitee := reloadUser(t, inviteeId)
	assert.Equal(t, 1000, invitee.Quota)
	assert.Equal(t, int64(2), ledgerCount(t))
}

// TestSettleReferralOnTopUp_SubsequentRechargeCommissionOnly verifies later top-ups grant
// only the percentage commission (no second fixed bonus, aff_count unchanged).
func TestSettleReferralOnTopUp_SubsequentRechargeCommissionOnly(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-1", 100000, "stripe"))
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-2", 200000, "stripe"))

	inviter := reloadUser(t, inviterId)
	// 7000 (first) + 5% of 200000 (=10000)
	assert.Equal(t, 17000, inviter.AffQuota)
	assert.Equal(t, 17000, inviter.AffHistoryQuota)
	assert.Equal(t, 1, inviter.AffCount) // still one activated invitee
	invitee := reloadUser(t, inviteeId)
	assert.Equal(t, 1000, invitee.Quota) // invitee fixed bonus only once
	assert.Equal(t, int64(3), ledgerCount(t))
}

// TestSettleReferralOnTopUp_SkipsWhenComplianceNotConfirmed verifies the compliance gate
// blocks all payouts.
func TestSettleReferralOnTopUp_SkipsWhenComplianceNotConfirmed(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-1", 100000, "stripe"))

	inviter := reloadUser(t, inviterId)
	assert.Equal(t, 0, inviter.AffQuota)
	assert.Equal(t, 0, inviter.AffCount)
	assert.Equal(t, int64(0), ledgerCount(t))
}

// TestSettleReferralOnTopUp_NoInviterOrSelfInvite verifies users without an inviter, and
// self-invites, are no-ops.
func TestSettleReferralOnTopUp_NoInviterOrSelfInvite(t *testing.T) {
	_, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)

	// No inviter.
	require.NoError(t, DB.Create(&User{Id: 8103, Username: "lone3", Status: common.UserStatusEnabled, AffCode: "afftst03"}).Error)
	require.NoError(t, SettleReferralOnTopUp(8103, "afftrade-x", 100000, "stripe"))
	assert.Equal(t, int64(0), ledgerCount(t))

	// Self-invite (inviter_id == self) must not pay.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviteeId).Update("inviter_id", inviteeId).Error)
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-self", 100000, "stripe"))
	assert.Equal(t, int64(0), ledgerCount(t))
}

// TestSettleReferral_FirstBonusUniquePerInvitee verifies the once-per-invitee invariant is
// enforced by the DB unique index (not just the non-atomic COUNT): a second first_bonus row
// for the same invitee — even carrying a different trade_no — must be rejected, and a second
// qualifying recharge must never create a second first_bonus or re-increment aff_count.
func TestSettleReferral_FirstBonusUniquePerInvitee(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "uniq-1", 100000, "stripe"))

	// Direct insert of a second first_bonus for this invitee must hit the unique index,
	// because the row is keyed on the synthetic per-invitee key, not the originating trade_no.
	dup := &AffiliateCommission{
		InviterId:       inviterId,
		InviteeId:       inviteeId,
		TradeNo:         affiliateFirstBonusKey(inviteeId),
		Kind:            AffiliateKindFirstBonus,
		CommissionQuota: 2000,
		CreatedAt:       common.GetTimestamp(),
	}
	require.Error(t, DB.Create(dup).Error, "unique index must block a second first_bonus per invitee")

	// A second qualifying recharge (different trade) grants commission only — no second bonus.
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "uniq-2", 100000, "stripe"))
	var firstBonusCount int64
	require.NoError(t, DB.Model(&AffiliateCommission{}).
		Where("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonus).
		Count(&firstBonusCount).Error)
	assert.Equal(t, int64(1), firstBonusCount, "exactly one first_bonus per invitee")
	assert.Equal(t, 1, reloadUser(t, inviterId).AffCount, "aff_count incremented only once")
}

// TestAffStatsAndLeaderboard verifies the dashboard aggregates after several recharges.
func TestAffStatsAndLeaderboard(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-1", 100000, "stripe"))
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-2", 200000, "stripe"))

	stats, err := GetAffStats(inviterId)
	require.NoError(t, err)
	assert.Equal(t, int64(17000), stats.PendingQuota)
	assert.Equal(t, int64(17000), stats.TotalEarnedQuota)
	assert.Equal(t, 1, stats.ActivatedCount)
	assert.Equal(t, int64(1), stats.TotalInvited)
	assert.Equal(t, int64(17000), stats.MonthCommissionQuota)

	board, err := GetAffLeaderboard(inviterId, 10)
	require.NoError(t, err)
	require.Len(t, board, 1)
	assert.Equal(t, inviteeId, board[0].InviteeId)
	assert.Equal(t, int64(17000), board[0].CommissionQuota) // 2000 + 5000 + 10000
	assert.Equal(t, 2, board[0].RechargeCount)              // two recharge_commission rows
	assert.Equal(t, "in***2", board[0].Username)            // masked
}

// TestSettleReferralOnTopUp_PerUserCommissionOverride verifies an inviter-level commission rate
// override takes precedence over the global default for the recharge commission.
func TestSettleReferralOnTopUp_PerUserCommissionOverride(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 0, 0, 5) // global 5%, no fixed bonus
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviterId).
		Update("aff_commission_percent", 10.0).Error)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-ovr", 100000, "stripe"))

	inviter := reloadUser(t, inviterId)
	// 10% override of 100000 = 10000 (the global 5% would have been 5000).
	assert.Equal(t, 10000, inviter.AffQuota)
	assert.Equal(t, 10000, inviter.AffHistoryQuota)
}

// TestSettleReferralOnTopUp_ZeroOverrideDisablesCommission verifies an explicit 0% override
// disables the recharge commission entirely (distinct from nil, which inherits the global rate),
// while the one-time fixed bonus still applies.
func TestSettleReferralOnTopUp_ZeroOverrideDisablesCommission(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5) // global 5%, fixed 2000/1000
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviterId).
		Update("aff_commission_percent", 0.0).Error)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "afftrade-zero", 100000, "stripe"))

	inviter := reloadUser(t, inviterId)
	// Fixed 2000 bonus only; the 0% override suppresses the recharge commission (no global 5%).
	assert.Equal(t, 2000, inviter.AffQuota)
	assert.Equal(t, 1, inviter.AffCount)
	// Only the first_bonus ledger row exists — no recharge_commission row was written.
	assert.Equal(t, int64(1), ledgerCount(t))
}

// TestUserEditAffCommissionPresence verifies the admin-edit contract for the per-user commission
// override: omitting the field (updateAffCommission=false) preserves an existing override, while an
// explicit clear (updateAffCommission=true with a nil value) resets it to NULL (inherit the global).
func TestUserEditAffCommissionPresence(t *testing.T) {
	require.NoError(t, DB.Unscoped().Where("id = ?", 8201).Delete(&User{}).Error)
	t.Cleanup(func() { _ = DB.Unscoped().Where("id = ?", 8201).Delete(&User{}).Error })

	override := 12.0
	require.NoError(t, DB.Create(&User{
		Id: 8201, Username: "affedit1", Status: common.UserStatusEnabled,
		AffCode: "affedt01", AffCommissionPercent: &override,
	}).Error)

	// Edit WITHOUT the field present (updateAffCommission=false) must preserve the override.
	absent := User{Id: 8201, Username: "affedit1", DisplayName: "n", Group: "default"}
	require.NoError(t, absent.Edit(false, false, false))
	got := reloadUser(t, 8201)
	require.NotNil(t, got.AffCommissionPercent)
	assert.Equal(t, 12.0, *got.AffCommissionPercent)

	// Edit WITH the field present and nil (updateAffCommission=true) clears it back to NULL.
	clear := User{Id: 8201, Username: "affedit1", DisplayName: "n", Group: "default"}
	require.NoError(t, clear.Edit(false, true, false))
	assert.Nil(t, reloadUser(t, 8201).AffCommissionPercent)
}

// TestSettleReferralOnTopUp_CashSettledSuppressesInviterCredit pins the first-top-up contract for
// the two settlement modes side by side: a cash-settled promoter's inviter-side rewards are
// suppressed (aff_quota/aff_history stay at the starting 0), while a normal inviter is credited the
// fixed bonus + commission. In BOTH modes the invitee bonus is paid, aff_count is incremented, and
// both ledger kinds are written — only the inviter wallet credit differs. The cash-settled
// recharge_commission row still records the full cash-basis amount; its first_bonus row records 0.
func TestSettleReferralOnTopUp_CashSettledSuppressesInviterCredit(t *testing.T) {
	const credited = int64(100000)
	const commission = 5000 // floor(100000 * 5 / 100)

	cases := []struct {
		name                 string
		cashSettled          bool
		tradeNo              string
		wantInviterAffQuota  int   // aff_quota and aff_history after settlement
		wantFirstBonusRecord int64 // commission_quota on the first_bonus ledger row
		wantCommissionRecord int64 // commission_quota on the recharge_commission ledger row
	}{
		{
			name:                 "normal inviter is credited bonus+commission",
			cashSettled:          false,
			tradeNo:              "affcash-normal-1",
			wantInviterAffQuota:  2000 + commission, // fixed 2000 + 5% of 100000
			wantFirstBonusRecord: 2000,
			wantCommissionRecord: commission,
		},
		{
			name:                 "cash-settled promoter wallet untouched, ledger still recorded",
			cashSettled:          true,
			tradeNo:              "affcash-cash-1",
			wantInviterAffQuota:  0,          // inviter reward fully suppressed
			wantFirstBonusRecord: 0,          // first_bonus row records 0 for cash-settled inviter
			wantCommissionRecord: commission, // cash basis still recorded in the ledger
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)
			if tc.cashSettled {
				require.NoError(t, DB.Model(&User{}).Where("id = ?", inviterId).
					Update("aff_cash_settled", true).Error)
			}

			require.NoError(t, SettleReferralOnTopUp(inviteeId, tc.tradeNo, credited, "stripe"))

			inviter := reloadUser(t, inviterId)
			assert.Equal(t, tc.wantInviterAffQuota, inviter.AffQuota, "aff_quota")
			assert.Equal(t, tc.wantInviterAffQuota, inviter.AffHistoryQuota, "aff_history")
			// aff_count is incremented for an activated (paying) invitee in BOTH modes.
			assert.Equal(t, 1, inviter.AffCount, "aff_count")

			// The invitee acquisition bonus is independent of how the inviter is settled.
			assert.Equal(t, 1000, reloadUser(t, inviteeId).Quota, "invitee quota")

			// Both ledger kinds are written in both modes.
			assert.Equal(t, int64(2), ledgerCount(t), "ledger rows")
			firstBonus := ledgerRow(t, "invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonus)
			assert.Equal(t, tc.wantFirstBonusRecord, firstBonus.CommissionQuota, "first_bonus commission_quota")
			recharge := ledgerRow(t, "trade_no = ? AND kind = ?", tc.tradeNo, AffiliateKindRechargeCommission)
			assert.Equal(t, tc.wantCommissionRecord, recharge.CommissionQuota, "recharge commission_quota")
			assert.Equal(t, credited, recharge.RechargeQuota, "recharge_quota basis")
		})
	}
}

// TestSettleReferralOnTopUp_CashSettledSubsequentRecharge verifies a cash-settled promoter's SECOND
// qualifying top-up (different trade_no) records another recharge_commission ledger row with the
// correct cash-basis amount, with NO second first_bonus, NO wallet credit, and aff_count unchanged.
func TestSettleReferralOnTopUp_CashSettledSubsequentRecharge(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviterId).
		Update("aff_cash_settled", true).Error)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "affcash-2a", 100000, "stripe"))
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "affcash-2b", 200000, "stripe"))

	inviter := reloadUser(t, inviterId)
	// Wallet stays at the starting 0 across both top-ups (cash-settled: ledger-only).
	assert.Equal(t, 0, inviter.AffQuota, "aff_quota stays 0")
	assert.Equal(t, 0, inviter.AffHistoryQuota, "aff_history stays 0")
	assert.Equal(t, 1, inviter.AffCount, "still one activated invitee, no second first_bonus")

	// Invitee bonus paid exactly once.
	assert.Equal(t, 1000, reloadUser(t, inviteeId).Quota, "invitee fixed bonus only once")

	// first_bonus(1) + recharge_commission(2) = 3 ledger rows.
	assert.Equal(t, int64(3), ledgerCount(t), "three ledger rows")

	// The second recharge records floor(200000 * 5 / 100) = 10000 as the cash basis.
	second := ledgerRow(t, "trade_no = ? AND kind = ?", "affcash-2b", AffiliateKindRechargeCommission)
	assert.Equal(t, int64(10000), second.CommissionQuota, "second recharge commission_quota")
	assert.Equal(t, int64(200000), second.RechargeQuota, "second recharge_quota basis")
}

// TestSettleReferralOnTopUp_CashSettledIdempotent verifies replaying the same trade_no for a
// cash-settled promoter neither duplicates ledger rows nor changes any balance.
func TestSettleReferralOnTopUp_CashSettledIdempotent(t *testing.T) {
	inviterId, inviteeId := affiliateTestSetup(t, 2000, 1000, 5)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviterId).
		Update("aff_cash_settled", true).Error)

	require.NoError(t, SettleReferralOnTopUp(inviteeId, "affcash-idem", 100000, "stripe"))
	// Replay the identical webhook.
	require.NoError(t, SettleReferralOnTopUp(inviteeId, "affcash-idem", 100000, "stripe"))

	inviter := reloadUser(t, inviterId)
	assert.Equal(t, 0, inviter.AffQuota, "no wallet credit on replay")
	assert.Equal(t, 0, inviter.AffHistoryQuota)
	assert.Equal(t, 1, inviter.AffCount, "aff_count not double-incremented")
	assert.Equal(t, 1000, reloadUser(t, inviteeId).Quota, "invitee bonus not double-paid")
	assert.Equal(t, int64(2), ledgerCount(t), "no duplicate ledger rows")
}
