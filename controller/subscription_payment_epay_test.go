package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	subscriptionEpayTestMerchantID  = "subscription-merchant-1001"
	subscriptionEpayTestMerchantKey = "subscription-merchant-test-key"
	subscriptionEpayTestMoney       = "19.95"
)

type subscriptionEpayGatewayCall struct {
	method string
	path   string
	query  url.Values
}

type subscriptionEpayGatewayRecorder struct {
	mu        sync.Mutex
	calls     []subscriptionEpayGatewayCall
	responder func(url.Values) (int, string)
}

func (g *subscriptionEpayGatewayRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := make(url.Values, len(r.URL.Query()))
	for key, values := range r.URL.Query() {
		query[key] = append([]string(nil), values...)
	}

	g.mu.Lock()
	g.calls = append(g.calls, subscriptionEpayGatewayCall{
		method: r.Method,
		path:   r.URL.Path,
		query:  query,
	})
	g.mu.Unlock()

	status := http.StatusOK
	body := ""
	if g.responder != nil {
		status, body = g.responder(query)
	} else {
		tradeNo := query.Get("out_trade_no")
		body = fmt.Sprintf(
			`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"type":"alipay","money":%q}`,
			subscriptionEpayTestMerchantID,
			"GW"+tradeNo,
			tradeNo,
			subscriptionEpayTestMoney,
		)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (g *subscriptionEpayGatewayRecorder) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.calls)
}

func (g *subscriptionEpayGatewayRecorder) lastCall(t *testing.T) subscriptionEpayGatewayCall {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	require.NotEmpty(t, g.calls)
	return g.calls[len(g.calls)-1]
}

type subscriptionEpayFixture struct {
	db      *gorm.DB
	user    *model.User
	plan    *model.SubscriptionPlan
	gateway *subscriptionEpayGatewayRecorder
	server  *httptest.Server
}

func setupSubscriptionEpayFixture(
	t *testing.T,
	responder func(url.Values) (int, string),
) *subscriptionEpayFixture {
	t.Helper()

	oldGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldGinMode) })

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainDBType, oldLogDBType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsnName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:subscription_epay_"+dsnName+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainDBType, oldLogDBType)
		common.RedisEnabled = oldRedisEnabled
	})

	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.SubscriptionPlan{},
		&model.SubscriptionOrder{},
		&model.UserSubscription{},
		&model.TopUp{},
		&model.Log{},
		&model.Site{},
		&model.SiteDomain{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	require.NoError(t, model.ReloadSiteCache())

	oldPayAddress := operation_setting.PayAddress
	oldEpayID := operation_setting.EpayId
	oldEpayKey := operation_setting.EpayKey
	oldCallbackAddress := operation_setting.CustomCallbackAddress
	oldPayMethods := operation_setting.PayMethods
	oldServerAddress := system_setting.ServerAddress
	oldPaymentSetting := *operation_setting.GetPaymentSetting()
	fetchSetting := system_setting.GetFetchSetting()
	oldFetchSetting := *fetchSetting

	gateway := &subscriptionEpayGatewayRecorder{responder: responder}
	server := newEpayQueryTLSServer(t, gateway)
	operation_setting.PayAddress = server.URL
	operation_setting.EpayId = subscriptionEpayTestMerchantID
	operation_setting.EpayKey = subscriptionEpayTestMerchantKey
	operation_setting.CustomCallbackAddress = "https://callback.gateway.example.test/epay/"
	operation_setting.PayMethods = []map[string]string{{"name": "Alipay", "type": "alipay"}}
	system_setting.ServerAddress = "https://main.example.test"
	paymentSetting := operation_setting.GetPaymentSetting()
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	fetchSetting.EnableSSRFProtection = false
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpFilterMode = false
	fetchSetting.IpList = nil
	fetchSetting.AllowedPorts = nil
	t.Cleanup(func() {
		operation_setting.PayAddress = oldPayAddress
		operation_setting.EpayId = oldEpayID
		operation_setting.EpayKey = oldEpayKey
		operation_setting.CustomCallbackAddress = oldCallbackAddress
		operation_setting.PayMethods = oldPayMethods
		system_setting.ServerAddress = oldServerAddress
		*paymentSetting = oldPaymentSetting
		*fetchSetting = oldFetchSetting
	})

	user := &model.User{
		Username: "subscription_epay_user",
		Password: "test-password-hash",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Group:    "default",
		AffCode:  "subscriptionepayaff",
	}
	require.NoError(t, db.Create(user).Error)

	plan := &model.SubscriptionPlan{
		Title:            "Subscription Epay Test Plan",
		PriceAmount:      19.95,
		Currency:         "USD",
		DurationUnit:     model.SubscriptionDurationMonth,
		DurationValue:    1,
		Enabled:          true,
		TotalAmount:      12345,
		QuotaResetPeriod: model.SubscriptionResetNever,
	}
	require.NoError(t, db.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	_, err = model.GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err, "preload the plan so settlement does not leave the SQLite transaction for a second connection")
	t.Cleanup(func() { model.InvalidateSubscriptionPlanCache(plan.Id) })

	return &subscriptionEpayFixture{
		db:      db,
		user:    user,
		plan:    plan,
		gateway: gateway,
		server:  server,
	}
}

