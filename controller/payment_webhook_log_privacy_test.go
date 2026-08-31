package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	waffoutils "github.com/waffo-com/waffo-go/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const paymentWebhookQuerySecret = "payment-webhook-query-secret-marker"

func capturePaymentWebhookLogs(run func()) string {
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultWriter
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = &logs
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	defer func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previousWriter
		gin.DefaultErrorWriter = previousErrorWriter
		common.LogWriterMu.Unlock()
	}()

	run()
	return logs.String()
}

func captureProcessStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	previousStderr := os.Stderr
	os.Stderr = writer
	func() {
		defer func() {
			os.Stderr = previousStderr
			_ = writer.Close()
		}()
		run()
	}()
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return string(output)
}

func newPaymentWebhookTestContext(path string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, path+"?token="+paymentWebhookQuerySecret, strings.NewReader(body))
	return ctx, recorder
}

func assertPaymentWebhookLogSecretsAbsent(t *testing.T, logs string, secrets ...string) {
	t.Helper()
	assert.NotContains(t, logs, paymentWebhookQuerySecret)
	for _, secret := range secrets {
		assert.NotContains(t, logs, secret)
	}
}

func TestEpayWebhookDoesNotLogQueryOrUnverifiedParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/user/epay/notify?token="+paymentWebhookQuerySecret+
			"&sign=private-epay-signature&customer_email=private-epay@example.test",
		nil,
	)

	logs := capturePaymentWebhookLogs(func() { EpayNotify(ctx) })

	assert.Equal(t, "fail", recorder.Body.String())
	assert.Contains(t, logs, `path="/api/user/epay/notify"`)
	assert.Contains(t, logs, "param_count=3")
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		"private-epay-signature", "private-epay@example.test", "customer_email", "sign=")

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	malformedForm := "sign=%ZZ&note=private-malformed-epay"
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/epay/notify?token="+paymentWebhookQuerySecret,
		strings.NewReader(malformedForm),
	)
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	logs = capturePaymentWebhookLogs(func() { EpayNotify(ctx) })

	assert.Equal(t, "fail", recorder.Body.String())
	assert.Contains(t, logs, "reason=form_parse_failed")
	assertPaymentWebhookLogSecretsAbsent(t, logs, malformedForm, "private-malformed-epay", "%ZZ")
}

func TestStripeWebhookInvalidSignatureDoesNotLogPayloadSignatureOrQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})
	setting.StripeApiSecret = "sk_test_private"
	setting.StripeWebhookSecret = "whsec_private"
	setting.StripePriceId = "price_test"

	payload := `{"customer_email":"private-stripe@example.test","secret":"private-stripe-body"}`
	signature := "private-stripe-signature"
	ctx, recorder := newPaymentWebhookTestContext("/api/user/stripe/webhook", payload)
	ctx.Request.Header.Set("Stripe-Signature", signature)

	logs := capturePaymentWebhookLogs(func() { StripeWebhook(ctx) })

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, logs, "reason=invalid_signature")
	assert.Contains(t, logs, "body_bytes="+strconv.Itoa(len(payload)))
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		payload, signature, "private-stripe@example.test", "private-stripe-body")
}

