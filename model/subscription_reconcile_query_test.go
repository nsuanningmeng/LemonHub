package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingEpaySubscriptionOrdersWindowAndOrder(t *testing.T) {
	require.NoError(t, DB.Where("trade_no LIKE ?", "SUBPENDQ%").Delete(&SubscriptionOrder{}).Error)
	t.Cleanup(func() {
		assert.NoError(t, DB.Where("trade_no LIKE ?", "SUBPENDQ%").Delete(&SubscriptionOrder{}).Error)
	})

	now := common.GetTimestamp()
	createOrder := func(tradeNo, provider, status string, createTime int64) {
		require.NoError(t, DB.Create(&SubscriptionOrder{
			UserId:          1,
			PlanId:          1,
			Money:           19.95,
			TradeNo:         tradeNo,
			PaymentMethod:   "alipay",
			PaymentProvider: provider,
			Status:          status,
			CreateTime:      createTime,
		}).Error)
	}
	createOrder("SUBPENDQ_in_old", PaymentProviderEpay, common.TopUpStatusPending, now-1000)
	createOrder("SUBPENDQ_in_new", PaymentProviderEpay, common.TopUpStatusPending, now-200)
	createOrder("SUBPENDQ_too_new", PaymentProviderEpay, common.TopUpStatusPending, now-10)
	createOrder("SUBPENDQ_too_old", PaymentProviderEpay, common.TopUpStatusPending, now-99999)
	createOrder("SUBPENDQ_stripe", PaymentProviderStripe, common.TopUpStatusPending, now-200)
	createOrder("SUBPENDQ_done", PaymentProviderEpay, common.TopUpStatusSuccess, now-200)

	createdAfter, createdBefore := now-5000, now-100
	assert.True(t, HasPendingEpaySubscriptionOrders(createdAfter, createdBefore))

	orders, err := GetPendingEpaySubscriptionOrders(createdAfter, createdBefore, 100)
	require.NoError(t, err)
	tradeNumbers := make([]string, 0, len(orders))
	for _, order := range orders {
		tradeNumbers = append(tradeNumbers, order.TradeNo)
	}
	assert.Equal(t, []string{"SUBPENDQ_in_new", "SUBPENDQ_in_old"}, tradeNumbers)
	require.NoError(t, MarkEpaySubscriptionOrderQueryAttempts([]int{orders[0].Id}, now))
	next, err := GetPendingEpaySubscriptionOrders(createdAfter, createdBefore, 1)
	require.NoError(t, err)
	require.Len(t, next, 1)
	assert.Equal(t, "SUBPENDQ_in_old", next[0].TradeNo, "an attempted first page must not starve the remaining backlog")

	assert.False(t, HasPendingEpaySubscriptionOrders(now+1000, now+2000))
	empty, err := GetPendingEpaySubscriptionOrders(now+1000, now+2000, 100)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestClaimEpaySubscriptionOrderQueryAttemptEnforcesPersistentCooldown(t *testing.T) {
	const tradeNo = "SUBPENDQ_CLAIM"
	require.NoError(t, DB.Where("trade_no = ?", tradeNo).Delete(&SubscriptionOrder{}).Error)
	t.Cleanup(func() {
		assert.NoError(t, DB.Where("trade_no = ?", tradeNo).Delete(&SubscriptionOrder{}).Error)
	})

	order := &SubscriptionOrder{
		UserId: 1, PlanId: 1, Money: 19.95, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(order).Error)

	claimed, err := ClaimEpaySubscriptionOrderQueryAttempt(order.Id, 100, 85)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = ClaimEpaySubscriptionOrderQueryAttempt(order.Id, 101, 86)
	require.NoError(t, err)
	assert.False(t, claimed)
	claimed, err = ClaimEpaySubscriptionOrderQueryAttempt(order.Id, 116, 101)
	require.NoError(t, err)
	assert.True(t, claimed)

	var reloaded SubscriptionOrder
	require.NoError(t, DB.First(&reloaded, order.Id).Error)
	require.NotNil(t, reloaded.EpayCallbackQueryTime)
	assert.EqualValues(t, 116, *reloaded.EpayCallbackQueryTime)
	assert.Nil(t, reloaded.EpayQueryTime, "public callbacks must not change reconciliation fairness")
}
