package controller

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The SDK already supports these environment overrides. Tests use a temporary
// public key while the production wrapper still executes the real signature,
// timestamp and typed-payload verification with its unchanged nil options.
func pancakeWebhookSigningKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	publicKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	t.Setenv("WAFFO_WEBHOOK_PROD_PUBLIC_KEY", publicKey)
	t.Setenv("WAFFO_WEBHOOK_TEST_PUBLIC_KEY", publicKey)
	t.Setenv("WAFFO_WEBHOOK_PUBLIC_KEY", "")
	return key
}

func signedPancakeWebhook(t *testing.T, key *rsa.PrivateKey, storeID, tradeNo string, userID int, timestamp time.Time) (string, string) {
	t.Helper()
	body, err := common.Marshal(map[string]any{
		"id": "pancake-store-event", "eventType": "order.completed", "storeId": storeID, "mode": "prod",
		"data": map[string]any{
			"orderId": "ORD_store_test", "orderMerchantExternalId": tradeNo,
			"merchantProvidedBuyerIdentity": service.WaffoPancakeBuyerIdentityFromUserID(userID),
			"amount":                        "3.00", "currency": "USD", "taxAmount": "0",
		},
	})
	require.NoError(t, err)
	ts := strconv.FormatInt(timestamp.UnixMilli(), 10)
	digest := sha256.Sum256([]byte(ts + "." + string(body)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	require.NoError(t, err)
	return string(body), "t=" + ts + ",v1=" + base64.StdEncoding.EncodeToString(signature)
}

func TestConfiguredPancakeWebhookRequiresSignedMatchingStore(t *testing.T) {
	key := pancakeWebhookSigningKey(t)
	previousStore := setting.WaffoPancakeStoreID
	t.Cleanup(func() { setting.WaffoPancakeStoreID = previousStore })
	for _, test := range []struct {
		name       string
		configured string
		eventStore string
		wantError  bool
	}{
		{name: "matching store", configured: "STO_ours", eventStore: "STO_ours"},
		{name: "trim configured store", configured: " STO_ours ", eventStore: "STO_ours"},
		{name: "empty configuration", eventStore: "STO_ours", wantError: true},
		{name: "blank configuration", configured: " \t ", eventStore: "STO_ours", wantError: true},
		{name: "other store", configured: "STO_ours", eventStore: "STO_other", wantError: true},
		{name: "missing event store", configured: "STO_ours", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			setting.WaffoPancakeStoreID = test.configured
			body, signature := signedPancakeWebhook(t, key, test.eventStore, "wallet-order", 7, time.Now())
			event, err := service.VerifyConfiguredWaffoPancakeWebhook(body, signature)
			if test.wantError {
				assert.EqualError(t, err, "Waffo Pancake webhook store mismatch")
				assert.Nil(t, event)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, event)
			assert.Equal(t, test.eventStore, event.StoreID)
			assert.Equal(t, "wallet-order", event.Data.OrderMerchantExternalID)
			assert.Equal(t, "new-api-user-7", event.Data.MerchantProvidedBuyerIdentity)
		})
	}

	setting.WaffoPancakeStoreID = "STO_ours"
	for _, test := range []struct {
		name   string
		stale  bool
		tamper bool
	}{
		{name: "signed store cannot be changed", tamper: true},
		{name: "expired signature cannot be replayed", stale: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			timestamp := time.Now()
			if test.stale {
				timestamp = timestamp.Add(-10 * time.Minute)
			}
			body, signature := signedPancakeWebhook(t, key, "STO_other", "wallet-order", 7, timestamp)
			if test.tamper {
				body = strings.Replace(body, "STO_other", "STO_ours", 1)
			}
			event, err := service.VerifyConfiguredWaffoPancakeWebhook(body, signature)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "store mismatch", "SDK verification must reject before store validation")
			assert.Nil(t, event)
		})
	}
}

func TestPancakeCheckoutRejectsMissingConfiguredStore(t *testing.T) {
	previousStore, previousMerchant := setting.WaffoPancakeStoreID, setting.WaffoPancakeMerchantID
	t.Cleanup(func() {
		setting.WaffoPancakeStoreID, setting.WaffoPancakeMerchantID = previousStore, previousMerchant
	})
	// Invalid credentials guarantee that even the old code cannot perform I/O.
	// Store validation must fail before constructing that SDK client.
	setting.WaffoPancakeMerchantID = ""
	for _, storeID := range []string{"", " \t "} {
		setting.WaffoPancakeStoreID = storeID
		session, err := service.CreateWaffoPancakeCheckoutSession(context.Background(), &service.WaffoPancakeCreateSessionParams{
			BuyerIdentity: "new-api-user-7", OrderMerchantExternalID: "wallet-order", ProductID: "PRO_ours",
		})
		assert.EqualError(t, err, "missing Waffo Pancake store id")
		assert.Nil(t, session)
	}
}

