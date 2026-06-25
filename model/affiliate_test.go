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
