package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// User-facing log views must not reveal the original channel error when the
// unified error message was applied to the response: content is replaced with
// the text the user actually saw, error type/code neutralized, channel fields
// stripped. Admin views do not go through formatUserLogs and keep originals.
func TestFormatUserLogsMasksOverriddenErrorLogs(t *testing.T) {
	t.Parallel()

	makeLog := func(other map[string]interface{}) *Log {
		return &Log{
			Type:    LogTypeError,
			Content: "status_code=503, No available OAuth accounts in pool",
			Other:   common.MapToJsonStr(other),
		}
	}

	overridden := makeLog(map[string]interface{}{
		"error_type":   "openai_error",
		"error_code":   "no_available_accounts",
		"channel_id":   7,
		"channel_name": "claude-max-pool",
		"admin_info": map[string]interface{}{
			"error_override_enabled": true,
			"error_override_text":    "服务繁忙，请稍后重试",
		},
	})
	legacyOverridden := makeLog(map[string]interface{}{
		"error_code": "no_available_accounts",
		"admin_info": map[string]interface{}{
			// v0.4.24 rows carry only the marker, no stored text
			"error_override_enabled": true,
		},
	})
	plain := makeLog(map[string]interface{}{
		"error_code": "server_error",
		"admin_info": map[string]interface{}{"use_channel": []string{"7"}},
	})

	formatUserLogs([]*Log{overridden, legacyOverridden, plain}, 0)

	assert.Equal(t, "服务繁忙，请稍后重试", overridden.Content)
	otherMap, err := common.StrToMap(overridden.Other)
	require.NoError(t, err)
	assert.Equal(t, "upstream_error", otherMap["error_code"])
	assert.Equal(t, "upstream_error", otherMap["error_type"])
	assert.NotContains(t, otherMap, "channel_name")
	assert.NotContains(t, otherMap, "admin_info")

	assert.Equal(t, dto.DefaultErrorOverrideMessage, legacyOverridden.Content)

	// 未启用统一错误信息的行为保持不变
	assert.Equal(t, "status_code=503, No available OAuth accounts in pool", plain.Content)
	plainOther, err := common.StrToMap(plain.Other)
	require.NoError(t, err)
	assert.Equal(t, "server_error", plainOther["error_code"])
}
