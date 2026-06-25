package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferralEndToEndSmoke exercises the FULL referral wiring through the real payment
// gateway entry points (RechargeCreem / RechargeWaffo) — not the settlement function in
// isolation — so it covers: registration-deferral, first-recharge bonus + commission,
// per-recharge commission across two gateways, gateway-level idempotency, and the
// stats / leaderboard read paths.
func TestReferralEndToEndSmoke(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}))

	ids := []int{8201, 8202, 8210, 8211}
	require.NoError(t, DB.Exec("DELETE FROM affiliate_commissions").Error)
	require.NoError(t, DB.Unscoped().Where("id IN ?", ids).Delete(&User{}).Error)
	require.NoError(t, DB.Where("trade_no LIKE ?", "smk-%").Delete(&TopUp{}).Error)

	prevInviter, prevInvitee, prevPercent, prevNew := common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent, common.QuotaForNewUser
	pay := operation_setting.GetPaymentSetting()
	prevConfirmed, prevVersion := pay.ComplianceConfirmed, pay.ComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent, common.QuotaForNewUser = prevInviter, prevInvitee, prevPercent, prevNew
		pay.ComplianceConfirmed, pay.ComplianceTermsVersion = prevConfirmed, prevVersion
		_ = DB.Exec("DELETE FROM affiliate_commissions").Error
		_ = DB.Unscoped().Where("id IN ?", ids).Delete(&User{}).Error
		_ = DB.Where("trade_no LIKE ?", "smk-%").Delete(&TopUp{}).Error
	})

	common.QuotaForInviter = 2000
	common.QuotaForInvitee = 1000
	common.AffRechargeCommissionPercent = 5
	common.QuotaForNewUser = 500
	pay.ComplianceConfirmed = true
	pay.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	reload := func(id int) User {
		var u User
		require.NoError(t, DB.Where("id = ?", id).First(&u).Error)
		return u
	}
	ledgerRows := func() int64 {
		var n int64
		require.NoError(t, DB.Model(&AffiliateCommission{}).Count(&n).Error)
		return n
	}
	makeTopUp := func(tradeNo, provider string, userId int, amount int64) {
		require.NoError(t, DB.Create(&TopUp{
			UserId:          userId,
			Amount:          amount,
			Money:           float64(amount),
			TradeNo:         tradeNo,
			PaymentProvider: provider,
			PaymentMethod:   provider,
			Status:          common.TopUpStatusPending,
			CreateTime:      common.GetTimestamp(),
		}).Error)
	}

	// --- Seed inviter + invitee (post-registration linkage). ---
	require.NoError(t, DB.Create(&User{Id: 8201, Username: "smk_inviter", Status: common.UserStatusEnabled, AffCode: "smkaff01"}).Error)
	require.NoError(t, DB.Create(&User{Id: 8202, Username: "smk_invitee", Status: common.UserStatusEnabled, AffCode: "smkaff02", InviterId: 8201}).Error)

	// ===================================================================
	// 1) Registration deferral: User.Insert with an inviter must NOT pay.
	// ===================================================================
	regInvitee := &User{Username: "smk_reg_invitee", SiteId: 0}
	require.NoError(t, regInvitee.Insert(8201))
	createdReg := func() User {
		var u User
		require.NoError(t, DB.Where("username = ? AND site_id = ?", "smk_reg_invitee", 0).First(&u).Error)
		return u
	}()
	ids = append(ids, createdReg.Id) // ensure cleanup removes it too
	assert.Equal(t, 8201, createdReg.InviterId, "InviterId must be persisted at registration")
	assert.Equal(t, common.QuotaForNewUser, createdReg.Quota, "new user gets only the new-user quota, no invitee bonus at registration")
	assert.Equal(t, 0, reload(8201).AffQuota, "inviter must NOT be paid at registration")
	assert.Equal(t, int64(0), ledgerRows(), "no ledger rows from registration")
	t.Log("✓ registration defers all referral rewards")

	// ===================================================================
	// 2) First qualifying recharge via Creem gateway -> bonus + commission.
	// ===================================================================
	makeTopUp("smk-1", PaymentProviderCreem, 8202, 100000)
	require.NoError(t, RechargeCreem("smk-1", "", "", "127.0.0.1"))

	inviter := reload(8201)
	invitee := reload(8202)
	assert.Equal(t, 7000, inviter.AffQuota, "fixed 2000 + 5%*100000 (=5000)")
	assert.Equal(t, 7000, inviter.AffHistoryQuota)
	assert.Equal(t, 1, inviter.AffCount, "one activated invitee")
	assert.Equal(t, 100000+1000, invitee.Quota, "creem credit (100000) + invitee fixed bonus (1000)")
	assert.Equal(t, int64(2), ledgerRows(), "first_bonus + recharge_commission")
	t.Log("✓ first recharge (Creem): inviter +7000, invitee +101000, 2 ledger rows")

	// ===================================================================
	// 3) Gateway-level idempotency: replaying the same trade_no must no-op.
	// ===================================================================
	require.Error(t, RechargeCreem("smk-1", "", "", "127.0.0.1"), "replaying a settled order returns an error")
	assert.Equal(t, 7000, reload(8201).AffQuota, "no double-pay on replay")
	assert.Equal(t, 100000+1000, reload(8202).Quota)
	assert.Equal(t, int64(2), ledgerRows())
	t.Log("✓ replaying smk-1 does not double-pay")

	// ===================================================================
	// 4) Second recharge via a DIFFERENT gateway (Waffo) -> commission only.
	//    Waffo credits Amount * QuotaPerUnit. Amount=2 -> 2*500000 = 1000000.
	// ===================================================================
	makeTopUp("smk-2", PaymentProviderWaffo, 8202, 2)
	require.NoError(t, RechargeWaffo("smk-2", "127.0.0.1"))

	credited2 := int64(2) * int64(common.QuotaPerUnit) // 1000000
	commission2 := credited2 * 5 / 100                 // 50000
	inviter = reload(8201)
	invitee = reload(8202)
	assert.Equal(t, 7000+int(commission2), inviter.AffQuota, "previous 7000 + 5% of second recharge")
	assert.Equal(t, 1, inviter.AffCount, "still one activated invitee (no second first-bonus)")
	assert.Equal(t, 100000+1000+int(credited2), invitee.Quota, "invitee got the waffo credit, no second fixed bonus")
	assert.Equal(t, int64(3), ledgerRows(), "one more recharge_commission row")
	t.Logf("✓ second recharge (Waffo): inviter aff_quota=%d, ledger=3, aff_count=1", inviter.AffQuota)

	// ===================================================================
	// 5) Stats + leaderboard read paths.
	// ===================================================================
	stats, err := GetAffStats(8201)
	require.NoError(t, err)
	expectedEarned := int64(7000) + commission2
	assert.Equal(t, expectedEarned, stats.PendingQuota)
	assert.Equal(t, expectedEarned, stats.TotalEarnedQuota)
	assert.Equal(t, 1, stats.ActivatedCount)
	assert.Equal(t, int64(2), stats.TotalInvited, "smk_invitee + smk_reg_invitee both have inviter_id=8201")
	assert.Equal(t, expectedEarned, stats.MonthCommissionQuota)
	t.Logf("✓ stats: pending=%d earned=%d activated=%d invited=%d month=%d",
		stats.PendingQuota, stats.TotalEarnedQuota, stats.ActivatedCount, stats.TotalInvited, stats.MonthCommissionQuota)

	board, err := GetAffLeaderboard(8201, 10)
	require.NoError(t, err)
	require.Len(t, board, 1)
	assert.Equal(t, 8202, board[0].InviteeId)
	assert.Equal(t, int64(2000)+5000+commission2, board[0].CommissionQuota, "first_bonus 2000 + comm 5000 + comm of second")
	assert.Equal(t, 2, board[0].RechargeCount, "two recharge_commission rows")
	assert.Equal(t, "sm***e", board[0].Username, "masked username")
	t.Logf("✓ leaderboard: invitee=%d contribution=%d recharges=%d username=%s",
		board[0].InviteeId, board[0].CommissionQuota, board[0].RechargeCount, board[0].Username)
}