func (f *subscriptionEpayFixture) createPendingOrder(t *testing.T, tradeNo string) *model.SubscriptionOrder {
	t.Helper()
	order := &model.SubscriptionOrder{
		UserId:          f.user.Id,
		PlanId:          f.plan.Id,
		Money:           f.plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	require.NoError(t, order.InsertWithPlanSnapshot(f.plan))
	return order
}

func signedSubscriptionEpayValues(tradeNo string, overrides map[string]string) url.Values {
	params := map[string]string{
		"pid":          subscriptionEpayTestMerchantID,
		"trade_no":     "GW" + tradeNo,
		"out_trade_no": tradeNo,
		"type":         "alipay",
		"name":         "Subscription Epay Test Plan",
		"money":        subscriptionEpayTestMoney,
		"trade_status": epay.StatusTradeSuccess,
	}
	for key, value := range overrides {
		params[key] = value
	}
	signed := epay.GenerateParams(params, subscriptionEpayTestMerchantKey)
	values := make(url.Values, len(signed))
	for key, value := range signed {
		values.Set(key, value)
	}
	return values
}

func runSubscriptionEpayCallback(
	t *testing.T,
	endpoint string,
	method string,
	values url.Values,
	host string,
) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/subscription/epay/" + endpoint
	var req *http.Request
	if method == http.MethodPost {
		req = httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path+"?"+values.Encode(), nil)
	}
	req.Host = host
	req.Header.Set("X-Forwarded-Proto", "https")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	switch endpoint {
	case "notify":
		SubscriptionEpayNotify(ctx)
	case "return":
		SubscriptionEpayReturn(ctx)
	default:
		require.FailNow(t, "unsupported subscription epay callback endpoint", endpoint)
	}
	// gin.Engine flushes a header-only POST redirect after the handler returns.
	// Direct handler tests must do the same because net/http intentionally writes
	// no redirect body for POST, so Gin's wrapped status would otherwise stay buffered.
	ctx.Writer.WriteHeaderNow()
	return recorder
}

func assertSubscriptionEpayPendingWithoutGrant(t *testing.T, f *subscriptionEpayFixture, tradeNo string) {
	t.Helper()
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Empty(t, order.ProviderPayload)

	var subscriptions int64
	require.NoError(t, f.db.Model(&model.UserSubscription{}).
		Where("user_id = ?", f.user.Id).
		Count(&subscriptions).Error)
	assert.Zero(t, subscriptions)

	var topUps int64
	require.NoError(t, f.db.Model(&model.TopUp{}).
		Where("trade_no = ?", tradeNo).
		Count(&topUps).Error)
	assert.Zero(t, topUps)
}

func registerSubscriptionEpayAlias(
	t *testing.T,
	f *subscriptionEpayFixture,
	siteID int,
	domain string,
	payConfig string,
) {
	t.Helper()
	site := &model.Site{
		Id:           siteID,
		Name:         "Subscription Epay Alias",
		Status:       model.SiteStatusNormal,
		DiscountRate: model.DiscountRateBase,
		PayConfig:    payConfig,
	}
	require.NoError(t, f.db.Create(site).Error)
	require.NoError(t, f.db.Create(&model.SiteDomain{SiteId: siteID, Domain: domain}).Error)
	require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", f.user.Id).Update("site_id", siteID).Error)
	f.user.SiteId = siteID
	require.NoError(t, model.ReloadSiteCache())
	t.Cleanup(func() {
		assert.NoError(t, f.db.Where("site_id = ?", siteID).Delete(&model.SiteDomain{}).Error)
		assert.NoError(t, f.db.Where("id = ?", siteID).Delete(&model.Site{}).Error)
		assert.NoError(t, model.ReloadSiteCache())
	})
}

