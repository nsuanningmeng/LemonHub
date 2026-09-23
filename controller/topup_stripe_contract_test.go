package controller

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripeQuoteCheckoutAndCreditUseSameUnits(t *testing.T) {
	for _, test := range []struct {
		name, display           string
		amount, units, quantity int64
		ratio, discount         float64
	}{
		{"large discounted recharge", operation_setting.QuotaDisplayTypeCNY, 30000, 30000, 24000, 1, 0.8},
		{"group price adjustment", operation_setting.QuotaDisplayTypeUSD, 30000, 30000, 60000, 2, 1},
		{"disabled discount", operation_setting.QuotaDisplayTypeUSD, 30000, 30000, 30000, 1, 0},
		{"token display", operation_setting.QuotaDisplayTypeTokens, 500000, 1, 1, 1, 1},
		{"large decimal ratio", operation_setting.QuotaDisplayTypeUSD, 9_007_199_250, 9_007_199_250, 9_907_919_175, 1.1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, user := setupTopUpRequest(t)
			requests := setupStripeTopUpProvider(t, `{"id":"price_wallet","active":true,"type":"one_time","billing_scheme":"per_unit","currency":"usd","unit_amount":100,"unit_amount_decimal":"100"}`)
			operation_setting.GetGeneralSetting().QuotaDisplayType = test.display
			operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{int(test.amount): test.discount}
			require.NoError(t, common.UpdateTopupGroupRatioByJSONString(fmt.Sprintf(`{"default":%v}`, test.ratio)))
			for _, handler := range []struct {
				name string
				run  gin.HandlerFunc
			}{
				{"quote", RequestStripeAmount}, {"pay", RequestStripePay},
			} {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Set("id", user.Id)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/"+handler.name,
					strings.NewReader(fmt.Sprintf(`{"amount":%d,"payment_method":"stripe"}`, test.amount)))
				ctx.Request.Header.Set("Content-Type", "application/json")
				handler.run(ctx)
				require.Equal(t, http.StatusOK, recorder.Code)
				if handler.name == "quote" {
					require.JSONEq(t, fmt.Sprintf(`{"message":"success","data":"%d.00"}`, test.quantity), recorder.Body.String())
				} else {
					require.JSONEq(t, `{"message":"success","data":{"pay_link":"https://checkout.example.invalid/large"}}`, recorder.Body.String())
				}
			}
			submitted := <-requests
			assert.Equal(t, fmt.Sprint(test.quantity), submitted.Get("line_items[0][quantity]"))
			assert.Equal(t, "usd", submitted.Get("currency"))
			var order model.TopUp
			require.NoError(t, db.Where("trade_no = ?", submitted.Get("client_reference_id")).First(&order).Error)
			assert.Equal(t, test.units, order.Amount)
			assert.Equal(t, float64(test.units), order.Money)
			credit, err := common.WalletQuotaFromDecimalStrict(decimal.NewFromFloat(order.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
			require.NoError(t, err)
			assert.Equal(t, test.units*500000, int64(credit))
		})
	}
}

func TestStripeRejectsInexactOrInvalidAmountsBeforeCheckout(t *testing.T) {
	for _, test := range []struct {
		name                           string
		amount                         int64
		display                        string
		ratio, unitPrice, quotaPerUnit float64
	}{
		{"fractional token unit", 500001, operation_setting.QuotaDisplayTypeTokens, 1, 1, 500000},
		{"fractional checkout quantity", 10, operation_setting.QuotaDisplayTypeUSD, 0.95, 1, 500000},
		{"unsafe integer input", common.MaxWalletQuota + 1, operation_setting.QuotaDisplayTypeUSD, 1, 1, 1},
		{"nonfinite unit price", 30000, operation_setting.QuotaDisplayTypeUSD, 1, math.NaN(), 500000},
		{"nonfinite quota unit", 30000, operation_setting.QuotaDisplayTypeTokens, 1, 1, math.Inf(1)},
		{"overflowing quantity", 30000, operation_setting.QuotaDisplayTypeUSD, math.MaxFloat64, 1, 500000},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupTopUpRequest(t)
			setupStripeTopUpProvider(t, `{}`)
			operation_setting.GetGeneralSetting().QuotaDisplayType = test.display
			setting.StripeUnitPrice, common.QuotaPerUnit = test.unitPrice, test.quotaPerUnit
			require.NoError(t, common.UpdateTopupGroupRatioByJSONString(fmt.Sprintf(`{"default":%v}`, test.ratio)))
			var err error
			require.NotPanics(t, func() { _, err = prepareStripeTopUp(test.amount, "default") })
			require.Error(t, err)
		})
	}
}

