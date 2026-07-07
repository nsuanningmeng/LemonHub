package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// FindTopUpByTradeNo must distinguish three outcomes the payment callbacks depend on:
// a found order, a genuinely missing order (nil, nil), and a transient DB failure
// (nil, err) — the latter must NOT read as "order missing", unlike the legacy
// GetTopUpByTradeNo which folds every error into nil.
func TestFindTopUpByTradeNoDiscriminatesErrors(t *testing.T) {
	require.NoError(t, DB.Where("trade_no LIKE ?", "FINDQ%").Delete(&TopUp{}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: 1, Amount: 10, Money: 100, TradeNo: "FINDQ1",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
	}).Error)
	t.Cleanup(func() { DB.Where("trade_no LIKE ?", "FINDQ%").Delete(&TopUp{}) })

	// Found.
	got, err := FindTopUpByTradeNo("FINDQ1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "FINDQ1", got.TradeNo)

	// Genuinely missing → (nil, nil), NOT an error.
	got, err = FindTopUpByTradeNo("FINDQ-does-not-exist")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Transient DB failure → (nil, err); the legacy helper folds the same error to nil.
	origDB := DB
	t.Cleanup(func() { DB = origDB })
	broken, err := gorm.Open(sqlite.Open("file:findq_broken?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, broken.AutoMigrate(&TopUp{}))
	sqlDB, err := broken.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close()) // force every query on this handle to error
	DB = broken

	got, err = FindTopUpByTradeNo("anything")
	assert.Error(t, err, "a closed-connection query must surface as an error")
	assert.Nil(t, got)
	assert.Nil(t, GetTopUpByTradeNo("anything"), "legacy helper folds the transient error to nil")
}

// GetPendingEpayTopUps / HasPendingEpayTopUps must select only pending epay orders inside
// the [createdAfter, createdBefore] window, newest-first, and ignore other providers,
// non-pending statuses, and out-of-window rows.
func TestPendingEpayTopUpsWindowAndOrder(t *testing.T) {
	require.NoError(t, DB.Where("trade_no LIKE ?", "PENDQ%").Delete(&TopUp{}).Error)
	t.Cleanup(func() { DB.Where("trade_no LIKE ?", "PENDQ%").Delete(&TopUp{}) })

	now := common.GetTimestamp()
	mk := func(tradeNo string, provider, status string, createTime int64) {
		require.NoError(t, DB.Create(&TopUp{
			UserId: 1, Amount: 10, Money: 100, TradeNo: tradeNo,
			PaymentProvider: provider, Status: status, CreateTime: createTime,
		}).Error)
	}
	// Insert oldest first so the auto-increment id order matches creation-time order;
	// the query returns newest-first (id desc), i.e. PENDQ_in_new (highest id) first.
	mk("PENDQ_in_old", PaymentProviderEpay, common.TopUpStatusPending, now-1000)  // in window (older)
	mk("PENDQ_in_new", PaymentProviderEpay, common.TopUpStatusPending, now-200)   // in window (newer)
	mk("PENDQ_too_new", PaymentProviderEpay, common.TopUpStatusPending, now-10)   // inside grace: excluded
	mk("PENDQ_too_old", PaymentProviderEpay, common.TopUpStatusPending, now-99999) // past window: excluded
	mk("PENDQ_stripe", PaymentProviderStripe, common.TopUpStatusPending, now-200) // wrong provider: excluded
	mk("PENDQ_done", PaymentProviderEpay, common.TopUpStatusSuccess, now-200)     // not pending: excluded

	after, before := now-5000, now-100
	assert.True(t, HasPendingEpayTopUps(after, before))

	list, err := GetPendingEpayTopUps(after, before, 100)
	require.NoError(t, err)
	got := make([]string, 0, len(list))
	for _, o := range list {
		got = append(got, o.TradeNo)
	}
	// Only the two in-window pending epay orders, NEWEST first.
	assert.Equal(t, []string{"PENDQ_in_new", "PENDQ_in_old"}, got)

	// Empty window → no rows, existence check false.
	assert.False(t, HasPendingEpayTopUps(now+1000, now+2000))
	empty, err := GetPendingEpayTopUps(now+1000, now+2000, 100)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
