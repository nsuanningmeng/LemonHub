package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createSubscriptionOrderSnapshotTestUser(t *testing.T, group string) *User {
	t.Helper()
	user := &User{
		Username: "subscription_order_snapshot_" + common.GetRandomString(8),
		Status:   common.UserStatusEnabled,
		Group:    group,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func createSubscriptionOrderSnapshotTestPlan(t *testing.T, maxPurchase int) *SubscriptionPlan {
	t.Helper()
	allowWalletOverflow := false
	plan := &SubscriptionPlan{
		Title:                   "Original checkout plan",
		PriceAmount:             12.34,
		Currency:                "USD",
		DurationUnit:            SubscriptionDurationCustom,
		CustomSeconds:           3600,
		Enabled:                 true,
		AllowWalletOverflow:     &allowWalletOverflow,
		MaxPurchasePerUser:      maxPurchase,
		UpgradeGroup:            "snapshot-pro",
		DowngradeGroup:          "snapshot-basic",
		TotalAmount:             12345,
		QuotaResetPeriod:        SubscriptionResetNever,
		QuotaResetCustomSeconds: 0,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func newSubscriptionOrderSnapshotTestOrder(userID, planID int, tradeNo string) *SubscriptionOrder {
	return &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           12.34,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
}

func TestCompleteSubscriptionOrderUsesCheckoutPlanSnapshot(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 0)
	order := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "snapshot-live-plan-change")
	require.NoError(t, order.InsertWithPlanSnapshot(plan))
	require.NotEmpty(t, order.PlanSnapshot)

	newAllowWalletOverflow := true
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"title":                 "Changed after checkout",
		"price_amount":          99.99,
		"custom_seconds":        7200,
		"total_amount":          99999,
		"upgrade_group":         "changed-pro",
		"downgrade_group":       "changed-basic",
		"allow_wallet_overflow": newAllowWalletOverflow,
	}).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	completed, err := CompleteSubscriptionOrderWithResult(
		order.TradeNo,
		`{"provider":"epay"}`,
		PaymentProviderEpay,
		"alipay",
	)
	require.NoError(t, err)
	assert.True(t, completed)

	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).First(&subscription).Error)
	assert.EqualValues(t, 12345, subscription.AmountTotal)
	assert.EqualValues(t, 3600, subscription.EndTime-subscription.StartTime)
	assert.Equal(t, "snapshot-pro", subscription.UpgradeGroup)
	assert.Equal(t, "starter", subscription.PrevUserGroup)
	assert.Equal(t, "snapshot-basic", subscription.DowngradeGroup)
	assert.False(t, subscription.AllowWalletOverflow)

	var reloadedUser User
	require.NoError(t, DB.First(&reloadedUser, user.Id).Error)
	assert.Equal(t, "snapshot-pro", reloadedUser.Group)
	assert.Equal(t, common.TopUpStatusSuccess, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}

func TestCompleteSubscriptionOrderRejectsMalformedSnapshotWithoutChangingPendingOrder(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 0)
	order := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "snapshot-malformed")
	order.PlanSnapshot = `{"version":`
	require.NoError(t, DB.Create(order).Error)

	completed, err := CompleteSubscriptionOrderWithResult(
		order.TradeNo,
		`{"provider":"epay"}`,
		PaymentProviderEpay,
		"alipay",
	)
	assert.False(t, completed)
	require.ErrorIs(t, err, ErrSubscriptionOrderPlanSnapshot)

	reloaded := GetSubscriptionOrderByTradeNo(order.TradeNo)
	require.NotNil(t, reloaded)
	assert.Equal(t, common.TopUpStatusPending, reloaded.Status)
	var subscriptions int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptions).Error)
	assert.Zero(t, subscriptions)
	var topups int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topups).Error)
	assert.Zero(t, topups)
}

