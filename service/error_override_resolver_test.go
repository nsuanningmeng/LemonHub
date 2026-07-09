package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Resolution contract: per-channel override wins over the global override;
// the global override applies to channels without their own config (and to
// channel-routing errors where no channel is bound at all).
func TestErrorOverrideTextForChannelPrecedence(t *testing.T) {
	origEnabled := operation_setting.ErrorOverrideGlobalEnabled
	origMessage := operation_setting.ErrorOverrideGlobalMessage
	t.Cleanup(func() {
		operation_setting.ErrorOverrideGlobalEnabled = origEnabled
		operation_setting.ErrorOverrideGlobalMessage = origMessage
	})

	channelOn := dto.ChannelSettings{ErrorOverrideEnabled: true, ErrorOverrideMessage: "渠道文案"}
	channelOff := dto.ChannelSettings{}

	// 全都未配置：不替换
	operation_setting.ErrorOverrideGlobalEnabled = false
	_, ok := ErrorOverrideTextForChannel(channelOff)
	require.False(t, ok)

	// 仅全局开启：全局文案兜底；留空回退默认文案
	operation_setting.ErrorOverrideGlobalEnabled = true
	operation_setting.ErrorOverrideGlobalMessage = "全局文案"
	text, ok := ErrorOverrideTextForChannel(channelOff)
	require.True(t, ok)
	assert.Equal(t, "全局文案", text)

	operation_setting.ErrorOverrideGlobalMessage = "   "
	text, ok = ErrorOverrideTextForChannel(channelOff)
	require.True(t, ok)
	assert.Equal(t, "上游服务暂时不可用，请稍后重试", text)

	// 渠道配置优先于全局
	operation_setting.ErrorOverrideGlobalMessage = "全局文案"
	text, ok = ErrorOverrideTextForChannel(channelOn)
	require.True(t, ok)
	assert.Equal(t, "渠道文案", text)

	// 仅渠道开启（全局关闭）
	operation_setting.ErrorOverrideGlobalEnabled = false
	text, ok = ErrorOverrideTextForChannel(channelOn)
	require.True(t, ok)
	assert.Equal(t, "渠道文案", text)
}

// 渠道来源错误的屏蔽按「泄密关键词」分类：暴露渠道/号池信息的报错（账号、
// 密钥、额度、oauth、pool 等）被替换；对用户有用的报错（内容审核、上下文
// 超长等）必须原样透传。
func TestErrorOverrideForChannelErrorKeywordGating(t *testing.T) {
	origEnabled := operation_setting.ErrorOverrideGlobalEnabled
	origMessage := operation_setting.ErrorOverrideGlobalMessage
	t.Cleanup(func() {
		operation_setting.ErrorOverrideGlobalEnabled = origEnabled
		operation_setting.ErrorOverrideGlobalMessage = origMessage
	})
	operation_setting.ErrorOverrideGlobalEnabled = true
	operation_setting.ErrorOverrideGlobalMessage = "统一文案"

	leakTexts := []string{
		"No available OAuth accounts in pool",
		"Incorrect API key provided: sk-abc123",
		"You exceeded your current quota, please check your plan and billing",
		"This organization has been disabled.",
		"Your credit balance is too low",
		"status_code=503, no_available_accounts",
		"当前号池已无可用账号",
	}
	for _, text := range leakTexts {
		_, masked := ErrorOverrideForChannelError(dto.ChannelSettings{}, text)
		assert.True(t, masked, "leak text should be masked: %s", text)
	}

	passTexts := []string{
		"Your request was rejected as a result of our safety system",
		"This model's maximum context length is 8192 tokens, however you requested 20000 tokens",
		"Invalid value for 'temperature': must be between 0 and 2",
		"The server is overloaded, please try again later",
	}
	for _, text := range passTexts {
		_, masked := ErrorOverrideForChannelError(dto.ChannelSettings{}, text)
		assert.False(t, masked, "useful text must pass through: %s", text)
	}

	// 渠道级开启同样按泄密分类：非泄密文本不替换，泄密文本用渠道文案
	channelOn := dto.ChannelSettings{ErrorOverrideEnabled: true, ErrorOverrideMessage: "渠道文案"}
	_, masked := ErrorOverrideForChannelError(channelOn, passTexts[0])
	assert.False(t, masked)
	text, masked := ErrorOverrideForChannelError(channelOn, leakTexts[0])
	require.True(t, masked)
	assert.Equal(t, "渠道文案", text)
}

// 任务链路的计费预扣费失败是本站错误：LocalError 必须为 true，否则会被当作
// 渠道错误进入统一错误信息屏蔽/错误日志/重试逻辑（用户额度不足会被误报为上游故障）。
func TestTaskErrorFromAPIErrorIsLocal(t *testing.T) {
	t.Parallel()

	apiErr := types.NewErrorWithStatusCode(errors.New("用户额度不足"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	taskErr := TaskErrorFromAPIError(apiErr)
	require.NotNil(t, taskErr)
	assert.True(t, taskErr.LocalError)
	assert.Equal(t, string(types.ErrorCodeInsufficientUserQuota), taskErr.Code)
	assert.Equal(t, "用户额度不足", taskErr.Message)
}
