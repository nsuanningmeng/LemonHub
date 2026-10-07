package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSensitiveWholeWordOptionPersistsAndReloads(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	oldWhole := setting.SensitiveWordsWholeWordEnabled
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.SensitiveWordsWholeWordEnabled = oldWhole
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})
	setting.SensitiveWordsWholeWordEnabled = false
	assert.False(t, setting.SensitiveWordsWholeWordEnabled, "new installations retain substring matching")
	for _, value := range []string{"true", "false"} {
		require.NoError(t, UpdateOption("SensitiveWordsWholeWordEnabled", value))
		var option Option
		require.NoError(t, db.First(&option, "key = ?", "SensitiveWordsWholeWordEnabled").Error)
		assert.Equal(t, value, option.Value)
		assert.Equal(t, value == "true", setting.SensitiveWordsWholeWordEnabled)
		// Reset process state and reload using the actual startup/sync DB loader.
		setting.SensitiveWordsWholeWordEnabled = value != "true"
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, "SensitiveWordsWholeWordEnabled")
		common.OptionMapRWMutex.Unlock()
		loadOptionsFromDatabase()
		assert.Equal(t, value == "true", setting.SensitiveWordsWholeWordEnabled)
		common.OptionMapRWMutex.RLock()
		assert.Equal(t, value, common.OptionMap["SensitiveWordsWholeWordEnabled"])
		common.OptionMapRWMutex.RUnlock()
	}
}