func TestCompleteLegacySubscriptionOrderRequiresPlanToPredateCheckout(t *testing.T) {
	t.Run("plan changed at checkout time is rejected", func(t *testing.T) {
		truncateTables(t)
		user := createSubscriptionOrderSnapshotTestUser(t, "starter")
		plan := createSubscriptionOrderSnapshotTestPlan(t, 0)
		checkoutTime := common.GetTimestamp() + 10
		require.NoError(t, DB.Exec("UPDATE subscription_plans SET updated_at = ? WHERE id = ?", checkoutTime, plan.Id).Error)
		order := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "legacy-plan-not-older")
		order.CreateTime = checkoutTime
		require.NoError(t, DB.Create(order).Error)

		completed, err := CompleteSubscriptionOrderWithResult(
			order.TradeNo,
			`{"provider":"epay"}`,
			PaymentProviderEpay,
			"alipay",
		)
		assert.False(t, completed)
		require.ErrorIs(t, err, ErrSubscriptionOrderPlanChanged)
		assert.Equal(t, common.TopUpStatusPending, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
		var subscriptions int64
		require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptions).Error)
		assert.Zero(t, subscriptions)
	})

	t.Run("strictly older plan with the same price is fulfilled", func(t *testing.T) {
		truncateTables(t)
		user := createSubscriptionOrderSnapshotTestUser(t, "starter")
		plan := createSubscriptionOrderSnapshotTestPlan(t, 0)
		checkoutTime := common.GetTimestamp() + 10
		require.NoError(t, DB.Exec("UPDATE subscription_plans SET updated_at = ? WHERE id = ?", checkoutTime-1, plan.Id).Error)
		order := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "legacy-plan-older")
		order.CreateTime = checkoutTime
		require.NoError(t, DB.Create(order).Error)

		completed, err := CompleteSubscriptionOrderWithResult(
			order.TradeNo,
			`{"provider":"epay"}`,
			PaymentProviderEpay,
			"alipay",
		)
		require.NoError(t, err)
		assert.True(t, completed)
		assert.Equal(t, common.TopUpStatusSuccess, GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
		var subscriptions int64
		require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptions).Error)
		assert.EqualValues(t, 1, subscriptions)
	})
}

func TestInsertWithPlanSnapshotReloadsAuthoritativePlanBeforeReserving(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 2)
	stalePlan := *plan

	// Simulate another instance updating the plan while this process still holds a
	// same-second cached copy. UpdatedAt equality must not make the cached row trusted.
	revisedAllowOverflow := true
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"title":                 "Authoritative revised plan",
		"price_amount":          23.45,
		"custom_seconds":        7200,
		"total_amount":          54321,
		"allow_wallet_overflow": revisedAllowOverflow,
		"max_purchase_per_user": 1,
		"updated_at":            stalePlan.UpdatedAt,
	}).Error)

	first := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "stale-plan-first")
	require.NoError(t, first.InsertWithPlanSnapshot(&stalePlan))
	assert.Equal(t, 23.45, first.Money)
	assert.Equal(t, 23.45, stalePlan.PriceAmount, "the caller must use the locked DB price upstream")
	assert.Equal(t, 1, stalePlan.MaxPurchasePerUser)
	assert.Equal(t, "Authoritative revised plan", stalePlan.Title)
	assert.True(t, first.PurchaseLimitReserved)

	snapshotPlan, err := subscriptionPlanFromOrderSnapshot(first)
	require.NoError(t, err)
	assert.Equal(t, 23.45, snapshotPlan.PriceAmount)
	assert.Equal(t, 1, snapshotPlan.MaxPurchasePerUser)
	assert.EqualValues(t, 7200, snapshotPlan.CustomSeconds)
	assert.EqualValues(t, 54321, snapshotPlan.TotalAmount)
	assert.True(t, *snapshotPlan.AllowWalletOverflow)

	secondStalePlan := *plan
	second := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "stale-plan-second")
	err = second.InsertWithPlanSnapshot(&secondStalePlan)
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimitReached)
	var orderCount int64
	require.NoError(t, DB.Model(&SubscriptionOrder{}).
		Where("trade_no IN ?", []string{first.TradeNo, second.TradeNo}).
		Count(&orderCount).Error)
	assert.EqualValues(t, 1, orderCount)
}

func TestInsertWithPlanSnapshotRejectsAuthoritativelyDisabledPlan(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 1)
	stalePlan := *plan
	require.True(t, stalePlan.Enabled)

	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"enabled":    false,
		"updated_at": stalePlan.UpdatedAt,
	}).Error)
	order := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "stale-plan-disabled")
	err := order.InsertWithPlanSnapshot(&stalePlan)
	require.ErrorIs(t, err, ErrSubscriptionOrderPlanChanged)

	var orderCount int64
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("trade_no = ?", order.TradeNo).Count(&orderCount).Error)
	assert.Zero(t, orderCount)
}

