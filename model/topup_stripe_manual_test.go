package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupManuallyCompletedStripeTopUp(t *testing.T) (User, TopUp) {
	t.Helper()
	truncateTables(t)
	useUserCacheMiniRedis(t)
	_, userID := affiliateTestSetup(t, 2000, 1000, 5)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userID).Update("quota", 125).Error)
	user := reloadUser(t, userID)
	require.NoError(t, populateUserCache(user))
	order := TopUp{UserId: user.Id, Amount: 30, Money: 30, TradeNo: "manual-stripe-recovery",
		PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(&order).Error)
	require.NoError(t, ManualCompleteTopUp(order.TradeNo, "test"))
	require.NoError(t, DB.First(&order, order.Id).Error)
	require.Equal(t, common.TopUpStatusSuccess, order.Status)
	require.Empty(t, order.PaymentIntent)
	require.Equal(t, 3125, getUserQuotaFromDB(t, user.Id))
	require.Zero(t, ledgerCount(t))
	return user, order
}

func TestStripeRechargeLinksManualTopUpWithoutRepeatingAccounting(t *testing.T) {
	for _, test := range []struct {
		name              string
		dispute           bool
		nullPaymentIntent bool
	}{
		{name: "refund"},
		{name: "dispute", dispute: true},
		{name: "legacy null payment intent", nullPaymentIntent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			user, order := setupManuallyCompletedStripeTopUp(t)
			if test.nullPaymentIntent {
				require.NoError(t, DB.Model(&order).Update("payment_intent", nil).Error)
			}
			// Receiving no association must not alter the manually settled order.
			require.NoError(t, Recharge(order.TradeNo, "cus_late", "", "test"))
			var persisted TopUp
			require.NoError(t, DB.First(&persisted, order.Id).Error)
			assert.Equal(t, order, persisted)

			require.NoError(t, Recharge(order.TradeNo, "cus_late", "pi_manual_recovery", "test"))
			require.NoError(t, Recharge(order.TradeNo, "cus_late", "pi_manual_recovery", "test"))
			// A different late event cannot replace an already recorded payment.
			require.NoError(t, Recharge(order.TradeNo, "cus_other", "pi_other", "test"))
			require.NoError(t, DB.First(&persisted, order.Id).Error)
			expected := order
			expected.PaymentIntent = "pi_manual_recovery"
			assert.Equal(t, expected, persisted)
			assert.Equal(t, 3125, getUserQuotaFromDB(t, user.Id))
			cached, err := cacheGetUserBase(user.Id)
			require.NoError(t, err)
			assert.Equal(t, 3125, cached.Quota)
			assert.Zero(t, ledgerCount(t), "linking an already settled order must not create referral rewards")
			assert.Empty(t, reloadUser(t, user.Id).StripeCustomer, "association repair must not change unrelated user fields")

			// The recovered association must support real refund/dispute accounting.
			require.NoError(t, ReverseStripeTopUp("pi_manual_recovery", 2850, 2850, test.dispute, "test"))
			require.NoError(t, ReverseStripeTopUp("pi_manual_recovery", 2850, 2850, test.dispute, "test"))
			require.NoError(t, Recharge(order.TradeNo, "cus_late", "pi_manual_recovery", "test"))
			require.NoError(t, DB.First(&persisted, order.Id).Error)
			expected.Status = common.TopUpStatusRefunded
			if test.dispute {
				expected.Status = common.TopUpStatusDisputed
			}
			expected.ClawedBackQuota = 3000
			assert.Equal(t, expected, persisted)
			assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id), "duplicate callbacks must neither debit nor credit twice")
			cached, err = cacheGetUserBase(user.Id)
			require.NoError(t, err)
			assert.Equal(t, 125, cached.Quota)
		})
	}
}

func TestStripeRechargeRetriesManualTopUpPaymentIntentWriteFailure(t *testing.T) {
	user, order := setupManuallyCompletedStripeTopUp(t)
	const callback = "test:stripe_manual_payment_intent_failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "top_ups" {
			tx.AddError(errors.New("payment intent storage unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })

	require.Error(t, Recharge(order.TradeNo, "cus_late", "pi_manual_retry", "test"))
	var persisted TopUp
	require.NoError(t, DB.First(&persisted, order.Id).Error)
	assert.Equal(t, order, persisted)
	assert.Equal(t, 3125, getUserQuotaFromDB(t, user.Id))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 3125, cached.Quota)
	assert.Zero(t, ledgerCount(t))

	require.NoError(t, DB.Callback().Update().Remove(callback))
	require.NoError(t, Recharge(order.TradeNo, "cus_late", "pi_manual_retry", "test"))
	require.NoError(t, DB.First(&persisted, order.Id).Error)
	order.PaymentIntent = "pi_manual_retry"
	assert.Equal(t, order, persisted)
	assert.Equal(t, 3125, getUserQuotaFromDB(t, user.Id))
	assert.Zero(t, ledgerCount(t))
}

func TestStripeRechargeDoesNotLinkRefundedOrDisputedOrders(t *testing.T) {
	for _, status := range []string{common.TopUpStatusRefunded, common.TopUpStatusDisputed} {
		t.Run(status, func(t *testing.T) {
			truncateTables(t)
			user := createReserveTestUser(t, 125)
			order := TopUp{UserId: user.Id, Amount: 30, Money: 30, TradeNo: "terminal-stripe-recovery",
				PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe,
				Status: status, ClawedBackQuota: 3000, CompleteTime: 123}
			require.NoError(t, DB.Create(&order).Error)
			require.NoError(t, Recharge(order.TradeNo, "cus_late", "pi_terminal", "test"))
			var persisted TopUp
			require.NoError(t, DB.First(&persisted, order.Id).Error)
			assert.Equal(t, order, persisted)
			assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id))
		})
	}
}