func TestSubscriptionEpayPaidCallbacksSettleOnce(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint string
		method   string
	}{
		{name: "notify GET", endpoint: "notify", method: http.MethodGet},
		{name: "notify POST", endpoint: "notify", method: http.MethodPost},
		{name: "return GET", endpoint: "return", method: http.MethodGet},
		{name: "return POST", endpoint: "return", method: http.MethodPost},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := setupSubscriptionEpayFixture(t, nil)
			const tradeNo = "SUBSCRIPTION_PAID_ONCE"
			fixture.createPendingOrder(t, tradeNo)
			callback := signedSubscriptionEpayValues(tradeNo, nil)

			first := runSubscriptionEpayCallback(t, testCase.endpoint, testCase.method, callback, "main.example.test")
			if testCase.endpoint == "notify" {
				assert.Equal(t, "success", first.Body.String())
			} else {
				require.Equal(t, http.StatusFound, first.Code)
				assert.Equal(t, "https://main.example.test/wallet?pay=success", first.Header().Get("Location"))
			}
			assert.Equal(t, 1, fixture.gateway.callCount())

			second := runSubscriptionEpayCallback(t, testCase.endpoint, testCase.method, callback, "main.example.test")
			if testCase.endpoint == "notify" {
				assert.Equal(t, "success", second.Body.String())
			} else {
				assert.Equal(t, "https://main.example.test/wallet?pay=success", second.Header().Get("Location"))
			}
			assert.Equal(t, 1, fixture.gateway.callCount(), "a completed order must not be queried again on replay")

			order := model.GetSubscriptionOrderByTradeNo(tradeNo)
			require.NotNil(t, order)
			assert.Equal(t, common.TopUpStatusSuccess, order.Status)
			assert.NotEmpty(t, order.ProviderPayload)

			var subscriptions []model.UserSubscription
			require.NoError(t, fixture.db.Where("user_id = ? AND plan_id = ?", fixture.user.Id, fixture.plan.Id).
				Find(&subscriptions).Error)
			require.Len(t, subscriptions, 1)
			assert.Equal(t, int64(12345), subscriptions[0].AmountTotal)
			assert.Equal(t, "active", subscriptions[0].Status)

			var topUps int64
			require.NoError(t, fixture.db.Model(&model.TopUp{}).Where("trade_no = ?", tradeNo).Count(&topUps).Error)
			assert.EqualValues(t, 1, topUps)

			query := fixture.gateway.lastCall(t)
			assert.Equal(t, http.MethodGet, query.method)
			assert.Equal(t, "/api.php", query.path)
			assert.Equal(t, "order", query.query.Get("act"))
			assert.Equal(t, subscriptionEpayTestMerchantID, query.query.Get("pid"))
			assert.Equal(t, subscriptionEpayTestMerchantKey, query.query.Get("key"))
			assert.Equal(t, tradeNo, query.query.Get("out_trade_no"))
		})
	}
}

func TestSubscriptionEpayRejectsInvalidCallbackBeforeQuery(t *testing.T) {
	testCases := []struct {
		name  string
		build func(string) url.Values
	}{
		{
			name: "invalid signature",
			build: func(tradeNo string) url.Values {
				values := signedSubscriptionEpayValues(tradeNo, nil)
				values.Set("sign", strings.Repeat("0", 32))
				return values
			},
		},
		{
			name: "wrong signed pid",
			build: func(tradeNo string) url.Values {
				return signedSubscriptionEpayValues(tradeNo, map[string]string{"pid": "another-merchant"})
			},
		},
		{
			name: "wrong signed amount",
			build: func(tradeNo string) url.Values {
				return signedSubscriptionEpayValues(tradeNo, map[string]string{"money": "0.01"})
			},
		},
		{
			name: "wrong signed payment type",
			build: func(tradeNo string) url.Values {
				return signedSubscriptionEpayValues(tradeNo, map[string]string{"type": "wxpay"})
			},
		},
		{
			name: "missing signed gateway trade number",
			build: func(tradeNo string) url.Values {
				return signedSubscriptionEpayValues(tradeNo, map[string]string{"trade_no": ""})
			},
		},
		{
			name: "unknown signed merchant order number",
			build: func(string) url.Values {
				return signedSubscriptionEpayValues("SUBSCRIPTION_ORDER_DOES_NOT_EXIST", nil)
			},
		},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := setupSubscriptionEpayFixture(t, nil)
			const localTradeNo = "SUBSCRIPTION_REJECTED_CALLBACK"
			fixture.createPendingOrder(t, localTradeNo)
			method := http.MethodGet
			if index%2 == 1 {
				method = http.MethodPost
			}

			result := runSubscriptionEpayCallback(t, "notify", method, testCase.build(localTradeNo), "main.example.test")
			assert.Equal(t, "fail", result.Body.String())
			assert.Zero(t, fixture.gateway.callCount(), "invalid callbacks must be rejected before an active gateway query")
			assertSubscriptionEpayPendingWithoutGrant(t, fixture, localTradeNo)
		})
	}
}