func TestInsertWithPlanSnapshotReservesPurchaseLimitUntilOrderExpires(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 1)

	first := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "limit-reservation-first")
	require.NoError(t, first.InsertWithPlanSnapshot(plan))
	assert.True(t, first.PurchaseLimitReserved)
	reloadedFirst := GetSubscriptionOrderByTradeNo(first.TradeNo)
	require.NotNil(t, reloadedFirst)
	assert.True(t, reloadedFirst.PurchaseLimitReserved)

	err := DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser User
		if lockErr := lockForUpdate(tx).Select("id").Where("id = ?", user.Id).First(&lockedUser).Error; lockErr != nil {
			return lockErr
		}
		_, createErr := CreateUserSubscriptionFromPlanTx(tx, user.Id, plan, PaymentMethodBalance)
		return createErr
	})
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimitReached, "balance/admin paths must honor an Epay reservation")

	second := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "limit-reservation-second")
	err = second.InsertWithPlanSnapshot(plan)
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimitReached)
	var secondCount int64
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("trade_no = ?", second.TradeNo).Count(&secondCount).Error)
	assert.Zero(t, secondCount)

	require.NoError(t, ExpireSubscriptionOrder(first.TradeNo, PaymentProviderEpay))
	third := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "limit-reservation-after-expiry")
	require.NoError(t, third.InsertWithPlanSnapshot(plan))

	var pendingCount int64
	require.NoError(t, DB.Model(&SubscriptionOrder{}).
		Where("user_id = ? AND plan_id = ? AND status = ?", user.Id, plan.Id, common.TopUpStatusPending).
		Count(&pendingCount).Error)
	assert.EqualValues(t, 1, pendingCount)
	assert.Equal(t, common.TopUpStatusExpired, GetSubscriptionOrderByTradeNo(first.TradeNo).Status)
}

func TestUnreservedExternalOrdersReapplyPurchaseLimitAtSettlement(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 1)
	plan.UpgradeGroup = ""
	plan.DowngradeGroup = ""
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]any{
		"upgrade_group":   "",
		"downgrade_group": "",
	}).Error)

	first := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "paid-history-first")
	first.PaymentMethod = PaymentMethodStripe
	first.PaymentProvider = PaymentProviderStripe
	require.NoError(t, first.SetPlanSnapshot(plan))
	require.NoError(t, DB.Create(first).Error)
	second := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "paid-history-second")
	second.PaymentMethod = PaymentMethodStripe
	second.PaymentProvider = PaymentProviderStripe
	require.NoError(t, second.SetPlanSnapshot(plan))
	require.NoError(t, DB.Create(second).Error)

	firstCompleted, err := CompleteSubscriptionOrderWithResult(
		first.TradeNo,
		`{"provider":"stripe"}`,
		PaymentProviderStripe,
		PaymentMethodStripe,
	)
	require.NoError(t, err)
	assert.True(t, firstCompleted)
	secondCompleted, err := CompleteSubscriptionOrderWithResult(
		second.TradeNo,
		`{"provider":"stripe"}`,
		PaymentProviderStripe,
		PaymentMethodStripe,
	)
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimitReached)
	assert.False(t, secondCompleted)

	var subscriptions int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).Count(&subscriptions).Error)
	assert.EqualValues(t, 1, subscriptions, "an unreserved external order must not bypass the plan limit")
	assert.Equal(t, common.TopUpStatusSuccess, GetSubscriptionOrderByTradeNo(first.TradeNo).Status)
	assert.Equal(t, common.TopUpStatusPending, GetSubscriptionOrderByTradeNo(second.TradeNo).Status)
}

func TestEpayReservationPreventsCrossGatewayPurchaseLimitBypass(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 1)
	plan.UpgradeGroup = ""
	plan.DowngradeGroup = ""

	stripeOrder := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "cross-gateway-stripe")
	stripeOrder.PaymentMethod = PaymentMethodStripe
	stripeOrder.PaymentProvider = PaymentProviderStripe
	require.NoError(t, stripeOrder.InsertWithPlanSnapshot(plan))
	assert.False(t, stripeOrder.PurchaseLimitReserved)

	epayOrder := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "cross-gateway-epay")
	require.NoError(t, epayOrder.InsertWithPlanSnapshot(plan))
	assert.True(t, epayOrder.PurchaseLimitReserved)

	stripeCompleted, err := CompleteSubscriptionOrderWithResult(
		stripeOrder.TradeNo,
		`{"provider":"stripe"}`,
		PaymentProviderStripe,
		PaymentMethodStripe,
	)
	require.NoError(t, err)
	assert.True(t, stripeCompleted, "an older checkout that is actually paid must preempt a later unpaid reservation")

	epayCompleted, err := CompleteSubscriptionOrderWithResult(
		epayOrder.TradeNo,
		`{"provider":"epay"}`,
		PaymentProviderEpay,
		"alipay",
	)
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimitReached)
	assert.False(t, epayCompleted)

	var subscriptions int64
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).
		Count(&subscriptions).Error)
	assert.EqualValues(t, 1, subscriptions)
	assert.Equal(t, common.TopUpStatusSuccess, GetSubscriptionOrderByTradeNo(stripeOrder.TradeNo).Status)
	reloadedEpay := GetSubscriptionOrderByTradeNo(epayOrder.TradeNo)
	require.NotNil(t, reloadedEpay)
	assert.Equal(t, common.TopUpStatusPending, reloadedEpay.Status)
	assert.False(t, reloadedEpay.PurchaseLimitReserved, "the later reservation must be revoked before the paid order is granted")
}

