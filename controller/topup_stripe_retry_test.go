package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
	"gorm.io/gorm"
)

func setupStripeWebhookRetry(t *testing.T) (*gorm.DB, model.User) {
	t.Helper()
	db, user := setupTopUpRequest(t)
	confirmPaymentComplianceForTest(t)
	previousLogDB := model.LOG_DB
	previousAPI, previousSecret, previousPrice := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = previousAPI, previousSecret, previousPrice
	})
	model.LOG_DB = db
	setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = "sk_test_retry", "whsec_test_retry", "price_test_retry"
	require.NoError(t, db.AutoMigrate(&model.SubscriptionOrder{}, &model.Log{}, &model.AffiliateCommission{}))
	return db, user
}

func deliverStripeRetryEvent(t *testing.T, eventType string, object map[string]any) int {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id": "evt_retry", "object": "event", "type": eventType,
		"data": map[string]any{"object": object},
	})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: setting.StripeWebhookSecret})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/webhook", bytes.NewReader(payload))
	ctx.Request.Header.Set("Stripe-Signature", signed.Header)
	StripeWebhook(ctx)
	ctx.Writer.WriteHeaderNow()
	return recorder.Code
}

func TestStripeWebhookRetriesFailedPersistenceWithoutDuplicateAccounting(t *testing.T) {
	for _, test := range []struct {
		eventType   string
		initial     string
		terminal    string
		quotaChange int
	}{
		{"checkout.session.completed", common.TopUpStatusPending, common.TopUpStatusSuccess, 500000},
		{"checkout.session.async_payment_succeeded", common.TopUpStatusPending, common.TopUpStatusSuccess, 500000},
		{"checkout.session.expired", common.TopUpStatusPending, common.TopUpStatusExpired, 0},
		{"checkout.session.async_payment_failed", common.TopUpStatusPending, common.TopUpStatusFailed, 0},
		{"charge.refunded", common.TopUpStatusSuccess, common.TopUpStatusRefunded, -500000},
		{"charge.dispute.created", common.TopUpStatusSuccess, common.TopUpStatusDisputed, -500000},
	} {
		t.Run(test.eventType, func(t *testing.T) {
			db, user := setupStripeWebhookRetry(t)
			order := model.TopUp{UserId: user.Id, TradeNo: "retry-order", PaymentIntent: "pi_retry", Amount: 1, Money: 1,
				PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: test.initial}
			require.NoError(t, db.Create(&order).Error)
			object := map[string]any{"client_reference_id": order.TradeNo, "payment_intent": order.PaymentIntent,
				"customer": "cus_retry", "status": "complete", "payment_status": "paid", "currency": "usd",
				"amount_total": 100, "amount": 100, "amount_refunded": 100}
			if test.eventType == "checkout.session.expired" {
				object["status"] = "expired"
			}
			const callback = "test:stripe_persistence_failure"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "top_ups" {
					tx.AddError(errors.New("database temporarily unavailable"))
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
			assert.Equal(t, http.StatusServiceUnavailable, deliverStripeRetryEvent(t, test.eventType, object))
			var failedOrder model.TopUp
			var unchanged model.User
			require.NoError(t, db.First(&failedOrder, order.Id).Error)
			require.NoError(t, db.First(&unchanged, user.Id).Error)
			assert.Equal(t, test.initial, failedOrder.Status)
			assert.Equal(t, user.Quota, unchanged.Quota)

			require.NoError(t, db.Callback().Update().Remove(callback))
			assert.Equal(t, http.StatusOK, deliverStripeRetryEvent(t, test.eventType, object))
			assert.Equal(t, http.StatusOK, deliverStripeRetryEvent(t, test.eventType, object), "an acknowledged replay must never charge or credit twice")
			var persisted model.TopUp
			var settled model.User
			require.NoError(t, db.First(&persisted, order.Id).Error)
			require.NoError(t, db.First(&settled, user.Id).Error)
			assert.Equal(t, test.terminal, persisted.Status)
			assert.Equal(t, user.Quota+test.quotaChange, settled.Quota)
		})
	}
}

func TestStripeWebhookDoesNotAcknowledgeOrderLookupFailures(t *testing.T) {
	for _, eventType := range []string{"checkout.session.completed", "checkout.session.expired", "checkout.session.async_payment_failed", "charge.refunded"} {
		t.Run(eventType, func(t *testing.T) {
			db, _ := setupStripeWebhookRetry(t)
			const callback = "test:stripe_lookup_failure"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				tx.AddError(errors.New("database unavailable during order lookup"))
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
			status := "complete"
			if eventType == "checkout.session.expired" {
				status = "expired"
			}
			assert.Equal(t, http.StatusServiceUnavailable, deliverStripeRetryEvent(t, eventType, map[string]any{
				"client_reference_id": "pending-order", "payment_intent": "pi_pending", "status": status,
				"payment_status": "paid", "amount": 100, "amount_refunded": 100,
			}))
		})
	}
}

func TestStripeFailedEventCannotOverwriteConcurrentPaidOrder(t *testing.T) {
	db, user := setupStripeWebhookRetry(t)
	order := model.TopUp{UserId: user.Id, TradeNo: "concurrent-paid", Amount: 1, Money: 1,
		PaymentProvider: model.PaymentProviderStripe, PaymentMethod: model.PaymentMethodStripe, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&order).Error)
	const callback = "test:concurrent_paid_order"
	completed := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "top_ups" || completed || tx.Error != nil {
			return
		}
		completed = true
		// Another application's paid callback commits after the failed-event
		// handler's initial read, before it attempts to mark that order failed.
		require.NoError(t, db.Model(&model.TopUp{}).Where("id = ?", order.Id).Updates(map[string]any{
			"status": common.TopUpStatusSuccess, "payment_intent": "pi_concurrent", "complete_time": int64(12345),
		}).Error)
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", user.Quota+500000).Error)
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
	assert.Equal(t, http.StatusOK, deliverStripeRetryEvent(t, "checkout.session.async_payment_failed", map[string]any{
		"client_reference_id": order.TradeNo,
	}))
	var persisted model.TopUp
	var settled model.User
	require.NoError(t, db.First(&persisted, order.Id).Error)
	require.NoError(t, db.First(&settled, user.Id).Error)
	assert.True(t, completed)
	assert.Equal(t, common.TopUpStatusSuccess, persisted.Status)
	assert.Equal(t, "pi_concurrent", persisted.PaymentIntent)
	assert.Equal(t, int64(12345), persisted.CompleteTime)
	assert.Equal(t, user.Quota+500000, settled.Quota)
}

