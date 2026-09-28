package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]interface{}{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	changed, err := stale.saveStatusState(DB, false)
	require.NoError(t, err)
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func setupAutomaticChannelStatusTest(t *testing.T, status int, multiKey, memoryCache bool) *Channel {
	t.Helper()
	setupChannelStatusTest(t)
	channelSyncLock.Lock()
	oldChannels, oldRoutes, oldConfigs := channelsIDM, group2model2channels, channel2advancedCustomConfig
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM, group2model2channels, channel2advancedCustomConfig = oldChannels, oldRoutes, oldConfigs
		channelSyncLock.Unlock()
	})
	channel := &Channel{
		Name:   "automatic-channel-status",
		Key:    "key-a",
		Status: status,
		Models: "test-model",
		Group:  "default",
	}
	if multiKey {
		channel.Key = "key-a\nkey-b"
		channel.ChannelInfo = ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
			MultiKeyStatusList:   map[int]int{1: common.ChannelStatusAutoDisabled},
		}
		if status != common.ChannelStatusEnabled {
			channel.ChannelInfo.MultiKeyStatusList[0] = common.ChannelStatusAutoDisabled
		}
	}
	require.NoError(t, channel.Insert())
	common.MemoryCacheEnabled = memoryCache
	if memoryCache {
		InitChannelCache()
	}
	return channel
}

func TestAutomaticChannelStatusPreservesManualDisable(t *testing.T) {
	for _, memoryCache := range []bool{false, true} {
		for _, multiKey := range []bool{false, true} {
			for _, target := range []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled} {
				t.Run(fmt.Sprintf("cache=%v/multi-key=%v/target=%d", memoryCache, multiKey, target), func(t *testing.T) {
					channel := setupAutomaticChannelStatusTest(t, common.ChannelStatusManuallyDisabled, multiKey, memoryCache)
					if memoryCache {
						// Another process may have disabled the channel since this cache was loaded.
						cached, err := CacheGetChannel(channel.Id)
						require.NoError(t, err)
						cached.Status = common.ChannelStatusEnabled
					}

					assert.False(t, UpdateChannelStatusAutomatically(channel.Id, "key-a", target, "late probe"))
					stored, err := GetChannelById(channel.Id, true)
					require.NoError(t, err)
					assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
					assert.Equal(t, channel.ChannelInfo, stored.ChannelInfo)
					var ability Ability
					require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
					assert.False(t, ability.Enabled)
					if memoryCache {
						cached, err := CacheGetChannel(channel.Id)
						require.NoError(t, err)
						assert.Equal(t, common.ChannelStatusManuallyDisabled, cached.Status)
						assert.Equal(t, channel.ChannelInfo, cached.ChannelInfo)
					}

					// An explicit administrator action must still be able to enable it.
					require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "manual operation"))
					stored, err = GetChannelById(channel.Id, true)
					require.NoError(t, err)
					assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
					require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
					assert.True(t, ability.Enabled)
				})
			}
		}
	}
}

func TestAutomaticChannelStatusLosesRaceToManualDisable(t *testing.T) {
	for _, multiKey := range []bool{false, true} {
		for _, target := range []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled} {
			t.Run(fmt.Sprintf("multi-key=%v/target=%d", multiKey, target), func(t *testing.T) {
				initial := common.ChannelStatusEnabled
				if target == common.ChannelStatusEnabled {
					initial = common.ChannelStatusAutoDisabled
				}
				channel := setupAutomaticChannelStatusTest(t, initial, multiKey, true)
				interleaved := false
				const callback = "test:manual-disable-before-automatic-write"
				require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
					if interleaved || tx.Statement.Table != "channels" {
						return
					}
					interleaved = true
					manual := tx.Session(&gorm.Session{NewDB: true})
					require.NoError(t, manual.Model(&Channel{}).Where("id = ?", channel.Id).
						Update("status", common.ChannelStatusManuallyDisabled).Error)
					require.NoError(t, manual.Model(&Ability{}).Where("channel_id = ?", channel.Id).
						Update("enabled", false).Error)
				}))
				t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callback)) })

				assert.False(t, UpdateChannelStatusAutomatically(channel.Id, "key-a", target, "late probe"))
				require.True(t, interleaved)
				stored, err := GetChannelById(channel.Id, true)
				require.NoError(t, err)
				assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
				assert.Equal(t, channel.ChannelInfo, stored.ChannelInfo)
				var ability Ability
				require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
				assert.False(t, ability.Enabled)
				cached, err := CacheGetChannel(channel.Id)
				require.NoError(t, err)
				assert.Equal(t, common.ChannelStatusManuallyDisabled, cached.Status)
				assert.Equal(t, channel.ChannelInfo, cached.ChannelInfo)
			})
		}
	}
}

func TestAutomaticChannelStatusCommitsDisableAndRecovery(t *testing.T) {
	for _, multiKey := range []bool{false, true} {
		t.Run(fmt.Sprintf("multi-key=%v", multiKey), func(t *testing.T) {
			channel := setupAutomaticChannelStatusTest(t, common.ChannelStatusEnabled, multiKey, true)
			for _, target := range []int{common.ChannelStatusAutoDisabled, common.ChannelStatusEnabled} {
				require.True(t, UpdateChannelStatusAutomatically(channel.Id, "key-a", target, "health check"))
				stored, err := GetChannelById(channel.Id, true)
				require.NoError(t, err)
				assert.Equal(t, target, stored.Status)
				var ability Ability
				require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
				assert.Equal(t, target == common.ChannelStatusEnabled, ability.Enabled)
				cached, err := CacheGetChannel(channel.Id)
				require.NoError(t, err)
				assert.Equal(t, target, cached.Status)
				assert.Equal(t, stored.ChannelInfo, cached.ChannelInfo)
			}
		})
	}
}

func TestAutomaticChannelStatusRollsBackWhenAbilitiesFail(t *testing.T) {
	channel := setupAutomaticChannelStatusTest(t, common.ChannelStatusEnabled, true, true)
	const callback = "test:reject-ability-status-write"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("ability update failed"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callback)) })

	assert.False(t, UpdateChannelStatusAutomatically(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "failed probe"))
	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, channel.ChannelInfo, stored.ChannelInfo)
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.True(t, ability.Enabled)
	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, cached.Status)
	assert.Equal(t, channel.ChannelInfo, cached.ChannelInfo)
}