func TestStripeCheckoutFailuresDoNotLogProviderErrorOrCustomerPII(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stripe-checkout-log-privacy.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.SubscriptionPlan{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	paymentSetting := operation_setting.GetPaymentSetting()
	previousPaymentSetting := *paymentSetting
	previousAPISecret := setting.StripeApiSecret
	previousWebhookSecret := setting.StripeWebhookSecret
	previousPriceID := setting.StripePriceId
	previousMinTopUp := setting.StripeMinTopUp
	previousBackend := stripeCheckoutBackend
	t.Cleanup(func() {
		stripeCheckoutBackend = previousBackend
		setting.StripeApiSecret = previousAPISecret
		setting.StripeWebhookSecret = previousWebhookSecret
		setting.StripePriceId = previousPriceID
		setting.StripeMinTopUp = previousMinTopUp
		*paymentSetting = previousPaymentSetting
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	setting.StripeApiSecret = "sk_test_private"
	setting.StripeWebhookSecret = "whsec_private"
	setting.StripePriceId = "price_wallet"
	setting.StripeMinTopUp = 1

	const customerEmail = "private-stripe-checkout@example.test"
	const providerError = "private-stripe-provider-error customer=private-stripe-checkout@example.test"
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"`+providerError+`","param":"customer_email"}}`)
	}))
	t.Cleanup(providerServer.Close)
	stripeCheckoutBackend = newStripeCheckoutBackend(providerServer.Client(), providerServer.URL)

	user := &model.User{Id: 701, Username: "stripe-privacy", Email: customerEmail, Group: "default", Quota: 0}
	require.NoError(t, db.Create(user).Error)
	plan := &model.SubscriptionPlan{
		Id:               702,
		Title:            "Stripe privacy plan",
		PriceAmount:      10,
		Currency:         "USD",
		DurationUnit:     "month",
		DurationValue:    1,
		Enabled:          true,
		StripePriceId:    "price_subscription",
		TotalAmount:      100,
		QuotaResetPeriod: "never",
	}
	require.NoError(t, db.Create(plan).Error)

	var sdkStderr string
	logs := capturePaymentWebhookLogs(func() {
		sdkStderr = captureProcessStderr(t, func() {
			walletRecorder := httptest.NewRecorder()
			walletContext, _ := gin.CreateTestContext(walletRecorder)
			walletContext.Request = httptest.NewRequest(http.MethodPost, "/api/user/pay", nil)
			walletContext.Set("id", user.Id)
			stripeAdaptor.RequestPay(walletContext, &StripePayRequest{Amount: 10, PaymentMethod: model.PaymentMethodStripe})
			assert.Equal(t, http.StatusOK, walletRecorder.Code)

			subscriptionRecorder := httptest.NewRecorder()
			subscriptionContext, _ := gin.CreateTestContext(subscriptionRecorder)
			subscriptionContext.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/subscription/stripe/pay",
				strings.NewReader(`{"plan_id":702}`),
			)
			subscriptionContext.Request.Header.Set("Content-Type", "application/json")
			subscriptionContext.Set("id", user.Id)
			SubscriptionRequestStripePay(subscriptionContext)
			assert.Equal(t, http.StatusOK, subscriptionRecorder.Code)
		})
	})

	assert.GreaterOrEqual(t, strings.Count(logs, "reason=sdk_error"), 2)
	assert.NotContains(t, logs, providerError)
	assert.NotContains(t, logs, customerEmail)
	assert.NotContains(t, sdkStderr, providerError)
	assert.NotContains(t, sdkStderr, customerEmail)
}

func TestCreemWebhookInvalidSignatureDoesNotLogPayloadOrSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	originalTestMode := setting.CreemTestMode
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
		setting.CreemTestMode = originalTestMode
	})
	setting.CreemApiKey = "creem-api-key"
	setting.CreemProducts = `[{"productId":"product"}]`
	setting.CreemWebhookSecret = "configured-webhook-secret"
	setting.CreemTestMode = false

	payload := `{"customer":{"email":"private-creem@example.test","name":"Private Creem Name"},"secret":"private-creem-body"}`
	signature := "private-creem-signature"
	ctx, recorder := newPaymentWebhookTestContext("/api/user/creem/webhook", payload)
	ctx.Request.Header.Set(CreemSignatureHeader, signature)

	logs := capturePaymentWebhookLogs(func() { CreemWebhook(ctx) })

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, logs, "body_bytes="+strconv.Itoa(len(payload)))
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		payload, signature, "private-creem@example.test", "Private Creem Name", "private-creem-body")

	malformedPayload := `{"customer_email":"private-creem-parse@example.test"`
	validSignature := generateCreemSignature(malformedPayload, setting.CreemWebhookSecret)
	ctx, recorder = newPaymentWebhookTestContext("/api/user/creem/webhook", malformedPayload)
	ctx.Request.Header.Set(CreemSignatureHeader, validSignature)

	logs = capturePaymentWebhookLogs(func() { CreemWebhook(ctx) })

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, logs, "body_bytes="+strconv.Itoa(len(malformedPayload)))
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		malformedPayload, validSignature, "private-creem-parse@example.test")
}

func TestCreemCompletedCallbackDoesNotLogCustomerPII(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "creem-log-privacy.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.SubscriptionOrder{}, &model.TopUp{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	event := &CreemWebhookEvent{Id: "event-safe-id", EventType: "checkout.completed"}
	event.Object.RequestId = "trade-safe-id\n[ERR] forged-trade-log"
	event.Object.Order.Id = "order-safe-id\n[ERR] forged-order-log"
	event.Object.Order.Status = "paid"
	event.Object.Order.Type = "onetime"
	event.Object.Order.AmountPaid = 1234
	event.Object.Order.Currency = "USD"
	event.Object.Product.Name = "Safe product"
	event.Object.Customer.Email = "private-customer@example.test"
	event.Object.Customer.Name = "Private Customer Name"
	ctx, recorder := newPaymentWebhookTestContext("/api/user/creem/webhook", "")

	logs := capturePaymentWebhookLogs(func() { handleCheckoutCompleted(ctx, event) })

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, logs, `trade_no="trade-safe-id\n[ERR] forged-trade-log"`)
	assert.Contains(t, logs, `creem_order_id="order-safe-id\n[ERR] forged-order-log"`)
	assert.NotContains(t, logs, "\n[ERR] forged-trade-log")
	assert.NotContains(t, logs, "\n[ERR] forged-order-log")
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		"private-customer@example.test", "Private Customer Name", "customer_email", "customer_name")
}

func TestWaffoWebhookInvalidSignatureDoesNotLogPayloadOrSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	keyPair, err := waffoutils.GenerateKeyPair()
	require.NoError(t, err)
	originalEnabled := setting.WaffoEnabled
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
	})
	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = "waffo-api-key"
	setting.WaffoPrivateKey = keyPair.PrivateKey
	setting.WaffoPublicCert = keyPair.PublicKey

	payload := `{"buyer_email":"private-waffo@example.test","secret":"private-waffo-body"}`
	signature := "private-waffo-signature"
	ctx, recorder := newPaymentWebhookTestContext("/api/user/waffo/webhook", payload)
	ctx.Request.Header.Set("X-SIGNATURE", signature)

	logs := capturePaymentWebhookLogs(func() { WaffoWebhook(ctx) })

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, logs, "body_bytes="+strconv.Itoa(len(payload)))
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		payload, signature, "private-waffo@example.test", "private-waffo-body")

	malformedPayload := `{"buyer_email":"private-waffo-parse@example.test"`
	validSignature, err := waffoutils.Sign(malformedPayload, keyPair.PrivateKey)
	require.NoError(t, err)
	ctx, recorder = newPaymentWebhookTestContext("/api/user/waffo/webhook", malformedPayload)
	ctx.Request.Header.Set("X-SIGNATURE", validSignature)

	logs = capturePaymentWebhookLogs(func() { WaffoWebhook(ctx) })

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, logs, "body_bytes="+strconv.Itoa(len(malformedPayload)))
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		malformedPayload, validSignature, "private-waffo-parse@example.test")
}

func TestWaffoPancakeWebhookInvalidSignatureDoesNotLogPayloadOrSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
	})
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "private-key"
	setting.WaffoPancakeProductID = "product"

	payload := `{"mode":"test","data":{"buyerEmail":"private-pancake@example.test","merchantProvidedBuyerIdentity":"private-pancake-identity"},"secret":"private-pancake-body"}`
	signature := "private-pancake-signature"
	ctx, recorder := newPaymentWebhookTestContext("/api/user/waffo-pancake/webhook/test", payload)
	ctx.Params = gin.Params{{Key: "env", Value: "test"}}
	ctx.Request.Header.Set("X-Waffo-Signature", signature)

	logs := capturePaymentWebhookLogs(func() { WaffoPancakeWebhook(ctx) })

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, logs, "body_bytes="+strconv.Itoa(len(payload)))
	assertPaymentWebhookLogSecretsAbsent(t, logs,
		payload, signature, "private-pancake@example.test", "private-pancake-identity", "private-pancake-body")
}
