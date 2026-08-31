package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRechargeCreemEmailEnrichmentIsNormalizedUniqueAndNonBlocking(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	holder := createReserveTestUser(t, 0)
	holder.SiteId = 17
	holder.Email = "taken@example.com"
	require.NoError(t, DB.Model(&User{}).Where("id = ?", holder.Id).
		Updates(map[string]interface{}{"site_id": holder.SiteId, "email": holder.Email}).Error)

	createPendingCreem := func(userId int, tradeNo string) {
		t.Helper()
		require.NoError(t, DB.Create(&TopUp{
			SiteId:          17,
			UserId:          userId,
			Amount:          100,
			Money:           1,
			TradeNo:         tradeNo,
			PaymentMethod:   PaymentMethodCreem,
			PaymentProvider: PaymentProviderCreem,
			Status:          common.TopUpStatusPending,
			CreateTime:      common.GetTimestamp(),
		}).Error)
	}

	duplicateUser := createReserveTestUser(t, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", duplicateUser.Id).Update("site_id", 17).Error)
	createPendingCreem(duplicateUser.Id, "creem-email-duplicate")
	require.NoError(t, RechargeCreem("creem-email-duplicate", " TAKEN@EXAMPLE.COM ", "", "127.0.0.1"))

	var duplicateReloaded User
	require.NoError(t, DB.First(&duplicateReloaded, duplicateUser.Id).Error)
	assert.Empty(t, duplicateReloaded.Email, "duplicate profile email is skipped without blocking paid credit")
	assert.Equal(t, 100, duplicateReloaded.Quota)

	uniqueUser := createReserveTestUser(t, 50)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", uniqueUser.Id).Update("site_id", 17).Error)
	uniqueUser.SiteId = 17
	require.NoError(t, populateUserCache(uniqueUser))
	reserved, err := TryReserveUserQuota(uniqueUser.Id, 30)
	require.NoError(t, err)
	require.True(t, reserved)
	createPendingCreem(uniqueUser.Id, "creem-email-unique")
	require.NoError(t, RechargeCreem("creem-email-unique", " New@Example.COM ", "", "127.0.0.1"))

	var uniqueReloaded User
	require.NoError(t, DB.First(&uniqueReloaded, uniqueUser.Id).Error)
	assert.Equal(t, "new@example.com", uniqueReloaded.Email)
	assert.Equal(t, 120, uniqueReloaded.Quota, "paid credit and the prior debit are both durable")
	cached, err := GetUserCache(uniqueUser.Id)
	require.NoError(t, err)
	assert.Equal(t, 120, cached.Quota, "email enrichment must preserve Redis' spendable balance")
}
