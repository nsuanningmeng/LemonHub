package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func cacheFailureFixture(t *testing.T) *Channel {
	t.Helper()
	resetPricingEndpointTestTables(t)
	channel := &Channel{Id: 8831, Type: constant.ChannelTypeOpenAI, Name: "old-cache", Key: "local-test-key", Group: "cache-group", Models: "cache-model", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, InitChannelCache())
	return channel
}

func TestChannelCacheReadFailurePreservesAllPublishedState(t *testing.T) {
	for _, table := range []string{"channels", "abilities"} {
		t.Run(table, func(t *testing.T) {
			channel := cacheFailureFixture(t)
			oldChannel := channelsIDM[channel.Id]
			oldGroups := group2model2channels
			oldAdvanced := channel2advancedCustomConfig
			require.NoError(t, DB.Model(channel).Update("name", "new-db-name").Error)
			var original []Ability
			require.NoError(t, DB.Find(&original).Error)
			callback := "test:cache_read_failure"
			forced := errors.New("forced cache read failure")
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(forced)
				}
			}))
			err := InitChannelCache()
			require.NoError(t, DB.Callback().Query().Remove(callback))
			require.ErrorIs(t, err, forced)
			assert.Same(t, oldChannel, channelsIDM[channel.Id])
			assert.Equal(t, "old-cache", channelsIDM[channel.Id].Name)
			assert.Equal(t, oldGroups, group2model2channels)
			assert.Equal(t, oldAdvanced, channel2advancedCustomConfig)
			var current []Ability
			require.NoError(t, DB.Find(&current).Error)
			assert.Equal(t, original, current)
			require.NoError(t, InitChannelCache())
			assert.Equal(t, "new-db-name", channelsIDM[channel.Id].Name)
		})
	}
}

func TestChannelCacheMissingDerivedGroupDoesNotRepairDatabase(t *testing.T) {
	channel := cacheFailureFixture(t)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Ability{}).Error)
	require.NoError(t, InitChannelCache())
	assert.Equal(t, []int{channel.Id}, group2model2channels[channel.Group][channel.Models])
	var count int64
	require.NoError(t, DB.Model(&Ability{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestFixAbilityConfirmedFailuresRollBackAndKeepCache(t *testing.T) {
	for _, phase := range []string{"channel_read", "delete", "insert", "ability_read"} {
		t.Run(phase, func(t *testing.T) {
			channel := cacheFailureFixture(t)
			old := channelsIDM[channel.Id]
			require.NoError(t, DB.Model(channel).Update("models", "new-model").Error)
			var before []Ability
			require.NoError(t, DB.Find(&before).Error)
			forced := errors.New("forced repair statement failure")
			cb := "test:ability_repair_failure"
			query := func(tx *gorm.DB) {
				if (phase == "channel_read" && tx.Statement.Table == "channels") || (phase == "ability_read" && tx.Statement.Table == "abilities") {
					tx.AddError(forced)
				}
			}
			switch phase {
			case "channel_read", "ability_read":
				require.NoError(t, DB.Callback().Query().After("gorm:query").Register(cb, query))
			case "delete":
				require.NoError(t, DB.Callback().Delete().Before("gorm:delete").Register(cb, func(tx *gorm.DB) {
					if tx.Statement.Table == "abilities" {
						tx.AddError(forced)
					}
				}))
			case "insert":
				require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
					if tx.Statement.Table == "abilities" {
						tx.AddError(forced)
					}
				}))
			}
			success, failed, err := FixAbility()
			switch phase {
			case "channel_read", "ability_read":
				require.NoError(t, DB.Callback().Query().Remove(cb))
			case "delete":
				require.NoError(t, DB.Callback().Delete().Remove(cb))
			case "insert":
				require.NoError(t, DB.Callback().Create().Remove(cb))
			}
			require.ErrorIs(t, err, forced)
			assert.Zero(t, success)
			assert.Zero(t, failed)
			assert.Same(t, old, channelsIDM[channel.Id])
			var after []Ability
			require.NoError(t, DB.Find(&after).Error)
			assert.Equal(t, before, after)
		})
	}
}

func TestFixAbilityCommitUncertaintyDoesNotPublishOrReplay(t *testing.T) {
	for _, mode := range []string{"commit_applied", "commit_not_applied"} {
		t.Run(mode, func(t *testing.T) {
			channel := cacheFailureFixture(t)
			old := channelsIDM[channel.Id]
			require.NoError(t, DB.Model(channel).Update("models", "new-model").Error)
			sqlDB, err := DB.DB()
			require.NoError(t, err)
			pool := &batchOutcomeTestPool{DB: sqlDB, mode: mode, forced: errors.New("lost commit acknowledgement")}
			oldPool, oldStatementPool := DB.ConnPool, DB.Statement.ConnPool
			DB.ConnPool, DB.Statement.ConnPool = pool, pool
			success, failed, err := FixAbility()
			DB.ConnPool, DB.Statement.ConnPool = oldPool, oldStatementPool
			require.ErrorIs(t, err, pool.forced)
			assert.Zero(t, success)
			assert.Zero(t, failed)
			assert.Equal(t, 1, pool.begins)
			assert.Same(t, old, channelsIDM[channel.Id])
			var abilities []Ability
			require.NoError(t, DB.Find(&abilities).Error)
			require.Len(t, abilities, 1)
			expected := "cache-model"
			if mode == "commit_applied" {
				expected = "new-model"
			}
			assert.Equal(t, expected, abilities[0].Model)
		})
	}
}

func TestFixAbilitySuccessPublishesOnlyCompletedRepair(t *testing.T) {
	channel := cacheFailureFixture(t)
	require.NoError(t, DB.Model(channel).Update("models", "repaired-model").Error)
	success, failed, err := FixAbility()
	require.NoError(t, err)
	assert.Equal(t, 1, success)
	assert.Zero(t, failed)
	assert.Equal(t, []int{channel.Id}, group2model2channels[channel.Group]["repaired-model"])
	var abilities []Ability
	require.NoError(t, DB.Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "repaired-model", abilities[0].Model)
}