func TestSubscriptionEpayGatewayMustConfirmExactPaidOrder(t *testing.T) {
	paidResponse := func(query url.Values, overrides map[string]string) string {
		fields := map[string]string{
			"pid":          subscriptionEpayTestMerchantID,
			"trade_no":     "GW" + query.Get("out_trade_no"),
			"out_trade_no": query.Get("out_trade_no"),
			"type":         "alipay",
			"money":        subscriptionEpayTestMoney,
		}
		for key, value := range overrides {
			fields[key] = value
		}
		return fmt.Sprintf(
			`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"type":%q,"money":%q}`,
			fields["pid"],
			fields["trade_no"],
			fields["out_trade_no"],
			fields["type"],
			fields["money"],
		)
	}

	testCases := []struct {
		name      string
		endpoint  string
		method    string
		responder func(url.Values) (int, string)
	}{
		{
			name:     "gateway has no order",
			endpoint: "notify",
			method:   http.MethodGet,
			responder: func(url.Values) (int, string) {
				return http.StatusOK, `{"code":0,"msg":"order not found"}`
			},
		},
		{
			name:     "gateway order is unpaid",
			endpoint: "notify",
			method:   http.MethodPost,
			responder: func(query url.Values) (int, string) {
				body := paidResponse(query, nil)
				return http.StatusOK, strings.Replace(body, `"status":1`, `"status":0`, 1)
			},
		},
		{
			name:     "gateway amount differs",
			endpoint: "return",
			method:   http.MethodGet,
			responder: func(query url.Values) (int, string) {
				return http.StatusOK, paidResponse(query, map[string]string{"money": "19.94"})
			},
		},
		{
			name:     "gateway merchant differs",
			endpoint: "return",
			method:   http.MethodPost,
			responder: func(query url.Values) (int, string) {
				return http.StatusOK, paidResponse(query, map[string]string{"pid": "different-merchant"})
			},
		},
		{
			name:     "gateway trade number differs",
			endpoint: "notify",
			method:   http.MethodGet,
			responder: func(query url.Values) (int, string) {
				return http.StatusOK, paidResponse(query, map[string]string{"trade_no": "GW-DIFFERENT"})
			},
		},
		{
			name:     "gateway merchant order differs",
			endpoint: "notify",
			method:   http.MethodPost,
			responder: func(query url.Values) (int, string) {
				return http.StatusOK, paidResponse(query, map[string]string{"out_trade_no": "DIFFERENT-ORDER"})
			},
		},
		{
			name:     "gateway payment type differs",
			endpoint: "return",
			method:   http.MethodGet,
			responder: func(query url.Values) (int, string) {
				return http.StatusOK, paidResponse(query, map[string]string{"type": "wxpay"})
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := setupSubscriptionEpayFixture(t, testCase.responder)
			const tradeNo = "SUBSCRIPTION_QUERY_UNCONFIRMED"
			fixture.createPendingOrder(t, tradeNo)

			result := runSubscriptionEpayCallback(
				t,
				testCase.endpoint,
				testCase.method,
				signedSubscriptionEpayValues(tradeNo, nil),
				"main.example.test",
			)
			if testCase.endpoint == "notify" {
				assert.Equal(t, "fail", result.Body.String())
			} else {
				require.Equal(t, http.StatusFound, result.Code)
				assert.Equal(t, "https://main.example.test/wallet?pay=pending", result.Header().Get("Location"))
			}
			assert.Equal(t, 1, fixture.gateway.callCount())
			assertSubscriptionEpayPendingWithoutGrant(t, fixture, tradeNo)
		})
	}
}

