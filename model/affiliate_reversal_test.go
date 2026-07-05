package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReverseReferralOnTopUpClawback covers the referral clawback invariants: recharge commission
// is reversed proportionally and idempotently; the one-time first bonus is reversed only when the
// invitee is fully deactivated (and not when another top-up still credits them); cash-settled
// commission shrinks the ledger without touching the wallet.
func TestReverseReferralOnTopUpClawback(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}, &TopUp{}, &User{}))

	prev := struct {
		qpu          float64
		inviter      int
		invitee      int
		pct          float64
		confirmed    bool
		termsVersion string
	}{common.QuotaPerUnit, common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent,
		operation_setting.GetPaymentSetting().ComplianceConfirmed, operation_setting.GetPaymentSetting().ComplianceTermsVersion}

	ids := []int{7401, 7402, 7411, 7412, 7421, 7422}
	cleanup := func() {
		common.QuotaPerUnit = prev.qpu
		common.QuotaForInviter, common.QuotaForInvitee, common.AffRechargeCommissionPercent = prev.inviter, prev.invitee, prev.pct
		pay := operation_setting.GetPaymentSetting()
		pay.ComplianceConfirmed, pay.ComplianceTermsVersion = prev.confirmed, prev.termsVersion
		_ = DB.Unscoped().Where("id IN ?", ids).Delete(&User{}).Error
		_ = DB.Where("trade_no LIKE ?", "affrev-%").Delete(&TopUp{}).Error
		_ = DB.Where("invitee_id IN ?", ids).Delete(&AffiliateCommission{}).Error
	}
	cleanup()
	t.Cleanup(cleanup)

	common.QuotaPerUnit = 1 // 1 credited quota per unit of Money
	common.QuotaForInviter = 2000
	common.QuotaForInvitee = 1000
	common.AffRechargeCommissionPercent = 10
	pay := operation_setting.GetPaymentSetting()
	pay.ComplianceConfirmed = true
	pay.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	reload := func(id int) User {
		var u User
		require.NoError(t, DB.Where("id = ?", id).First(&u).Error)
		return u
	}
	seedPair := func(inviterId, inviteeId int, cashSettled bool) {
		require.NoError(t, DB.Create(&User{Id: inviterId, Username: fmt.Sprintf("aff_inv_%d", inviterId),
			AffCode: fmt.Sprintf("affc%d", inviterId), Status: common.UserStatusEnabled, AffCashSettled: cashSettled}).Error)
		require.NoError(t, DB.Create(&User{Id: inviteeId, Username: fmt.Sprintf("aff_ive_%d", inviteeId),
			AffCode: fmt.Sprintf("affc%d", inviteeId), Status: common.UserStatusEnabled, InviterId: inviterId}).Error)
	}
	seedTopUp := func(inviteeId int, tradeNo, pi string, money float64) {
		require.NoError(t, DB.Create(&TopUp{UserId: inviteeId, Amount: int64(money), Money: money,
			TradeNo: tradeNo, PaymentIntent: pi, PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe,
			Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp()}).Error)
	}
	setClawback := func(tradeNo string, clawed int64) {
		require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", tradeNo).Update("clawed_back_quota", clawed).Error)
	}

	t.Run("commission reverses proportionally and idempotently; full clawback reverses first bonus", func(t *testing.T) {
		seedPair(7401, 7402, false)
		seedTopUp(7402, "affrev-main", "pi_affrev", 100) // credited 100
		require.NoError(t, SettleReferralOnTopUp(7402, "affrev-main", 100, PaymentProviderStripe))
		// first bonus (2000 inviter + 1000 invitee) + commission floor(100*10%)=10.
		assert.EqualValues(t, 2010, reload(7401).AffQuota)
		assert.EqualValues(t, 1, reload(7401).AffCount)
		assert.EqualValues(t, 1000, reload(7402).Quota)

		// Partial clawback of 30/100 → commission reversed to floor(10*30/100)=3.
		setClawback("affrev-main", 30)
		require.NoError(t, ReverseReferralOnTopUpClawback(7402, "affrev-main", 30, 100, "ip"))
		assert.EqualValues(t, 2007, reload(7401).AffQuota, "aff_quota debited by 3")
		assert.EqualValues(t, 1, reload(7401).AffCount, "partial clawback keeps activation")
		var row AffiliateCommission
		require.NoError(t, DB.Where("trade_no = ? AND kind = ?", "affrev-main", AffiliateKindRechargeCommission).First(&row).Error)
		assert.EqualValues(t, 7, row.CommissionQuota)
		assert.EqualValues(t, 3, row.ReversedQuota)

		// Duplicate delivery at the same clawback → no further reversal.
		require.NoError(t, ReverseReferralOnTopUpClawback(7402, "affrev-main", 30, 100, "ip"))
		assert.EqualValues(t, 2007, reload(7401).AffQuota)

		// Full clawback → commission fully reversed (delta 7 more) AND first bonus reversed.
		setClawback("affrev-main", 100)
		require.NoError(t, ReverseReferralOnTopUpClawback(7402, "affrev-main", 100, 100, "ip"))
		inv := reload(7401)
		assert.EqualValues(t, 0, inv.AffQuota, "2010 - 10 commission - 2000 first bonus")
		assert.EqualValues(t, 0, inv.AffCount, "invitee deactivated")
		assert.EqualValues(t, 0, reload(7402).Quota, "invitee sign-up reward clawed back")

		// Idempotent: repeating full clawback changes nothing.
		require.NoError(t, ReverseReferralOnTopUpClawback(7402, "affrev-main", 100, 100, "ip"))
		assert.EqualValues(t, 0, reload(7401).AffQuota)
		assert.EqualValues(t, 0, reload(7401).AffCount)
	})

	t.Run("first bonus is NOT reversed while another top-up still credits the invitee", func(t *testing.T) {
		seedPair(7411, 7412, false)
		seedTopUp(7412, "affrev-a", "pi_a", 100)
		seedTopUp(7412, "affrev-b", "pi_b", 100)                                                // a second, untouched top-up
		require.NoError(t, SettleReferralOnTopUp(7412, "affrev-a", 100, PaymentProviderStripe)) // grants first bonus + commission
		require.NoError(t, SettleReferralOnTopUp(7412, "affrev-b", 100, PaymentProviderStripe)) // commission only (bonus once)
		require.EqualValues(t, 1, reload(7411).AffCount)

		// Fully claw back top-up A; B still has net credit → activation stands.
		setClawback("affrev-a", 100)
		require.NoError(t, ReverseReferralOnTopUpClawback(7412, "affrev-a", 100, 100, "ip"))
		assert.EqualValues(t, 1, reload(7411).AffCount, "still activated by top-up B")
		assert.EqualValues(t, 1000, reload(7412).Quota, "sign-up reward retained")
		var revMarker int64
		DB.Model(&AffiliateCommission{}).Where("invitee_id = ? AND kind = ?", 7412, AffiliateKindFirstBonusReversal).Count(&revMarker)
		assert.EqualValues(t, 0, revMarker, "no first-bonus reversal")
	})

	t.Run("cash-settled commission shrinks the ledger without touching the wallet", func(t *testing.T) {
		seedPair(7421, 7422, true) // inviter is a cash-settled promoter
		seedTopUp(7422, "affrev-cash", "pi_cash", 100)
		require.NoError(t, SettleReferralOnTopUp(7422, "affrev-cash", 100, PaymentProviderStripe))
		// Cash-settled: commission recorded in ledger only, wallet untouched; inviter first bonus suppressed.
		assert.EqualValues(t, 0, reload(7421).AffQuota)
		var row AffiliateCommission
		require.NoError(t, DB.Where("trade_no = ? AND kind = ?", "affrev-cash", AffiliateKindRechargeCommission).First(&row).Error)
		assert.EqualValues(t, 10, row.CommissionQuota)
		assert.True(t, row.CashSettled)

		setClawback("affrev-cash", 100)
		require.NoError(t, ReverseReferralOnTopUpClawback(7422, "affrev-cash", 100, 100, "ip"))
		require.NoError(t, DB.Where("trade_no = ? AND kind = ?", "affrev-cash", AffiliateKindRechargeCommission).First(&row).Error)
		assert.EqualValues(t, 0, row.CommissionQuota, "cash-owed basis reduced to 0")
		assert.EqualValues(t, 10, row.ReversedQuota)
		assert.EqualValues(t, 0, reload(7421).AffQuota, "wallet never touched for cash-settled")
	})
}

