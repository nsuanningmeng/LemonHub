package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpRequest(t *testing.T) (*gorm.DB, model.User) {
	t.Helper()
	oldDB, oldRedis := model.DB, common.RedisEnabled
	oldDatabaseType := common.MainDatabaseType()
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldPrice, oldMinTopup := operation_setting.Price, operation_setting.MinTopUp
	oldDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	oldGroupRatios := common.TopupGroupRatio2JSONString()
	// GetUserGroup uses the dialect column names initialized during normal startup.
	initModelListColumnNames(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = oldDB, oldRedis
		common.SetMainDatabaseType(oldDatabaseType)
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		operation_setting.Price, operation_setting.MinTopUp = oldPrice, oldMinTopup
		operation_setting.GetPaymentSetting().AmountDiscount = oldDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, sqlDB.Close())
	})
	model.DB, common.RedisEnabled = db, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.Price, operation_setting.MinTopUp = 1, 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Site{}, &model.SiteDomain{}))
	user := model.User{Username: "low_balance_topup", Group: "default", Quota: 500000,
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func TestRequestAmountQuotesTenForLowBalanceCNYUser(t *testing.T) {
	_, user := setupTopUpRequest(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/amount", strings.NewReader(`{"amount":10}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"success","data":"10.00"}`, recorder.Body.String())
}

func TestPaymentAmountQuotesLargeAmountsForExistingLargeWallet(t *testing.T) {
	db, user := setupTopUpRequest(t)
	setupStripeTopUpProvider(t, `{"id":"price_wallet","object":"price","active":true,"type":"one_time","billing_scheme":"per_unit","currency":"usd","unit_amount":100,"unit_amount_decimal":"100"}`)
	require.NoError(t, db.Model(&user).Update("quota", 999_994_138_685_000).Error)
	oldStripePrice, oldStripeMin := setting.StripeUnitPrice, setting.StripeMinTopUp
	oldWaffoPrice, oldWaffoMin := setting.WaffoUnitPrice, setting.WaffoMinTopUp
	oldPancakePrice, oldPancakeMin := setting.WaffoPancakeUnitPrice, setting.WaffoPancakeMinTopUp
	t.Cleanup(func() {
		setting.StripeUnitPrice, setting.StripeMinTopUp = oldStripePrice, oldStripeMin
		setting.WaffoUnitPrice, setting.WaffoMinTopUp = oldWaffoPrice, oldWaffoMin
		setting.WaffoPancakeUnitPrice, setting.WaffoPancakeMinTopUp = oldPancakePrice, oldPancakeMin
	})
	setting.StripeUnitPrice, setting.StripeMinTopUp = 1, 1
	setting.WaffoUnitPrice, setting.WaffoMinTopUp = 1, 1
	setting.WaffoPancakeUnitPrice, setting.WaffoPancakeMinTopUp = 1, 1
	providers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{"epay", RequestAmount},
		{"stripe", RequestStripeAmount},
		{"waffo", RequestWaffoAmount},
		{"waffo-pancake", RequestWaffoPancakeAmount},
	}
	for _, provider := range providers {
		for _, amount := range []int{5000, 10000, 30000} {
			t.Run(fmt.Sprintf("%s/%d", provider.name, amount), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Set("id", user.Id)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/amount", strings.NewReader(fmt.Sprintf(`{"amount":%d}`, amount)))
				ctx.Request.Header.Set("Content-Type", "application/json")

				provider.handler(ctx)

				assert.Equal(t, http.StatusOK, recorder.Code)
				assert.JSONEq(t, fmt.Sprintf(`{"message":"success","data":"%d.00"}`, amount), recorder.Body.String())
			})
		}
	}
}