func TestSubscriptionEpayReturnQueryFailureStaysPendingOnTrustedAlias(t *testing.T) {
	fixture := setupSubscriptionEpayFixture(t, func(url.Values) (int, string) {
		return http.StatusBadGateway, `{"error":"temporary gateway failure"}`
	})
	const (
		tradeNo = "SUBSCRIPTION_RETURN_QUERY_FAILURE"
		alias   = "return.alias.example.test"
	)
	registerSubscriptionEpayAlias(t, fixture, 7101, alias, "")
	fixture.createPendingOrder(t, tradeNo)

	result := runSubscriptionEpayCallback(
		t,
		"return",
		http.MethodPost,
		signedSubscriptionEpayValues(tradeNo, nil),
		alias,
	)
	require.Equal(t, http.StatusFound, result.Code)
	assert.Equal(t, "https://"+alias+"/wallet?pay=pending", result.Header().Get("Location"))
	assert.Equal(t, 1, fixture.gateway.callCount())
	assertSubscriptionEpayPendingWithoutGrant(t, fixture, tradeNo)

	query := fixture.gateway.lastCall(t)
	assert.Equal(t, subscriptionEpayTestMerchantID, query.query.Get("pid"))
	assert.Equal(t, subscriptionEpayTestMerchantKey, query.query.Get("key"))
	assert.Equal(t, tradeNo, query.query.Get("out_trade_no"))
}

func TestSubscriptionRequestEpayKeepsStableNotifyTrustedReturnAndGlobalMerchant(t *testing.T) {
	fixture := setupSubscriptionEpayFixture(t, nil)
	const alias = "checkout.alias.example.test"
	subSiteConfig, err := common.Marshal(sitePayConfig{
		EpayId:     "sub-site-merchant",
		EpayKey:    "sub-site-secret",
		PayAddress: "https://sub-site-gateway.example.invalid",
		PayMethods: []string{"alipay"},
	})
	require.NoError(t, err)
	registerSubscriptionEpayAlias(t, fixture, 7201, alias, string(subSiteConfig))

	body := fmt.Sprintf(`{"plan_id":%d,"payment_method":"alipay"}`, fixture.plan.Id)
	req := httptest.NewRequest(http.MethodPost, "/api/subscription/epay/pay", strings.NewReader(body))
	req.Host = alias
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	ctx.Set("id", fixture.user.Id)

	SubscriptionRequestEpay(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
		URL     string            `json:"url"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, fixture.server.URL+"/submit.php", response.URL)
	assert.Equal(t, subscriptionEpayTestMerchantID, response.Data["pid"])
	assert.Equal(t, "https://callback.gateway.example.test/epay/api/subscription/epay/notify", response.Data["notify_url"])
	assert.Equal(t, "https://"+alias+"/api/subscription/epay/return", response.Data["return_url"])
	assert.Equal(t, subscriptionEpayTestMoney, response.Data["money"])
	assert.Zero(t, fixture.gateway.callCount(), "creating a payment form must not query or fabricate an upstream order")

	paramsForSignature := make(map[string]string, len(response.Data))
	for key, value := range response.Data {
		paramsForSignature[key] = value
	}
	expectedSign := epay.GenerateParams(paramsForSignature, subscriptionEpayTestMerchantKey)["sign"]
	assert.Equal(t, expectedSign, response.Data["sign"], "the real SDK form must use the global merchant key")

	tradeNo := response.Data["out_trade_no"]
	require.NotEmpty(t, tradeNo)
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, fixture.user.Id, order.UserId)
	assert.Equal(t, fixture.plan.Id, order.PlanId)
	assert.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.NotEmpty(t, order.PlanSnapshot, "the fulfillment contract must be captured before payment")

	callback := signedSubscriptionEpayValues(tradeNo, map[string]string{
		"name":  response.Data["name"],
		"money": response.Data["money"],
		"type":  response.Data["type"],
	})
	returned := runSubscriptionEpayCallback(t, "return", http.MethodGet, callback, alias)
	require.Equal(t, http.StatusFound, returned.Code)
	assert.Equal(t, "https://"+alias+"/wallet?pay=success", returned.Header().Get("Location"))
	assert.Equal(t, 1, fixture.gateway.callCount())

	query := fixture.gateway.lastCall(t)
	assert.Equal(t, http.MethodGet, query.method)
	assert.Equal(t, "/api.php", query.path)
	assert.Equal(t, "order", query.query.Get("act"))
	assert.Equal(t, subscriptionEpayTestMerchantID, query.query.Get("pid"))
	assert.Equal(t, subscriptionEpayTestMerchantKey, query.query.Get("key"))
	assert.Equal(t, tradeNo, query.query.Get("out_trade_no"))

	settled := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, settled)
	assert.Equal(t, common.TopUpStatusSuccess, settled.Status)
}

func TestSubscriptionRequestEpayReusesReservedPendingOrderAndOriginalMethodAtPurchaseLimit(t *testing.T) {
	fixture := setupSubscriptionEpayFixture(t, nil)
	operation_setting.PayMethods = []map[string]string{
		{"name": "Alipay", "type": "alipay"},
		{"name": "WeChat", "type": "wxpay"},
	}
	require.NoError(t, fixture.db.Model(&model.SubscriptionPlan{}).
		Where("id = ?", fixture.plan.Id).
		Update("max_purchase_per_user", 1).Error)
	fixture.plan.MaxPurchasePerUser = 1
	model.InvalidateSubscriptionPlanCache(fixture.plan.Id)

	requestPayment := func(paymentMethod string) map[string]string {
		body := fmt.Sprintf(`{"plan_id":%d,"payment_method":%q}`, fixture.plan.Id, paymentMethod)
		req := httptest.NewRequest(http.MethodPost, "/api/subscription/epay/pay", strings.NewReader(body))
		req.Host = "checkout.example.test"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-Proto", "https")
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = req
		ctx.Set("id", fixture.user.Id)

		SubscriptionRequestEpay(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Message string            `json:"message"`
			Data    map[string]string `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.Equal(t, "success", response.Message)
		return response.Data
	}

	first := requestPayment("alipay")
	second := requestPayment("wxpay")
	require.NotEmpty(t, first["out_trade_no"])
	assert.Equal(t, first["out_trade_no"], second["out_trade_no"])
	assert.Equal(t, first["money"], second["money"])
	assert.Equal(t, "alipay", second["type"], "an issued order must retain its original gateway payment type")

	var pendingOrders []model.SubscriptionOrder
	require.NoError(t, fixture.db.Where(
		"user_id = ? AND plan_id = ? AND status = ?",
		fixture.user.Id,
		fixture.plan.Id,
		common.TopUpStatusPending,
	).Find(&pendingOrders).Error)
	require.Len(t, pendingOrders, 1)
	assert.NotEmpty(t, pendingOrders[0].PlanSnapshot)
	assert.True(t, pendingOrders[0].PurchaseLimitReserved)
}