// affReversalEnv sets crisp referral config (QuotaPerUnit=1) and returns a cleanup for the given ids.
func affReversalEnv(t *testing.T, ids []int) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&AffiliateCommission{}, &TopUp{}, &User{}))
	pay := operation_setting.GetPaymentSetting()
	prev := []interface{}{common.QuotaPerUnit, common.QuotaForInviter, common.QuotaForInvitee,
		common.AffRechargeCommissionPercent, pay.ComplianceConfirmed, pay.ComplianceTermsVersion}
	clean := func() {
		common.QuotaPerUnit = prev[0].(float64)
		common.QuotaForInviter, common.QuotaForInvitee = prev[1].(int), prev[2].(int)
		common.AffRechargeCommissionPercent = prev[3].(float64)
		pay.ComplianceConfirmed, pay.ComplianceTermsVersion = prev[4].(bool), prev[5].(string)
		_ = DB.Unscoped().Where("id IN ?", ids).Delete(&User{}).Error
		_ = DB.Where("invitee_id IN ?", ids).Delete(&AffiliateCommission{}).Error
		for _, id := range ids {
			_ = DB.Where("user_id = ?", id).Delete(&TopUp{}).Error
		}
	}
	clean()
	t.Cleanup(clean)
	common.QuotaPerUnit = 1
	common.QuotaForInviter = 2000
	common.QuotaForInvitee = 1000
	common.AffRechargeCommissionPercent = 10
	pay.ComplianceConfirmed = true
	pay.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func affReload(t *testing.T, id int) User {
	t.Helper()
	var u User
	require.NoError(t, DB.Where("id = ?", id).First(&u).Error)
	return u
}