func TestPancakeWebhookStoreBindingProtectsWalletAndSubscription(t *testing.T) {
	key := pancakeWebhookSigningKey(t)
	previousStore, previousMerchant := setting.WaffoPancakeStoreID, setting.WaffoPancakeMerchantID
	previousKey, previousProduct := setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeStoreID, setting.WaffoPancakeMerchantID = previousStore, previousMerchant
		setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = previousKey, previousProduct
	})
	setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey = "merchant", "configured-private-key"
	setting.WaffoPancakeProductID = "PRO_ours"
	confirmPaymentComplianceForTest(t)
	for _, orderType := range []string{"wallet", "subscription"} {
		for _, configuredStore := range []string{"", "STO_other", "STO_ours"} {
			t.Run(fmt.Sprintf("%s/store=%s", orderType, configuredStore), func(t *testing.T) {
				db := setupManageUserTestDB(t)
				require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionPlan{}, &model.SubscriptionOrder{}, &model.UserSubscription{}))
				user := model.User{Username: "store-buyer", Quota: 40, Group: "default", Role: common.RoleCommonUser}
				require.NoError(t, db.Create(&user).Error)
				plan := model.SubscriptionPlan{Title: "Store-bound plan", PriceAmount: 3, Currency: "USD", Enabled: true,
					DurationUnit: model.SubscriptionDurationDay, DurationValue: 1, TotalAmount: 100}
				require.NoError(t, db.Create(&plan).Error)
				topUp := model.TopUp{UserId: user.Id, Amount: 3, Money: 3, TradeNo: "WAFFO_PANCAKE-wallet",
					PaymentProvider: model.PaymentProviderWaffoPancake, PaymentMethod: model.PaymentMethodWaffoPancake,
					Status: common.TopUpStatusPending}
				require.NoError(t, db.Create(&topUp).Error)
				order := model.SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, TradeNo: "WAFFO_PANCAKE_SUB-plan",
					PaymentProvider: model.PaymentProviderWaffoPancake, PaymentMethod: model.PaymentMethodWaffoPancake,
					Status: common.TopUpStatusPending}
				require.NoError(t, order.InsertWithPlanSnapshot(&plan))
				tradeNo := topUp.TradeNo
				if orderType == "subscription" {
					tradeNo = order.TradeNo
				}
				setting.WaffoPancakeStoreID = configuredStore
				body, signature := signedPancakeWebhook(t, key, "STO_ours", tradeNo, user.Id, time.Now())
				router := gin.New()
				router.POST("/webhook/:env", WaffoPancakeWebhook)
				request := httptest.NewRequest(http.MethodPost, "/webhook/prod", strings.NewReader(body))
				request.Header.Set("X-Waffo-Signature", signature)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)

				wantQuota, wantSubscriptions := 40, int64(0)
				wantTopUpStatus, wantOrderStatus := common.TopUpStatusPending, common.TopUpStatusPending
				if configuredStore == "STO_ours" {
					assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
					if orderType == "wallet" {
						wantQuota += int(3 * common.QuotaPerUnit)
						wantTopUpStatus = common.TopUpStatusSuccess
					} else {
						wantOrderStatus, wantSubscriptions = common.TopUpStatusSuccess, 1
					}
				} else {
					assert.Equal(t, http.StatusUnauthorized, response.Code)
				}
				var storedUser model.User
				require.NoError(t, db.First(&storedUser, user.Id).Error)
				assert.Equal(t, wantQuota, storedUser.Quota)
				require.NoError(t, db.First(&topUp, topUp.Id).Error)
				assert.Equal(t, wantTopUpStatus, topUp.Status)
				require.NoError(t, db.First(&order, order.Id).Error)
				assert.Equal(t, wantOrderStatus, order.Status)
				var subscriptions int64
				require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", user.Id).Count(&subscriptions).Error)
				assert.Equal(t, wantSubscriptions, subscriptions)
			})
		}
	}
}