func TestReconcileEpayPendingSubscriptionsUsesGlobalMerchantAndCombinedSummary(t *testing.T) {
	fixture := setupSubscriptionEpayFixture(t, nil)
	const tradeNo = "SUBSCRIPTION_RECONCILE_PAID"

	subSiteConfig, err := common.Marshal(sitePayConfig{
		EpayId:     "unrelated-sub-site-merchant",
		EpayKey:    "unrelated-sub-site-key",
		PayAddress: "https://unrelated-sub-site-gateway.example.invalid",
		PayMethods: []string{"alipay"},
	})
	require.NoError(t, err)
	registerSubscriptionEpayAlias(t, fixture, 7301, "reconcile.alias.example.test", string(subSiteConfig))

	fixture.createPendingOrder(t, tradeNo)
	require.NoError(t, fixture.db.Model(&model.SubscriptionOrder{}).
		Where("trade_no = ?", tradeNo).
		Update("create_time", common.GetTimestamp()-epayReconcileGraceSeconds-1).Error)
	t.Setenv("EPAY_TOPUP_RECONCILE_ENABLED", "true")
	assert.True(t, (epayTopupReconcileHandler{}).Enabled(), "subscription-only work must schedule the existing epay task")

	task, err := model.CreateSystemTask(model.SystemTaskTypeEpayTopupReconcile, nil, nil)
	require.NoError(t, err)
	const runnerID = "subscription-epay-reconcile-test-runner"
	claimedTask, claimed, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeEpayTopupReconcile,
		runnerID,
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, claimedTask)

	(epayTopupReconcileHandler{}).Run(context.Background(), claimedTask, runnerID)
	finishedTask, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finishedTask)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finishedTask.Status)
	var summary epayReconcileTaskSummary
	require.NoError(t, common.Unmarshal([]byte(finishedTask.Result), &summary))
	assert.Zero(t, summary.TopUps.Scanned)
	assert.Equal(t, 1, summary.Subscriptions.Scanned)
	assert.Equal(t, 1, summary.Subscriptions.Settled)
	assert.Zero(t, summary.Subscriptions.Unpaid)
	assert.Zero(t, summary.Subscriptions.Failed)

	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var payload map[string]string
	require.NoError(t, common.Unmarshal([]byte(order.ProviderPayload), &payload))
	assert.Equal(t, "reconcile", payload["source"])
	assert.Equal(t, subscriptionEpayTestMerchantID, payload["pid"])
	assert.Equal(t, "GW"+tradeNo, payload["trade_no"])
	assert.Equal(t, tradeNo, payload["out_trade_no"])

	var subscriptions int64
	require.NoError(t, fixture.db.Model(&model.UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", fixture.user.Id, fixture.plan.Id).
		Count(&subscriptions).Error)
	assert.EqualValues(t, 1, subscriptions)

	query := fixture.gateway.lastCall(t)
	assert.Equal(t, subscriptionEpayTestMerchantID, query.query.Get("pid"))
	assert.Equal(t, subscriptionEpayTestMerchantKey, query.query.Get("key"))
	assert.Equal(t, tradeNo, query.query.Get("out_trade_no"))
}

