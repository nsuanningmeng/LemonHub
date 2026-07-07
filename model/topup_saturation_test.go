package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompleteEpayTopUpSaturatesOversizedAmount protects the billing invariant that a
// top-up credit can never wrap negative (or exceed the int32 quota column): an order
// whose Amount*QuotaPerUnit overflows int32 must settle with the credit clamped to
// common.MaxQuota — a positive, storable value — instead of a wrapped/overflowing one.
func TestCompleteEpayTopUpSaturatesOversizedAmount(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TopUp{}, &User{}))
	const tradeNo = "SATURATE1"
	cleanup := func() {
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
	}
	cleanup()
	defer cleanup()

	// Smallest amount whose credit exceeds the int32 quota bound.
	overflowAmount := int64(float64(common.MaxQuota)/common.QuotaPerUnit) + 10

	pw, _ := common.Password2Hash("x")
	u := &User{Username: "saturateu", Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "saturateaff"}
	require.NoError(t, DB.Create(u).Error)
	defer DB.Where("id = ?", u.Id).Delete(&User{})

	require.NoError(t, DB.Create(&TopUp{
		UserId: u.Id, Amount: overflowAmount, Money: 1, TradeNo: tradeNo,
		PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay",
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
	}).Error)

	finalStatus, quotaAdded, err := CompleteEpayTopUp(tradeNo, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, common.TopUpStatusSuccess, finalStatus)
	assert.Equal(t, common.MaxQuota, quotaAdded, "oversized credit must clamp to MaxQuota, never wrap")

	var got User
	require.NoError(t, DB.Select("quota").First(&got, u.Id).Error)
	assert.Equal(t, common.MaxQuota, got.Quota)
	assert.Positive(t, got.Quota, "a top-up must never debit the account")
}