func affSeedPair(t *testing.T, inviterId, inviteeId int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{Id: inviterId, Username: fmt.Sprintf("u%d", inviterId),
		AffCode: fmt.Sprintf("c%d", inviterId), Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: inviteeId, Username: fmt.Sprintf("u%d", inviteeId),
		AffCode: fmt.Sprintf("c%d", inviteeId), Status: common.UserStatusEnabled, InviterId: inviterId}).Error)
}

func affSeedTopUp(t *testing.T, userId int, tradeNo, pi, provider string, amount int64, money float64) {
	t.Helper()
	require.NoError(t, DB.Create(&TopUp{UserId: userId, Amount: amount, Money: money, TradeNo: tradeNo,
		PaymentIntent: pi, PaymentProvider: provider, PaymentMethod: provider,
		Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp()}).Error)
}

// A subscription-generated TopUp (Amount=0, Money>0, empty provider) credits NO balance, so a fully
// refunded Stripe activation must still fully deactivate the invitee. The old Money*QuotaPerUnit
// formula wrongly scored it net-positive and kept the first bonus.
func TestReverseReferral_SubscriptionRowDoesNotBlockDeactivation(t *testing.T) {
	affReversalEnv(t, []int{7501, 7502})
	affSeedPair(t, 7501, 7502)
	affSeedTopUp(t, 7502, "sub-stripe", "pi_sub", PaymentProviderStripe, 100, 100) // credited 100
	affSeedTopUp(t, 7502, "sub-order", "", "", 0, 50)                              // subscription row: credits 0
	require.NoError(t, SettleReferralOnTopUp(7502, "sub-stripe", 100, PaymentProviderStripe))
	require.EqualValues(t, 1, affReload(t, 7501).AffCount)

	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "sub-stripe").Update("clawed_back_quota", 100).Error)
	require.NoError(t, ReverseReferralOnTopUpClawback(7502, "sub-stripe", 100, 100, "ip"))
	assert.EqualValues(t, 0, affReload(t, 7501).AffCount, "subscription row must not keep the invitee activated")
}