func TestReconcileEpayPendingSubscriptionsRetriesTransientQueryFailure(t *testing.T) {
	attempts := 0
	fixture := setupSubscriptionEpayFixture(t, func(query url.Values) (int, string) {
		attempts++
		if attempts == 1 {
			return http.StatusBadGateway, `{"error":"temporary gateway failure"}`
		}
		tradeNo := query.Get("out_trade_no")
		return http.StatusOK, fmt.Sprintf(
			`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"type":"alipay","money":%q}`,
			subscriptionEpayTestMerchantID,
			"GW"+tradeNo,
			tradeNo,
			subscriptionEpayTestMoney,
		)
	})
	const tradeNo = "SUBSCRIPTION_RECONCILE_RETRY"
	fixture.createPendingOrder(t, tradeNo)
	require.NoError(t, fixture.db.Model(&model.SubscriptionOrder{}).
		Where("trade_no = ?", tradeNo).
		Update("create_time", common.GetTimestamp()-epayReconcileGraceSeconds-1).Error)

	first, err := reconcileEpayPendingSubscriptionsOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, first.Scanned)
	assert.Equal(t, 1, first.Failed)
	assert.Zero(t, first.Settled)
	assertSubscriptionEpayPendingWithoutGrant(t, fixture, tradeNo)

	second, err := reconcileEpayPendingSubscriptionsOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, second.Scanned)
	assert.Equal(t, 1, second.Settled)
	assert.Zero(t, second.Failed)
	assert.Equal(t, 2, fixture.gateway.callCount())
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
}