func TestWaffoPancakeOrderLookupKeepsDatabaseFailuresRetryable(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		name := "topup"
		resolve := service.ResolveWaffoPancakeTradeNo
		if subscription {
			name, resolve = "subscription", service.ResolveWaffoPancakeSubscriptionTradeNo
		}
		t.Run(name, func(t *testing.T) {
			db, user := setupStripeWebhookRetry(t)
			const tradeNo = "pancake-lookup"
			if subscription {
				require.NoError(t, db.Create(&model.SubscriptionOrder{TradeNo: tradeNo, UserId: user.Id, PaymentProvider: model.PaymentProviderWaffoPancake}).Error)
			} else {
				require.NoError(t, db.Create(&model.TopUp{TradeNo: tradeNo, UserId: user.Id, PaymentProvider: model.PaymentProviderWaffoPancake}).Error)
			}
			event := &service.WaffoPancakeWebhookEvent{Data: service.WaffoPancakeWebhookData{
				OrderMerchantExternalID: tradeNo, MerchantProvidedBuyerIdentity: service.WaffoPancakeBuyerIdentityFromUserID(user.Id),
			}}
			const callback = "test:pancake_lookup_outage"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				tx.AddError(errors.New("database temporarily unavailable"))
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
			_, err := resolve(event)
			require.ErrorIs(t, err, service.ErrWaffoPancakeOrderLookup)
			require.NoError(t, db.Callback().Query().Remove(callback))
			resolved, err := resolve(event)
			require.NoError(t, err)
			assert.Equal(t, tradeNo, resolved)
			event.Data.MerchantProvidedBuyerIdentity = "wrong-buyer"
			_, err = resolve(event)
			require.Error(t, err)
			assert.NotErrorIs(t, err, service.ErrWaffoPancakeOrderLookup, "a proven identity mismatch stays a permanent rejection")
		})
	}
}
