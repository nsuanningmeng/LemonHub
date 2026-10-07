package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestLayeredOptionPersistenceAndReloadRejectMalformedBeforeMutation(t *testing.T) {
	db := optionPersistenceDB(t)
	cfg := config.GlobalConfig.Get("billing_setting")
	saved, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(cfg, saved)) })
	const key = "billing_setting.billing_mode"
	const good = `{"layered-model":"tiered_expr"}`
	require.NoError(t, UpdateOption(key, good))
	require.Equal(t, "tiered_expr", billing_setting.GetBillingMode("layered-model"))
	err = UpdateOptionsBulk(map[string]string{key: `{"private-secret":`, "billing_setting.billing_expr": `{"layered-model":"p * 2"}`})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-secret")
	assert.Equal(t, good, requireOptionValue(t, db, key))
	assert.Equal(t, good, common.OptionMap[key])
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode("layered-model"))
	require.NoError(t, db.Model(&Option{}).Where("key = ?", key).Update("value", "{private-secret-invalid").Error)
	require.Error(t, loadOptionsFromDatabase())
	assert.Equal(t, good, common.OptionMap[key])
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode("layered-model"))
	require.Error(t, InitOptionMap())
	assert.Equal(t, good, common.OptionMap[key])
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode("layered-model"))
	assert.Equal(t, "{private-secret-invalid", requireOptionValue(t, db, key))
}
func TestLayeredOptionValidStagingSQLFailureKeepsLiveAndStoredState(t *testing.T) {
	db := optionPersistenceDB(t)
	cfg := config.GlobalConfig.Get("billing_setting")
	saved, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(cfg, saved)) })
	const key = "billing_setting.billing_mode"
	const good = `{"layered-sql":"tiered_expr"}`
	require.NoError(t, UpdateOption(key, good))
	forced := errors.New("forced option SQL failure")
	const callback = "test:layered_write_failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(forced)
		}
	}))
	err = UpdateOption(key, `{}`)
	require.NoError(t, db.Callback().Update().Remove(callback))
	require.ErrorIs(t, err, forced)
	assert.Equal(t, good, requireOptionValue(t, db, key))
	assert.Equal(t, good, common.OptionMap[key])
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode("layered-sql"))
	require.NoError(t, UpdateOption("unregistered.opaque", "{invalid"))
	assert.Equal(t, "{invalid", requireOptionValue(t, db, "unregistered.opaque"))
}
