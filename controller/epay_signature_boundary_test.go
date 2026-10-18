package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpayCallbacksRejectAmbiguousSignaturesBeforeGatewayQuery(t *testing.T) {
	const tradeNo = "SIGNED_BOUNDARY"
	const signatureSuffix = "&out_trade_no=SIGNED_BOUNDARY&pid=subscription-merchant-1001&trade_no=GWSIGNED_BOUNDARY&trade_status=TRADE_SUCCESS&type=alipay"
	testCases := []struct {
		name        string
		productName string
		extraKey    string
		sign        string
	}{
		{
			name:        "separator in product name",
			productName: "item&out_trade_no=other",
			// This is a valid legacy signature, not a random invalid signature.
			sign: epay.MD5String("money=19.95&name=item&out_trade_no=other"+signatureSuffix, subscriptionEpayTestMerchantKey),
		},
		{
			name:        "separator in parameter name",
			productName: "item",
			extraKey:    "z&x",
			sign:        epay.MD5String("money=19.95&name=item"+signatureSuffix+"&z&x=value", subscriptionEpayTestMerchantKey),
		},
		{
			name:        "missing signature",
			productName: "item",
		},
	}
	endpoints := []struct {
		name    string
		path    string
		handler gin.HandlerFunc
		browser bool
	}{
		{"topup notify", "/api/user/epay/notify", EpayNotify, false},
		{"topup return", "/api/user/epay/return", EpayReturn, true},
		{"subscription notify", "/api/subscription/epay/notify", SubscriptionEpayNotify, false},
		{"subscription return", "/api/subscription/epay/return", SubscriptionEpayReturn, true},
	}
	for _, endpoint := range endpoints {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(endpoint.name+"/"+method, func(t *testing.T) {
				fixture := setupSubscriptionEpayFixture(t, nil)
				fixture.createPendingOrder(t, tradeNo)
				require.NoError(t, fixture.db.Create(&model.TopUp{
					UserId: fixture.user.Id, Amount: 10, Money: fixture.plan.PriceAmount,
					TradeNo: tradeNo, PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
					CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
				}).Error)
				initialQuota := epayUserQuota(t, fixture.user.Id)
				router := gin.New()
				router.Any(endpoint.path, endpoint.handler)
				for _, tc := range testCases {
					t.Run(tc.name, func(t *testing.T) {
						params := url.Values{
							"pid": {subscriptionEpayTestMerchantID}, "type": {"alipay"},
							"money": {subscriptionEpayTestMoney}, "name": {tc.productName},
							"out_trade_no": {tradeNo}, "trade_no": {"GW" + tradeNo},
							"trade_status": {epay.StatusTradeSuccess}, "sign_type": {"MD5"},
						}
						if tc.extraKey != "" {
							params.Set(tc.extraKey, "value")
						}
						if tc.sign != "" {
							params.Set("sign", tc.sign)
						}
						request := httptest.NewRequest(method, endpoint.path+"?"+params.Encode(), nil)
						if method == http.MethodPost {
							request = httptest.NewRequest(method, endpoint.path, strings.NewReader(params.Encode()))
							request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
						}
						request.Host = "main.example.test"
						recorder := httptest.NewRecorder()
						router.ServeHTTP(recorder, request)
						if endpoint.browser {
							assert.Equal(t, http.StatusFound, recorder.Code)
							location, err := url.Parse(recorder.Header().Get("Location"))
							require.NoError(t, err)
							assert.Equal(t, "/wallet", location.Path)
							assert.Equal(t, "fail", location.Query().Get("pay"))
						} else {
							assert.Equal(t, http.StatusOK, recorder.Code)
							assert.Equal(t, "fail", recorder.Body.String())
						}
						assert.Zero(t, fixture.gateway.callCount(), "invalid callbacks must not trigger gateway requests")
						assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, tradeNo))
						assert.Equal(t, initialQuota, epayUserQuota(t, fixture.user.Id))
						order := model.GetSubscriptionOrderByTradeNo(tradeNo)
						require.NotNil(t, order)
						assert.Equal(t, common.TopUpStatusPending, order.Status)
						assert.Empty(t, order.ProviderPayload)
						var grants int64
						require.NoError(t, fixture.db.Model(&model.UserSubscription{}).Where("user_id = ?", fixture.user.Id).Count(&grants).Error)
						assert.Zero(t, grants)
					})
				}
			})
		}
	}
}
