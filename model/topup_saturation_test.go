package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompleteEpayTopUpRejectsOversizedAmount protects the billing invariant that a
// top-up credit can never wrap negative or reach the reserved int32 saturation bound.
// Invalid legacy orders remain pending for manual resolution and credit no quota.
func TestCompleteEpayTopUpRejectsOversizedAmount(t *testing.T) {
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
	require.ErrorIs(t, err, ErrInvalidTopUpQuota)
	assert.Empty(t, finalStatus)
	assert.Zero(t, quotaAdded)

	var got User
	require.NoError(t, DB.Select("quota").First(&got, u.Id).Error)
	assert.Zero(t, got.Quota)

	var gotTopUp TopUp
	require.NoError(t, DB.Select("status").Where("trade_no = ?", tradeNo).First(&gotTopUp).Error)
	assert.Equal(t, common.TopUpStatusPending, gotTopUp.Status)
}
