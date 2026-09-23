package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSiteTopupCostMilliRejectsUnrepresentableCost(t *testing.T) {
	for _, test := range []struct {
		name         string
		money        float64
		discountRate int
		want         int64
	}{
		{"large payment", 30000, model.DiscountRateBase, 30_000_000},
		{"discounted payment", 30000, 8000, 24_000_000},
		{"half milli rounds up", 0.0005, model.DiscountRateBase, 1},
		{"above exact wallet range", 1e13, model.DiscountRateBase, 0},
		{"above int64 range", 1e20, model.DiscountRateBase, 0},
		{"nonfinite amount", math.Inf(1), model.DiscountRateBase, 0},
		{"invalid amount", math.NaN(), model.DiscountRateBase, 0},
		{"negative discount", 30000, -1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, siteTopupCostMilli(test.money, test.discountRate))
		})
	}
}

func TestTopUpQuotaValidation(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	testCases := []struct {
		name        string
		displayType string
		amount      int64
		wantQuota   int
		wantErr     bool
	}{
		{
			name:        "currency amount below limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      4294,
			wantQuota:   2_147_000_000,
		},
		{
			name:        "currency amount above old int32 limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      4295,
			wantQuota:   2_147_500_000,
		},
		{
			name:        "large CNY topup",
			displayType: operation_setting.QuotaDisplayTypeCNY,
			amount:      30000,
			wantQuota:   15_000_000_000,
		},
		{
			name:        "currency amount at wallet limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      18_014_398_509,
			wantQuota:   9_007_199_254_500_000,
		},
		{
			name:        "currency amount above wallet limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      18_014_398_510,
			wantErr:     true,
		},
		{
			name:        "token amount preserves settlement truncation",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      common.MaxQuota,
			wantQuota:   2_147_000_000,
		},
		{
			name:        "token amount at wallet limit preserves settlement truncation",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      common.MaxWalletQuota,
			wantQuota:   9_007_199_254_500_000,
		},
		{
			name:        "token amount above exact input limit",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      common.MaxWalletQuota + 1,
			wantErr:     true,
		},
		{
			name:        "token amount above wallet settlement limit",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      9_007_199_255_000_000,
			wantErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.displayType
			quota, err := getTopUpQuota(tc.amount)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantQuota, quota)
		})
	}
}

func TestValidateTopUpQuotaReturnsMaximumAmount(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	maxAmount := decimal.NewFromInt(common.MaxWalletQuota).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Floor().IntPart()

	_, err := validateTopUpQuota(maxAmount)
	require.NoError(t, err)
	_, err = validateTopUpQuota(maxAmount + 1)
	require.EqualError(t, err, "单笔充值数量不能大于 18014398509")

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	_, err = validateTopUpQuota(common.MaxWalletQuota)
	require.NoError(t, err)
	_, err = validateTopUpQuota(common.MaxWalletQuota + 1)
	require.EqualError(t, err, "单笔充值数量不能大于 9007199254740991")
}

func TestMinimumTopUpAmountPreservesLargeTokenMinimum(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	assert.Equal(t, int64(2_500_000_000), getMinimumTopUpAmount(5000))
	assert.Greater(t, getMinimumTopUpAmount(common.MaxWalletQuota), int64(common.MaxWalletQuota), "an invalid minimum must reject all representable payments")
}

func TestRequestAmountRejectsTopUpThatCannotBeSettled(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/amount",
		strings.NewReader(`{"amount":18014398510}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"error","data":"单笔充值数量不能大于 18014398509"}`, recorder.Body.String())
}

func TestRequestAmountRejectsTopUpThatWouldOverflowWallet(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldDB := model.DB
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		model.DB = oldDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id:       42,
		Username: "topup_capacity_user",
		Quota:    common.MaxWalletQuota - 15_000_000_000 + 1,
		Status:   common.UserStatusEnabled,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 42)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/amount",
		strings.NewReader(`{"amount":30000}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"message":"error","data":"top-up quota limit exceeded"}`, recorder.Body.String())
}

func TestValidateCreditedQuotaRejectsOverflow(t *testing.T) {
	_, err := validateCreditedQuota(decimal.NewFromInt(common.MaxWalletQuota))
	require.NoError(t, err)
	_, err = validateCreditedQuota(decimal.Zero)
	require.EqualError(t, err, "充值额度必须大于 0")
	_, err = validateCreditedQuota(decimal.NewFromInt(common.MaxWalletQuota + 1))
	require.EqualError(
		t,
		err,
		"充值额度超出系统可表示范围",
	)
}

func TestStripePriceAdjustmentsDoNotChangeCreditedQuota(t *testing.T) {
	setupTopUpRequest(t)
	setupStripeTopUpProvider(t, `{}`)
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":2}`))
	quote, err := prepareStripeTopUp(30000, "vip")
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(15_000_000_000).Equal(quote.quota))
	assert.Equal(t, int64(60000), quote.quantity)
	_, err = prepareStripeTopUp(18_014_398_509, "vip")
	require.NoError(t, err)
	_, err = prepareStripeTopUp(18_014_398_510, "vip")
	require.Error(t, err)
}
