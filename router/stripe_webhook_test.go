package router

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
)

func setupStripeWebhookRouter(t *testing.T) (*gin.Engine, *redis.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousRedisEnabled, previousRedisClient := common.RedisEnabled, common.RDB
	previousGlobalEnabled := common.GlobalApiRateLimitEnable
	previousGlobalNum, previousGlobalDuration := common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	previousWebhookNum, previousWebhookDuration := common.PaymentWebhookRateLimitNum, common.PaymentWebhookRateLimitDuration
	previousAPI, previousSecret, previousPrice := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := *paymentSetting
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRedisClient
		common.GlobalApiRateLimitEnable = previousGlobalEnabled
		common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = previousGlobalNum, previousGlobalDuration
		common.PaymentWebhookRateLimitNum, common.PaymentWebhookRateLimitDuration = previousWebhookNum, previousWebhookDuration
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = previousAPI, previousSecret, previousPrice
		*paymentSetting = previousCompliance
	})

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.Ping(context.Background()).Err())
	common.RedisEnabled, common.RDB = true, client
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = 1, 60
	common.PaymentWebhookRateLimitNum, common.PaymentWebhookRateLimitDuration = 2, 60
	setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = "sk_test_router", "whsec_test_router", "price_test_router"
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	SetApiRouter(engine)
	return engine, client
}

func performStripeWebhookRouteRequest(engine *gin.Engine, validSignature bool) *httptest.ResponseRecorder {
	// An unpaid Checkout completion is acknowledged while awaiting its asynchronous
	// result, so this exercises the actual handler without a database fixture.
	payload := []byte(`{"id":"evt_router","object":"event","type":"checkout.session.completed","data":{"object":{"id":"cs_router","object":"checkout.session","status":"complete","payment_status":"unpaid"}}}`)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: setting.StripeWebhookSecret})
	request := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Stripe-Signature", signed.Header)
	if !validSignature {
		request.Header.Set("Stripe-Signature", "invalid-signature")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func TestStripeWebhookUsesSeparateRateLimitAndPreservesSignatureVerification(t *testing.T) {
	engine, _ := setupStripeWebhookRouter(t)
	for _, expectedStatus := range []int{http.StatusOK, http.StatusTooManyRequests} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/notice", nil))
		require.Equal(t, expectedStatus, response.Code)
	}

	assert.Equal(t, http.StatusOK, performStripeWebhookRouteRequest(engine, true).Code,
		"exhausting the ordinary API budget must not block a signed Stripe event")
	assert.Equal(t, http.StatusBadRequest, performStripeWebhookRouteRequest(engine, false).Code,
		"the independent webhook route must still verify Stripe signatures")
	assert.Equal(t, http.StatusTooManyRequests, performStripeWebhookRouteRequest(engine, true).Code,
		"the payment webhook backstop must still enforce its own budget")
}

func TestStripeWebhookRemainsAvailableWhenRedisRateLimitFails(t *testing.T) {
	engine, client := setupStripeWebhookRouter(t)
	// A closed client produces a deterministic Redis failure without network waits.
	require.NoError(t, client.Close())
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/notice", nil))
	require.Equal(t, http.StatusInternalServerError, response.Code)

	assert.Equal(t, http.StatusOK, performStripeWebhookRouteRequest(engine, true).Code,
		"a Redis outage must not prevent the handler from receiving signed events")
	assert.Equal(t, http.StatusBadRequest, performStripeWebhookRouteRequest(engine, false).Code,
		"failing open for rate limiting must not bypass signature verification")
}
