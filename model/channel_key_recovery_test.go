package model

import (
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHealthRecoveryUpdatesOneCurrentKeyAndCacheWithoutChangingAdminContract(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache_%t", memory), func(t *testing.T) {
			ch := setupAutomaticChannelStatusTest(t, common.ChannelStatusAutoDisabled, true, memory)
			require.True(t, RecoverAutoDisabledChannelKey(ch.Id, "key-a"))
			require.True(t, RecoverAutoDisabledChannelKey(ch.Id, "key-b"), "channel enabled after first key must not skip sibling recovery")
			assert.False(t, RecoverAutoDisabledChannelKey(ch.Id, "key-a"), "already-enabled key is not a new recovery")
			stored, err := GetChannelById(ch.Id, true)
			require.NoError(t, err)
			assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
			assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
			assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
			var ability Ability
			require.NoError(t, DB.Where("channel_id = ?", ch.Id).First(&ability).Error)
			assert.True(t, ability.Enabled)
			if memory {
				cached, err := CacheGetChannel(ch.Id)
				require.NoError(t, err)
				assert.Equal(t, common.ChannelStatusEnabled, cached.Status)
				assert.Empty(t, cached.ChannelInfo.MultiKeyStatusList)
			}
			require.True(t, UpdateChannelStatus(ch.Id, "", common.ChannelStatusManuallyDisabled, "administrator"))
			assert.False(t, RecoverAutoDisabledChannelKey(ch.Id, "key-a"))
			require.True(t, UpdateChannelStatus(ch.Id, "", common.ChannelStatusEnabled, "administrator enabled key"), "manual enable remains legal")
		})
	}
}

func TestHealthRecoveryCancellationAndIdentityChangesFailClosed(t *testing.T) {
	ch := setupAutomaticChannelStatusTest(t, common.ChannelStatusAutoDisabled, true, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, RecoverAutoDisabledChannelKey(ch.Id, "key-a", ctx))
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", ch.Id).Update("key", "replacement\nkey-b").Error)
	assert.False(t, RecoverAutoDisabledChannelKey(ch.Id, "key-a"))
	stored, err := GetChannelById(ch.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "replacement\nkey-b", stored.Key)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
}
