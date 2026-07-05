package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReverseStripeTopUp covers the refund/chargeback clawback invariants: refunds
// claw back proportionally to the cumulative refunded fraction, redelivered/duplicate
// events never double-debit, disputes reverse the full remaining credit and flag the
// order, the balance may go negative (soft-quota), and an unknown payment_intent is
// ignored via ErrTopUpNotFound.
func TestReverseStripeTopUp(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TopUp{}, &User{}, &Log{}))

	prevQPU := common.QuotaPerUnit
	uids := []int{9301, 9302, 9303, 9304, 9305, 9306}
	cleanup := func() {
		common.QuotaPerUnit = prevQPU
		_ = DB.Unscoped().Where("id IN ?", uids).Delete(&User{}).Error
		_ = DB.Where("trade_no LIKE ?", "rev-%").Delete(&TopUp{}).Error
	}
	cleanup()
	t.Cleanup(cleanup)

	// 1 credited quota per unit of Money → assertions read in whole units.
	common.QuotaPerUnit = 1

	reloadUser := func(id int) User {
		var u User
		require.NoError(t, DB.Where("id = ?", id).First(&u).Error)
		return u
	}
	reloadTopUp := func(tradeNo string) TopUp {
		var tp TopUp
		require.NoError(t, DB.Where("trade_no = ?", tradeNo).First(&tp).Error)
		return tp
	}
	seed := func(userId int, tradeNo, paymentIntent string, money float64, startQuota int) {
		require.NoError(t, DB.Create(&User{
			Id: userId, Username: fmt.Sprintf("rev_%d", userId),
			AffCode: fmt.Sprintf("revaff%d", userId), // aff_code is UNIQUE
			Status:  common.UserStatusEnabled, Quota: startQuota,
		}).Error)
		require.NoError(t, DB.Create(&TopUp{
			UserId: userId, Amount: int64(money), Money: money,
			TradeNo: tradeNo, PaymentIntent: paymentIntent,
			PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe,
			Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(),
		}).Error)
	}

	t.Run("proportional partial refund is idempotent then completes to refunded", func(t *testing.T) {
		seed(9301, "rev-partial", "pi_partial", 100, 100) // credited 100, balance 100

		// Partial refund of 30/100 minor units → claw back 30.
		require.NoError(t, ReverseStripeTopUp("pi_partial", 30, 100, false, "1.2.3.4"))
		assert.EqualValues(t, 70, reloadUser(9301).Quota)
		tp := reloadTopUp("rev-partial")
		assert.EqualValues(t, 30, tp.ClawedBackQuota)
		assert.Equal(t, common.TopUpStatusSuccess, tp.Status, "partial refund keeps success")

		// Duplicate delivery (same cumulative amount_refunded) must not debit again.
		require.NoError(t, ReverseStripeTopUp("pi_partial", 30, 100, false, "1.2.3.4"))
		assert.EqualValues(t, 70, reloadUser(9301).Quota)

		// Cumulative refund reaches full → claws remaining 70, marks refunded.
		require.NoError(t, ReverseStripeTopUp("pi_partial", 100, 100, false, "1.2.3.4"))
		assert.EqualValues(t, 0, reloadUser(9301).Quota)
		assert.Equal(t, common.TopUpStatusRefunded, reloadTopUp("rev-partial").Status)
	})

	t.Run("dispute reverses full remaining, flags disputed, allows negative balance", func(t *testing.T) {
		seed(9302, "rev-dispute", "pi_dispute", 100, 20) // credited 100, already spent down to 20
		require.NoError(t, ReverseStripeTopUp("pi_dispute", 100, 100, true, "1.2.3.4"))
		assert.EqualValues(t, -80, reloadUser(9302).Quota, "soft-quota: balance may go negative")
		assert.Equal(t, common.TopUpStatusDisputed, reloadTopUp("rev-dispute").Status)
	})

	t.Run("unknown or empty payment_intent returns ErrTopUpNotFound", func(t *testing.T) {
		assert.ErrorIs(t, ReverseStripeTopUp("pi_nonexistent", 100, 100, false, "1.2.3.4"), ErrTopUpNotFound)
		assert.ErrorIs(t, ReverseStripeTopUp("", 100, 100, false, "1.2.3.4"), ErrTopUpNotFound)
	})

	t.Run("dispute status is sticky: a later refund does not downgrade to refunded", func(t *testing.T) {
		seed(9304, "rev-sticky", "pi_sticky", 100, 100)
		// Chargeback first → full reversal, flagged disputed.
		require.NoError(t, ReverseStripeTopUp("pi_sticky", 100, 100, true, "1.2.3.4"))
		assert.EqualValues(t, 0, reloadUser(9304).Quota)
		assert.Equal(t, common.TopUpStatusDisputed, reloadTopUp("rev-sticky").Status)
		// Merchant then refunds the disputed charge → no further debit, and the
		// order MUST stay disputed (keeps its chargeback review flag).
		require.NoError(t, ReverseStripeTopUp("pi_sticky", 100, 100, false, "1.2.3.4"))
		assert.EqualValues(t, 0, reloadUser(9304).Quota, "no additional debit")
		assert.Equal(t, common.TopUpStatusDisputed, reloadTopUp("rev-sticky").Status,
			"refund after dispute must not demote disputed -> refunded")
	})

	t.Run("proportional rounding is exact (1/3 of 100 -> 33)", func(t *testing.T) {
		seed(9305, "rev-round", "pi_round", 100, 100)
		require.NoError(t, ReverseStripeTopUp("pi_round", 1, 3, false, "1.2.3.4"))
		assert.EqualValues(t, 67, reloadUser(9305).Quota) // 100 - round(100*1/3)=100-33
		assert.EqualValues(t, 33, reloadTopUp("rev-round").ClawedBackQuota)
	})

	t.Run("refund with non-positive charge amount does not over-claw", func(t *testing.T) {
		seed(9306, "rev-badamt", "pi_badamt", 100, 100)
		// chargeMinor<=0 (e.g. a parse failure upstream) must NOT escalate to a full
		// clawback of a partial refund; it is rejected as an invalid amount.
		assert.ErrorIs(t, ReverseStripeTopUp("pi_badamt", 30, 0, false, "1.2.3.4"), ErrTopUpAmountInvalid)
		assert.EqualValues(t, 100, reloadUser(9306).Quota, "balance unchanged")
		assert.Equal(t, common.TopUpStatusSuccess, reloadTopUp("rev-badamt").Status)
	})

	t.Run("Money<=0 order is rejected without state change", func(t *testing.T) {
		require.NoError(t, DB.Create(&User{
			Id: 9303, Username: "rev_zero", AffCode: "revaffzero",
			Status: common.UserStatusEnabled, Quota: 500,
		}).Error)
		require.NoError(t, DB.Create(&TopUp{
			UserId: 9303, Amount: 0, Money: 0, TradeNo: "rev-zero", PaymentIntent: "pi_zero",
			PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe,
			Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(),
		}).Error)
		assert.ErrorIs(t, ReverseStripeTopUp("pi_zero", 100, 100, false, "1.2.3.4"), ErrTopUpStatusInvalid)
		assert.EqualValues(t, 500, reloadUser(9303).Quota)
		assert.Equal(t, common.TopUpStatusSuccess, reloadTopUp("rev-zero").Status)
	})
}