func TestStripePriceVerificationUsesCurrencyMinorUnits(t *testing.T) {
	for _, test := range []struct {
		currency, minor, want string
		price                 float64
		fails                 bool
	}{
		{"usd", "100", "1.00", 1, false},
		{"eur", "100", "1.00", 1, false},
		{"jpy", "1", "1", 1, false},
		{"isk", "100", "1.00", 1, false},
		{"ugx", "100", "1.00", 1, false},
		{"huf", "150", "1.50", 1.5, false},
		{"twd", "150", "1.50", 1.5, false},
		{"isk", "150", "", 1.5, true},
		{"ugx", "150", "", 1.5, true},
		{"kwd", "1000", "", 1, true},
		{"", "100", "", 1, true},
		{"usd", "101", "", 1, true},
		{"usd", "100.000000000001", "", 1, true},
	} {
		t.Run(test.currency+"/"+test.minor, func(t *testing.T) {
			setupTopUpRequest(t)
			setupStripeTopUpProvider(t, fmt.Sprintf(`{"id":"price_wallet","active":true,"type":"one_time","billing_scheme":"per_unit","currency":%q,"unit_amount_decimal":%q}`, test.currency, test.minor))
			setting.StripeUnitPrice = test.price
			quote, err := prepareStripeTopUp(1, "default")
			require.NoError(t, err)
			err = quote.verifyPrice(context.Background())
			if test.fails {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, quote.payMoney)
			assert.Equal(t, test.currency, quote.currency)
		})
	}
}

func TestStripeRejectsPriceThatCanChangeCheckoutAmount(t *testing.T) {
	for _, fields := range []string{
		`"active":false,"type":"one_time","billing_scheme":"per_unit"`,
		`"active":true,"type":"recurring","billing_scheme":"per_unit"`,
		`"active":true,"type":"one_time","billing_scheme":"tiered"`,
		`"active":true,"type":"one_time","billing_scheme":"per_unit","transform_quantity":{"divide_by":10,"round":"up"}`,
		`"active":true,"type":"one_time","billing_scheme":"per_unit","custom_unit_amount":{"enabled":true}`,
	} {
		t.Run(fields, func(t *testing.T) {
			setupTopUpRequest(t)
			setupStripeTopUpProvider(t, `{"id":"price_wallet","currency":"usd","unit_amount":100,`+fields+`}`)
			quote, err := prepareStripeTopUp(30000, "default")
			require.NoError(t, err)
			require.Error(t, quote.verifyPrice(context.Background()))
		})
	}
}

func TestStripeCheckoutUsesVerifiedConfigurationSnapshot(t *testing.T) {
	setupTopUpRequest(t)
	setupStripeTopUpProvider(t, `{}`)
	type checkoutRequest struct {
		form          url.Values
		authorization string
	}
	requests := make(chan checkoutRequest, 1)
	changed := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			setting.StripePriceId = "price_changed"
			setting.StripeApiSecret = "sk_test_changed"
			close(changed)
			_, _ = w.Write([]byte(`{"id":"price_wallet","active":true,"type":"one_time","billing_scheme":"per_unit","currency":"usd","unit_amount":100}`))
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- checkoutRequest{r.PostForm, r.Header.Get("Authorization")}
		_, _ = w.Write([]byte(`{"id":"cs_snapshot","object":"checkout.session","url":"https://checkout.example.invalid/snapshot"}`))
	}))
	t.Cleanup(provider.Close)
	stripeCheckoutBackend = newStripeCheckoutBackend(provider.Client(), provider.URL)
	quote, err := prepareStripeTopUp(30000, "default")
	require.NoError(t, err)
	require.NoError(t, quote.verifyPrice(context.Background()))
	<-changed
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/pay", nil)
	_, err = genStripeLink(ctx, "ref_snapshot", "", "", quote, "", "")
	require.NoError(t, err)
	submitted := <-requests
	assert.Equal(t, "price_wallet", submitted.form.Get("line_items[0][price]"))
	assert.Equal(t, "Bearer sk_test_wallet", submitted.authorization)
}