func TestRequestEpayCreatesPendingOrdersForSmallAndLargeWallets(t *testing.T) {
	db, user := setupTopUpRequest(t)
	oldAddress, oldID, oldKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	oldMethods := operation_setting.PayMethods
	oldServer, oldCallback := system_setting.ServerAddress, operation_setting.CustomCallbackAddress
	oldFetch := *system_setting.GetFetchSetting()
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = oldAddress, oldID, oldKey
		operation_setting.PayMethods = oldMethods
		system_setting.ServerAddress, operation_setting.CustomCallbackAddress = oldServer, oldCallback
		*system_setting.GetFetchSetting() = oldFetch
	})
	var gatewayCalls atomic.Int32
	gateway := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		gatewayCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(gateway.Close)
	gatewayURL, err := url.Parse(gateway.URL)
	require.NoError(t, err)
	fetch := system_setting.GetFetchSetting()
	fetch.AllowPrivateIp, fetch.DomainFilterMode, fetch.IpFilterMode = true, false, false
	fetch.DomainList, fetch.IpList, fetch.AllowedPorts = nil, nil, []string{gatewayURL.Port()}
	operation_setting.PayAddress = gateway.URL
	operation_setting.EpayId, operation_setting.EpayKey = "test-merchant", "test-merchant-key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Alipay"}}
	system_setting.ServerAddress, operation_setting.CustomCallbackAddress = "https://wallet.example.invalid", ""
	for _, test := range []struct {
		name   string
		quota  int
		amount int
	}{
		{"small wallet", 500000, 10},
		{"5000 topup", 50000, 5000},
		{"10000 topup", 50000, 10000},
		{"existing large wallet", 999_994_138_685_000, 30000},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, db.Model(&user).Update("quota", test.quota).Error)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", user.Id)
			ctx.Request = httptest.NewRequest(http.MethodPost, system_setting.ServerAddress+"/api/user/pay", strings.NewReader(fmt.Sprintf(`{"amount":%d,"payment_method":"alipay"}`, test.amount)))
			ctx.Request.Header.Set("Content-Type", "application/json")

			RequestEpay(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Message string            `json:"message"`
				Data    map[string]string `json:"data"`
				URL     string            `json:"url"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "success", response.Message)
			assert.Equal(t, gateway.URL+"/submit.php", response.URL)
			assert.Equal(t, fmt.Sprintf("%d.00", test.amount), response.Data["money"])
			assert.Equal(t, "alipay", response.Data["type"])
			assert.Equal(t, operation_setting.EpayId, response.Data["pid"])
			signature := response.Data["sign"]
			require.NotEmpty(t, signature)
			assert.Equal(t, epay.GenerateParams(response.Data, operation_setting.EpayKey)["sign"], signature)
			assert.Zero(t, gatewayCalls.Load(), "creating a payment form must not contact the provider")
			var order model.TopUp
			require.NoError(t, db.Where("trade_no = ?", response.Data["out_trade_no"]).First(&order).Error)
			assert.Equal(t, user.Id, order.UserId)
			assert.Equal(t, int64(test.amount), order.Amount)
			assert.Equal(t, float64(test.amount), order.Money)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			assert.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
			var unchangedUser model.User
			require.NoError(t, db.First(&unchangedUser, user.Id).Error)
			assert.Equal(t, test.quota, unchangedUser.Quota, "creating an unpaid order must not change wallet balance")
		})
	}
}

func TestRequestStripeCreatesLargePendingOrder(t *testing.T) {
	db, user := setupTopUpRequest(t)
	requests := setupStripeTopUpProvider(t, `{"id":"price_wallet","object":"price","active":true,"type":"one_time","billing_scheme":"per_unit","currency":"usd","unit_amount":100,"unit_amount_decimal":"100"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/pay", strings.NewReader(`{"amount":30000,"payment_method":"stripe"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestStripePay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"message":"success","data":{"pay_link":"https://checkout.example.invalid/large"}}`, recorder.Body.String())
	submitted := <-requests
	assert.Equal(t, "30000", submitted.Get("line_items[0][quantity]"))
	assert.Equal(t, "price_wallet", submitted.Get("line_items[0][price]"))
	assert.Equal(t, "usd", submitted.Get("currency"))
	var order model.TopUp
	require.NoError(t, db.Where("trade_no = ?", submitted.Get("client_reference_id")).First(&order).Error)
	assert.Equal(t, int64(30000), order.Amount)
	assert.Equal(t, float64(30000), order.Money)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, model.PaymentProviderStripe, order.PaymentProvider)
	var unchangedUser model.User
	require.NoError(t, db.First(&unchangedUser, user.Id).Error)
	assert.Equal(t, user.Quota, unchangedUser.Quota)
}

func setupStripeTopUpProvider(t *testing.T, priceJSON string) <-chan url.Values {
	t.Helper()
	oldSecret, oldWebhook, oldPrice := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	oldMin, oldBackend := setting.StripeMinTopUp, stripeCheckoutBackend
	oldUnitPrice := setting.StripeUnitPrice
	payment := operation_setting.GetPaymentSetting()
	oldPayment := *payment
	t.Cleanup(func() {
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = oldSecret, oldWebhook, oldPrice
		setting.StripeMinTopUp, stripeCheckoutBackend = oldMin, oldBackend
		setting.StripeUnitPrice = oldUnitPrice
		*payment = oldPayment
	})
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	setting.StripeApiSecret, setting.StripeWebhookSecret = "sk_test_wallet", "whsec_test_wallet"
	setting.StripePriceId, setting.StripeMinTopUp = "price_wallet", 1
	setting.StripeUnitPrice = 1
	requests := make(chan url.Values, 10)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/prices/price_wallet" {
			_, _ = w.Write([]byte(priceJSON))
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- r.PostForm
		_, _ = w.Write([]byte(`{"id":"cs_large_topup","object":"checkout.session","url":"https://checkout.example.invalid/large"}`))
	}))
	t.Cleanup(provider.Close)
	stripeCheckoutBackend = newStripeCheckoutBackend(provider.Client(), provider.URL)
	return requests
}
