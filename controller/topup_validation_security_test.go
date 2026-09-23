package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentQuotesRejectNonFiniteConfiguration(t *testing.T) {
	db, user := setupTopUpRequest(t)
	oldWaffoPrice, oldPancakePrice := setting.WaffoUnitPrice, setting.WaffoPancakeUnitPrice
	t.Cleanup(func() {
		setting.WaffoUnitPrice, setting.WaffoPancakeUnitPrice = oldWaffoPrice, oldPancakePrice
	})
	for _, tc := range []struct {
		name                          string
		price, quotaPerUnit, discount float64
	}{
		{"NaN price", math.NaN(), 500000, 1},
		{"infinite price", math.Inf(1), 500000, 1},
		{"overflowing total", 1e308, 500000, 1},
		{"NaN quota unit", 1, math.NaN(), 1},
		{"infinite quota unit", 1, math.Inf(1), 1},
		{"zero quota unit", 1, 0, 1},
		{"NaN discount", 1, 500000, math.NaN()},
		{"infinite discount", 1, 500000, math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaPerUnit = tc.quotaPerUnit
			operation_setting.Price = tc.price
			setting.WaffoUnitPrice, setting.WaffoPancakeUnitPrice = tc.price, tc.price
			operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{30000: tc.discount}
			for _, provider := range []struct {
				name    string
				handler gin.HandlerFunc
			}{
				{"epay", RequestAmount}, {"waffo", RequestWaffoAmount}, {"pancake", RequestWaffoPancakeAmount},
			} {
				t.Run(provider.name, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Set("id", user.Id)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/amount", strings.NewReader(`{"amount":30000}`))
					ctx.Request.Header.Set("Content-Type", "application/json")
					require.NotPanics(t, func() { provider.handler(ctx) })
					assert.Equal(t, http.StatusOK, recorder.Code)
					var response struct {
						Message string `json:"message"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					assert.Equal(t, "error", response.Message)
				})
			}
		})
	}
	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreemCheckoutRequiresFulfillableWebhook(t *testing.T) {
	db, user := setupTopUpRequest(t)
	confirmPaymentComplianceForTest(t)
	oldKey, oldProducts, oldSecret := setting.CreemApiKey, setting.CreemProducts, setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey, setting.CreemProducts, setting.CreemWebhookSecret = oldKey, oldProducts, oldSecret
	})
	setting.CreemApiKey = "test-key-not-used"
	setting.CreemProducts = `[{"productId":"prod-test","price":30000,"quota":15000000000}]`
	setting.CreemWebhookSecret = ""
	assert.False(t, isCreemTopUpEnabled())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/creem/pay", nil)
	creemAdaptor.RequestPay(ctx, &CreemPayRequest{ProductId: "prod-test", PaymentMethod: model.PaymentMethodCreem})
	assert.Contains(t, recorder.Body.String(), "回调配置不完整")
	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count, "must reject before a pending order or external checkout exists")
}
