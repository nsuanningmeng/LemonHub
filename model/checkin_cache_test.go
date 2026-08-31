package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionalCheckinCreditIsSynchronouslyVisibleInCache(t *testing.T) {
	useUserCacheMiniRedis(t)
	require.NoError(t, DB.AutoMigrate(&Checkin{}, &User{}))
	user := createReserveTestUser(t, 100)
	require.NoError(t, populateUserCache(user))
	checkin := &Checkin{
		UserId:       user.Id,
		CheckinDate:  "2099-12-31",
		QuotaAwarded: 25,
		CreatedAt:    common.GetTimestamp(),
	}
	t.Cleanup(func() {
		_ = DB.Where("user_id = ?", user.Id).Delete(&Checkin{}).Error
		_ = DB.Unscoped().Delete(&user).Error
	})

	_, err := userCheckinWithTransaction(checkin, user.Id, checkin.QuotaAwarded)
	require.NoError(t, err)
	assert.Equal(t, 125, getUserQuotaFromDB(t, user.Id))
	cached, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.Equal(t, 125, cached.Quota)
}

func TestCheckinCreditRejectsInt32OverflowAtomically(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Checkin{}, &User{}))

	t.Run("transactional path", func(t *testing.T) {
		user := createReserveTestUser(t, common.MaxQuota-5)
		checkin := &Checkin{UserId: user.Id, CheckinDate: "2099-12-30", QuotaAwarded: 10, CreatedAt: common.GetTimestamp()}
		t.Cleanup(func() {
			_ = DB.Where("user_id = ?", user.Id).Delete(&Checkin{}).Error
			_ = DB.Unscoped().Delete(&user).Error
		})

		_, err := userCheckinWithTransaction(checkin, user.Id, checkin.QuotaAwarded)
		require.Error(t, err)
		assert.Equal(t, common.MaxQuota-5, getUserQuotaFromDB(t, user.Id))
		var count int64
		require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("sqlite sequential fallback", func(t *testing.T) {
		user := createReserveTestUser(t, common.MaxQuota-5)
		checkin := &Checkin{UserId: user.Id, CheckinDate: "2099-12-29", QuotaAwarded: 10, CreatedAt: common.GetTimestamp()}
		t.Cleanup(func() {
			_ = DB.Where("user_id = ?", user.Id).Delete(&Checkin{}).Error
			_ = DB.Unscoped().Delete(&user).Error
		})

		_, err := userCheckinWithoutTransaction(checkin, user.Id, checkin.QuotaAwarded)
		require.Error(t, err)
		assert.Equal(t, common.MaxQuota-5, getUserQuotaFromDB(t, user.Id))
		var count int64
		require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	})
}
