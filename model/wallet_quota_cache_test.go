package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLargeWalletCreditsAndDebitsRemainExactInCache(t *testing.T) {
	useUserCacheMiniRedis(t)
	const credit = 15_000_000_001
	user := createReserveTestUser(t, common.MaxWalletQuota-credit)
	require.NoError(t, populateUserCache(user))

	require.NoError(t, IncreaseUserQuota(user.Id, credit, true))
	assert.Equal(t, common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, common.MaxWalletQuota, cached.Quota)

	require.ErrorIs(t, IncreaseUserQuota(user.Id, 1, true), common.ErrWalletQuotaOutOfRange)
	require.NoError(t, DecreaseUserQuota(user.Id, credit, true))
	assert.Equal(t, user.Quota, getUserQuotaFromDB(t, user.Id))
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, user.Quota, cached.Quota)
}

func TestWalletDebitRejectsNegativeSafeIntegerOverflow(t *testing.T) {
	useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, -common.MaxWalletQuota+1)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DecreaseUserQuota(user.Id, 1, true))
	require.Error(t, DecreaseUserQuota(user.Id, 1, true))
	assert.Equal(t, -common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, -common.MaxWalletQuota, cached.Quota)
	require.ErrorIs(t, persistUserQuotaDeltaDirect(user.Id, -1), common.ErrWalletQuotaOutOfRange)
}