func TestReconcileEpayPendingSubscriptionsRejectsIncompleteGatewayEvidence(t *testing.T) {
	testCases := []struct {
		name   string
		body   func(string) string
		unpaid bool
	}{
		{
			name: "missing paid status",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"type":"alipay","money":%q}`,
					subscriptionEpayTestMerchantID, "GW"+tradeNo, tradeNo, subscriptionEpayTestMoney)
			},
			unpaid: true,
		},
		{
			name: "missing merchant id",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"status":1,"trade_no":%q,"out_trade_no":%q,"type":"alipay","money":%q}`,
					"GW"+tradeNo, tradeNo, subscriptionEpayTestMoney)
			},
		},
		{
			name: "missing gateway trade number",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"status":1,"pid":%q,"out_trade_no":%q,"type":"alipay","money":%q}`,
					subscriptionEpayTestMerchantID, tradeNo, subscriptionEpayTestMoney)
			},
		},
		{
			name: "missing merchant order number",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"status":1,"pid":%q,"trade_no":%q,"type":"alipay","money":%q}`,
					subscriptionEpayTestMerchantID, "GW"+tradeNo, subscriptionEpayTestMoney)
			},
		},
		{
			name: "missing payment type",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"money":%q}`,
					subscriptionEpayTestMerchantID, "GW"+tradeNo, tradeNo, subscriptionEpayTestMoney)
			},
		},
		{
			name: "missing amount",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"type":"alipay"}`,
					subscriptionEpayTestMerchantID, "GW"+tradeNo, tradeNo)
			},
		},
		{
			name: "wrong merchant order number",
			body: func(tradeNo string) string {
				return fmt.Sprintf(`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":"another-order","type":"alipay","money":%q}`,
					subscriptionEpayTestMerchantID, "GW"+tradeNo, subscriptionEpayTestMoney)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := setupSubscriptionEpayFixture(t, func(query url.Values) (int, string) {
				return http.StatusOK, testCase.body(query.Get("out_trade_no"))
			})
			const tradeNo = "SUBSCRIPTION_RECONCILE_REJECTED"
			fixture.createPendingOrder(t, tradeNo)
			require.NoError(t, fixture.db.Model(&model.SubscriptionOrder{}).
				Where("trade_no = ?", tradeNo).
				Update("create_time", common.GetTimestamp()-epayReconcileGraceSeconds-1).Error)

			summary, err := reconcileEpayPendingSubscriptionsOnce(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 1, summary.Scanned)
			assert.Zero(t, summary.Settled)
			if testCase.unpaid {
				assert.Equal(t, 1, summary.Unpaid)
				assert.Zero(t, summary.Failed)
			} else {
				assert.Zero(t, summary.Unpaid)
				assert.Equal(t, 1, summary.Failed)
			}
			assertSubscriptionEpayPendingWithoutGrant(t, fixture, tradeNo)
		})
	}
}

func TestReconcileEpayPendingOrdersReportsFailureThenRetries(t *testing.T) {
	attempts := 0
	fixture := setupSubscriptionEpayFixture(t, func(query url.Values) (int, string) {
		attempts++
		if attempts == 1 {
			return http.StatusBadGateway, "temporary gateway failure"
		}
		tradeNo := query.Get("out_trade_no")
		return http.StatusOK, fmt.Sprintf(
			`{"code":1,"status":1,"pid":%q,"trade_no":%q,"out_trade_no":%q,"type":"alipay","money":%q}`,
			subscriptionEpayTestMerchantID,
			"GW"+tradeNo,
			tradeNo,
			subscriptionEpayTestMoney,
		)
	})
	const tradeNo = "SUBSCRIPTION_RECONCILE_RETRY"
	fixture.createPendingOrder(t, tradeNo)
	require.NoError(t, fixture.db.Model(&model.SubscriptionOrder{}).
		Where("trade_no = ?", tradeNo).
		Update("create_time", common.GetTimestamp()-epayReconcileGraceSeconds-1).Error)

	task, err := model.CreateSystemTask(model.SystemTaskTypeEpayTopupReconcile, nil, nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeEpayTopupReconcile,
		"subscription-epay-retry-runner",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, claimed)
	(epayTopupReconcileHandler{}).Run(context.Background(), claimedTask, "subscription-epay-retry-runner")
	finishedTask, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finishedTask)
	assert.Equal(t, model.SystemTaskStatusFailed, finishedTask.Status)
	var first epayReconcileTaskSummary
	require.NoError(t, common.Unmarshal([]byte(finishedTask.Result), &first))
	assert.Equal(t, 1, first.Subscriptions.Failed)
	assertSubscriptionEpayPendingWithoutGrant(t, fixture, tradeNo)

	second, err := reconcileEpayPendingOrdersOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, second.Subscriptions.Settled)
	settled := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, settled)
	assert.Equal(t, common.TopUpStatusSuccess, settled.Status)
}

func TestCompleteSubscriptionOrderWithResultDistinguishesReplay(t *testing.T) {
	fixture := setupSubscriptionEpayFixture(t, nil)
	const tradeNo = "SUBSCRIPTION_COMPLETION_RESULT"
	fixture.createPendingOrder(t, tradeNo)

	completed, err := model.CompleteSubscriptionOrderWithResult(
		tradeNo,
		`{"source":"test"}`,
		model.PaymentProviderEpay,
		"alipay",
	)
	require.NoError(t, err)
	assert.True(t, completed)

	completed, err = model.CompleteSubscriptionOrderWithResult(
		tradeNo,
		`{"source":"replay"}`,
		model.PaymentProviderEpay,
		"alipay",
	)
	require.NoError(t, err)
	assert.False(t, completed)

	var count int64
	require.NoError(t, fixture.db.Model(&model.UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", fixture.user.Id, fixture.plan.Id).
		Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
