package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompleteEpayTopUpRejectsOversizedAmount protects the billing invariant that a
// top-up credit can never wrap negative or exceed the wallet's exact integer domain.
// Invalid legacy orders remain pending for manual resolution and credit no quota.
func TestCompleteEpayTopUpRejectsOversizedAmount(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TopUp{}, &User{}))
	const tradeNo = "SATURATE1"
	cleanup := func() {
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
	}
	cleanup()
	defer cleanup()

	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	overflowAmount := int64(common.MaxWalletQuota)/500000 + 1

	pw, _ := common.Password2Hash("x")
	u := &User{Username: "saturateu", Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "saturateaff"}
	require.NoError(t, DB.Create(u).Error)
	defer DB.Where("id = ?", u.Id).Delete(&User{})

	require.NoError(t, DB.Create(&TopUp{
		UserId: u.Id, Amount: overflowAmount, Money: 1, TradeNo: tradeNo,
		PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay",
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
	}).Error)

	finalStatus, quotaAdded, err := CompleteEpayTopUp(tradeNo, 0, 1)
	require.ErrorIs(t, err, ErrInvalidTopUpQuota)
	assert.Empty(t, finalStatus)
	assert.Zero(t, quotaAdded)

	var got User
	require.NoError(t, DB.Select("quota").First(&got, u.Id).Error)
	assert.Zero(t, got.Quota)

	var gotTopUp TopUp
	require.NoError(t, DB.Select("status").Where("trade_no = ?", tradeNo).First(&gotTopUp).Error)
	assert.Equal(t, common.TopUpStatusPending, gotTopUp.Status)
}

func TestLargeEpayTopUpCreditsExistingLargeWalletExactlyOnce(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	for _, amount := range []int64{5000, 10000, 30000} {
		t.Run(fmt.Sprintf("amount_%d", amount), func(t *testing.T) {
			truncateTables(t)
			useUserCacheMiniRedis(t)
			const existingQuota = 5_000_000_000
			user := createReserveTestUser(t, existingQuota)
			require.NoError(t, populateUserCache(user))
			order := TopUp{UserId: user.Id, Amount: amount, Money: float64(amount),
				TradeNo: "large-epay", PaymentProvider: PaymentProviderEpay,
				PaymentMethod: "alipay", Status: common.TopUpStatusPending}
			require.NoError(t, DB.Create(&order).Error)

			creditedQuota := int(amount * 500000)
			require.NoError(t, ValidateTopUpQuotaCapacity(user.Id, creditedQuota))
			alreadyDone, err := RechargeEpay(order.TradeNo, "alipay", "test")
			require.NoError(t, err)
			assert.False(t, alreadyDone)
			assert.Equal(t, existingQuota+creditedQuota, getUserQuotaFromDB(t, user.Id))
			cached, err := cacheGetUserBase(user.Id)
			require.NoError(t, err)
			assert.Equal(t, existingQuota+creditedQuota, cached.Quota)

			alreadyDone, err = RechargeEpay(order.TradeNo, "alipay", "test")
			require.NoError(t, err)
			assert.True(t, alreadyDone)
			assert.Equal(t, existingQuota+creditedQuota, getUserQuotaFromDB(t, user.Id))
		})
	}
}

func TestLargeTopUpProviderAndManualSettlement(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	for _, testCase := range []struct {
		name     string
		provider string
		amount   int64
		settle   func(string) error
	}{
		{"stripe", PaymentProviderStripe, 10000, func(tradeNo string) error { return Recharge(tradeNo, "customer", "pi-large", "test") }},
		{"creem", PaymentProviderCreem, 5_000_000_000, func(tradeNo string) error { return RechargeCreem(tradeNo, "", "", "test") }},
		{"waffo", PaymentProviderWaffo, 10000, func(tradeNo string) error { return RechargeWaffo(tradeNo, "test") }},
		{"waffo_pancake", PaymentProviderWaffoPancake, 10000, RechargeWaffoPancake},
		{"manual_epay", PaymentProviderEpay, 10000, func(tradeNo string) error { return ManualCompleteTopUp(tradeNo, "test") }},
		{"manual_stripe", PaymentProviderStripe, 10000, func(tradeNo string) error { return ManualCompleteTopUp(tradeNo, "test") }},
		{"manual_creem", PaymentProviderCreem, 5_000_000_000, func(tradeNo string) error { return ManualCompleteTopUp(tradeNo, "test") }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			user := createReserveTestUser(t, 5_000_000_000)
			order := TopUp{UserId: user.Id, Amount: testCase.amount, Money: 10000,
				TradeNo: "large-provider", PaymentProvider: testCase.provider,
				PaymentMethod: testCase.provider, Status: common.TopUpStatusPending}
			require.NoError(t, DB.Create(&order).Error)

			require.NoError(t, testCase.settle(order.TradeNo))
			assert.Equal(t, 10_000_000_000, getUserQuotaFromDB(t, user.Id))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
			// Providers may reject the terminal state, but a redelivery must never credit twice.
			_ = testCase.settle(order.TradeNo)
			assert.Equal(t, 10_000_000_000, getUserQuotaFromDB(t, user.Id))
		})
	}
}
