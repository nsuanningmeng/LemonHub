package common

import (
	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestChannelInitializationIsolatesOverrideHeadersAndAuditPerAttempt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &RelayInfo{RequestHeaders: map[string]string{"x-original-client": "unchanged"}}
	rootcommon.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	rootcommon.SetContextKey(c, constant.ContextKeyChannelId, 1)
	rootcommon.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, map[string]any{"X-First-Configured": "first"})
	rootcommon.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"model": "first-model", "operations": []any{map[string]any{"mode": "set_header", "path": "Authorization", "value": "Bearer first"}}})
	info.InitChannelMeta(c)
	_, err := ApplyParamOverrideWithRelayInfo([]byte(`{"model":"original"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.NotEmpty(t, info.ParamOverrideAudit)
	require.Equal(t, "Bearer first", GetEffectiveHeaderOverride(info)["authorization"])
	for _, sameChannel := range []bool{false, true} {
		if !sameChannel {
			rootcommon.SetContextKey(c, constant.ContextKeyChannelId, 2)
		}
		rootcommon.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, map[string]any{"X-Current-Configured": "current"})
		rootcommon.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any(nil))
		info.InitChannelMeta(c)
		assert.False(t, info.UseRuntimeHeadersOverride)
		assert.Nil(t, info.RuntimeHeadersOverride)
		assert.Nil(t, info.ParamOverrideAudit)
		assert.Equal(t, map[string]any{"x-current-configured": "current"}, GetEffectiveHeaderOverride(info))
		assert.Equal(t, map[string]string{"x-original-client": "unchanged"}, info.RequestHeaders)
		info.ParamOverride = map[string]any{"model": "current-model", "operations": []any{map[string]any{"mode": "set_header", "path": "X-Current-Operation", "value": "current"}}}
		_, err = ApplyParamOverrideWithRelayInfo([]byte(`{"model":"original"}`), info)
		require.NoError(t, err)
		current := GetEffectiveHeaderOverride(info)
		assert.NotContains(t, current, "authorization")
		assert.NotContains(t, current, "x-first-configured")
		assert.Equal(t, "current", current["x-current-configured"])
		assert.Equal(t, "current", current["x-current-operation"])
		assert.NotContains(t, info.ParamOverrideAudit, "set model = first-model")
	}
}