func TestPaidOlderCheckoutKeepsLaterEpayReservationWhenCapacityRemains(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 3)
	plan.UpgradeGroup = ""
	plan.DowngradeGroup = ""

	stripeOrder := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "spare-capacity-stripe")
	stripeOrder.PaymentMethod = PaymentMethodStripe
	stripeOrder.PaymentProvider = PaymentProviderStripe
	require.NoError(t, stripeOrder.InsertWithPlanSnapshot(plan))
	epayOrder := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "spare-capacity-epay")
	require.NoError(t, epayOrder.InsertWithPlanSnapshot(plan))

	stripeCompleted, err := CompleteSubscriptionOrderWithResult(
		stripeOrder.TradeNo,
		`{"provider":"stripe"}`,
		PaymentProviderStripe,
		PaymentMethodStripe,
	)
	require.NoError(t, err)
	assert.True(t, stripeCompleted)
	reloadedEpay := GetSubscriptionOrderByTradeNo(epayOrder.TradeNo)
	require.NotNil(t, reloadedEpay)
	assert.True(t, reloadedEpay.PurchaseLimitReserved, "spare capacity must not revoke a valid later reservation")

	epayCompleted, err := CompleteSubscriptionOrderWithResult(
		epayOrder.TradeNo,
		`{"provider":"epay"}`,
		PaymentProviderEpay,
		"alipay",
	)
	require.NoError(t, err)
	assert.True(t, epayCompleted)

	var subscriptions int64
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).
		Count(&subscriptions).Error)
	assert.EqualValues(t, 2, subscriptions)
}

func TestUnreservedEpaySnapshotCannotBypassLimitAddedAfterCheckout(t *testing.T) {
	truncateTables(t)
	user := createSubscriptionOrderSnapshotTestUser(t, "starter")
	plan := createSubscriptionOrderSnapshotTestPlan(t, 0)
	plan.UpgradeGroup = ""
	plan.DowngradeGroup = ""

	first := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "old-unlimited-epay-first")
	require.NoError(t, first.InsertWithPlanSnapshot(plan))
	assert.False(t, first.PurchaseLimitReserved)
	second := newSubscriptionOrderSnapshotTestOrder(user.Id, plan.Id, "old-unlimited-epay-second")
	require.NoError(t, second.InsertWithPlanSnapshot(plan))
	assert.False(t, second.PurchaseLimitReserved)

	reusable, err := GetReusablePendingSubscriptionOrder(user.Id, plan.Id, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Nil(t, reusable, "an unreserved old quote must never be regenerated as a reserved checkout")

	require.NoError(t, DB.Model(&SubscriptionPlan{}).
		Where("id = ?", plan.Id).
		Update("max_purchase_per_user", 1).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	firstCompleted, err := CompleteSubscriptionOrderWithResult(
		first.TradeNo,
		`{"provider":"epay"}`,
		PaymentProviderEpay,
		"alipay",
	)
	require.NoError(t, err)
	assert.True(t, firstCompleted)

	secondCompleted, err := CompleteSubscriptionOrderWithResult(
		second.TradeNo,
		`{"provider":"epay"}`,
		PaymentProviderEpay,
		"alipay",
	)
	require.ErrorIs(t, err, ErrSubscriptionPurchaseLimitReached)
	assert.False(t, secondCompleted)

	var subscriptions int64
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).
		Count(&subscriptions).Error)
	assert.EqualValues(t, 1, subscriptions)
	assert.Equal(t, common.TopUpStatusPending, GetSubscriptionOrderByTradeNo(second.TradeNo).Status)
}