// A Creem promo top-up (Money=0, Amount>0) credited real quota via the Amount formula, so a fully
// refunded Stripe activation must NOT deactivate the invitee. The old Money-only formula wrongly
// scored it 0 and over-reversed.
func TestReverseReferral_CreemPromoKeepsActivation(t *testing.T) {
	affReversalEnv(t, []int{7511, 7512})
	affSeedPair(t, 7511, 7512)
	affSeedTopUp(t, 7512, "cp-stripe", "pi_cp", PaymentProviderStripe, 100, 100)
	affSeedTopUp(t, 7512, "cp-creem", "", PaymentProviderCreem, 30, 0) // Creem credits Amount=30, Money=0
	require.NoError(t, SettleReferralOnTopUp(7512, "cp-stripe", 100, PaymentProviderStripe))

	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "cp-stripe").Update("clawed_back_quota", 100).Error)
	require.NoError(t, ReverseReferralOnTopUpClawback(7512, "cp-stripe", 100, 100, "ip"))
	assert.EqualValues(t, 1, affReload(t, 7511).AffCount, "Creem-credited invitee stays activated")
	var markers int64
	DB.Model(&AffiliateCommission{}).Where("invitee_id = ? AND kind = ?", 7512, AffiliateKindFirstBonusReversal).Count(&markers)
	assert.EqualValues(t, 0, markers)
}

// After a full refund reverses the first bonus, a genuine re-activation must re-earn it.
func TestReverseReferral_ReactivationReEarnsFirstBonus(t *testing.T) {
	affReversalEnv(t, []int{7521, 7522})
	affSeedPair(t, 7521, 7522)
	affSeedTopUp(t, 7522, "re-a", "pi_ra", PaymentProviderStripe, 100, 100)
	require.NoError(t, SettleReferralOnTopUp(7522, "re-a", 100, PaymentProviderStripe))
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "re-a").Update("clawed_back_quota", 100).Error)
	require.NoError(t, ReverseReferralOnTopUpClawback(7522, "re-a", 100, 100, "ip"))
	require.EqualValues(t, 0, affReload(t, 7521).AffCount, "deactivated after refund")

	// New paid activation → re-grant.
	affSeedTopUp(t, 7522, "re-b", "pi_rb", PaymentProviderStripe, 100, 100)
	require.NoError(t, SettleReferralOnTopUp(7522, "re-b", 100, PaymentProviderStripe))
	assert.EqualValues(t, 1, affReload(t, 7521).AffCount, "re-activation re-earns the bonus + count")
}

// Legacy first_bonus rows (RechargeQuota<=0) must NOT guess the invitee reward from current config.
func TestReverseReferral_LegacyInviteeRewardNotGuessed(t *testing.T) {
	affReversalEnv(t, []int{7531, 7532})
	affSeedPair(t, 7531, 7532)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 7531).Updates(map[string]interface{}{"aff_quota": 2000, "aff_history": 2000, "aff_count": 1}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 7532).Update("quota", 5000).Error)
	affSeedTopUp(t, 7532, "lg", "pi_lg", PaymentProviderStripe, 100, 100)
	// Legacy first_bonus row: inviter reward stored, invitee reward NOT stored (RechargeQuota=0).
	require.NoError(t, DB.Create(&AffiliateCommission{InviterId: 7531, InviteeId: 7532,
		TradeNo: affiliateFirstBonusKey(7532), Kind: AffiliateKindFirstBonus,
		RechargeQuota: 0, CommissionQuota: 2000, CreatedAt: common.GetTimestamp()}).Error)

	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "lg").Update("clawed_back_quota", 100).Error)
	require.NoError(t, ReverseReferralOnTopUpClawback(7532, "lg", 100, 100, "ip"))
	assert.EqualValues(t, 0, affReload(t, 7531).AffQuota, "inviter exact bonus reversed")
	assert.EqualValues(t, 5000, affReload(t, 7532).Quota, "legacy invitee reward NOT guessed/clawed")
}
