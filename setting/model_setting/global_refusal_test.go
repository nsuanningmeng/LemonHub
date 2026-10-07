package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeRefusalPricingOptionDefaultsOffAndRoundTrips(t *testing.T) {
	var settings GlobalSettings
	manager := config.NewConfigManager()
	manager.Register("global", &settings)
	assert.False(t, settings.ClaudePreOutputRefusalFreeEnabled)
	require.NoError(t, manager.LoadFromDB(map[string]string{"global.claude_pre_output_refusal_free_enabled": "true"}))
	assert.True(t, settings.ClaudePreOutputRefusalFreeEnabled)
	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	assert.Equal(t, "true", saved["global.claude_pre_output_refusal_free_enabled"])
	require.NoError(t, manager.LoadFromDB(map[string]string{"global.claude_pre_output_refusal_free_enabled": "false"}))
	assert.False(t, settings.ClaudePreOutputRefusalFreeEnabled)
}
