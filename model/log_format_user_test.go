package model

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsRemovesPrivilegedMetadataWithoutChangingBillingValues(t *testing.T) {
	for _, logType := range []int{LogTypeConsume, LogTypeError, LogTypeManage} {
		t.Run(fmt.Sprintf("type_%d", logType), func(t *testing.T) {
			log := &Log{
				Id: 99, Type: logType, ChannelName: "private-channel", Content: "public content",
				Other: `{"channel_id":7,"channel_name":"private-channel","channel_type":1,"reject_reason":"private-policy","root_info":{"secret":"root"},"admin_info":{"secret":"admin"},"audit_info":{"route":"private"},"stream_status":{"error":"private"},"is_model_mapped":true,"upstream_model_name":"private-model","quota":9007199254740993,"model_ratio":0.125,"zero":0,"enabled":false,"op":{"action":"user.update"}}`,
			}
			formatUserLogs([]*Log{log}, 20)
			assert.Equal(t, 21, log.Id)
			assert.Empty(t, log.ChannelName)
			assert.Equal(t, "public content", log.Content)
			var other map[string]json.RawMessage
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			for _, key := range []string{"channel_id", "channel_name", "channel_type", "reject_reason", "root_info", "admin_info", "audit_info", "stream_status", "is_model_mapped", "upstream_model_name"} {
				assert.NotContains(t, other, key)
			}
			assert.Equal(t, json.RawMessage(`9007199254740993`), other["quota"])
			assert.Equal(t, json.RawMessage(`0.125`), other["model_ratio"])
			assert.Equal(t, json.RawMessage(`0`), other["zero"])
			assert.Equal(t, json.RawMessage(`false`), other["enabled"])
			assert.JSONEq(t, `{"action":"user.update"}`, string(other["op"]))
		})
	}
}

func TestFormatUserLogsDiscardsMalformedMetadata(t *testing.T) {
	for _, other := range []string{`{"root_info":{"secret":"private"}`, `["private"]`, `null`, ""} {
		log := &Log{Other: other, ChannelName: "private"}
		formatUserLogs([]*Log{log}, 0)
		assert.JSONEq(t, `{}`, log.Other)
		assert.Empty(t, log.ChannelName)
	}
}

func TestFormatUserLogsMasksEnabledOverrideWithMalformedText(t *testing.T) {
	log := &Log{Content: "private upstream error", Other: `{"admin_info":{"error_override_enabled":true,"error_override_text":7},"error_code":"private-code"}`}
	formatUserLogs([]*Log{log}, 0)
	assert.Equal(t, dto.DefaultErrorOverrideMessage, log.Content)
	assert.JSONEq(t, `{"error_code":"upstream_error"}`, log.Other)
}

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
