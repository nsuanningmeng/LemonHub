package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: remix 预设的时长/分辨率倍率必须在步骤 4 的基础价格重建后存活，
// 并进入步骤 6 的预扣费额度计算。修复前 RelayTaskSubmit 直接整体替换
// info.PriceData，导致 8 秒 remix 按 1 秒基价预扣与结算（少计费）。
func TestRebasePriceDataPreservesRemixPresetRatiosIntoQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	// ResolveOriginTask 的 remix 路径：从原始任务 BillingContext 预设倍率
	info.PriceData.AddOtherRatio("seconds", 8)
	info.PriceData.AddOtherRatio("size", 1.666667)

	// RelayTaskSubmit 步骤 4：基础价格重建
	rebasePriceData(info, types.PriceData{Quota: 1000, UsePrice: true})

	ratios := info.PriceData.OtherRatios()
	require.Equal(t, map[string]float64{"seconds": 8, "size": 1.666667}, ratios)

	// 步骤 6：倍率应用到基础额度（饱和转换）
	quota, clamp := common.QuotaFromFloatChecked(info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.Quota)))
	require.Nil(t, clamp)
	assert.Equal(t, 13333, quota) // 1000 × 8 × 1.666667，截断取整
}

func TestRebasePriceDataWithoutPresetKeepsBaseQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{}

	rebasePriceData(info, types.PriceData{Quota: 1000, UsePrice: true})

	assert.Nil(t, info.PriceData.OtherRatios())
	quota, clamp := common.QuotaFromFloatChecked(info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.Quota)))
	require.Nil(t, clamp)
	assert.Equal(t, 1000, quota)
}

// RelayTaskSubmit 每次重试都会重新执行步骤 4/5：预设倍率必须在第二次重建后
// 仍然存活，且同 key 的适配器估算（步骤 5 的 AddOtherRatio）覆盖预设值。
func TestRebasePriceDataSurvivesRetryAndEstimateOverride(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	info.PriceData.AddOtherRatio("seconds", 8)

	// 第一次尝试：重建 + 适配器估算覆盖同 key
	rebasePriceData(info, types.PriceData{Quota: 1000, UsePrice: true})
	info.PriceData.AddOtherRatio("seconds", 4)
	require.Equal(t, map[string]float64{"seconds": 4}, info.PriceData.OtherRatios())

	// 重试：再次重建，上一次生效的倍率整体保留
	rebasePriceData(info, types.PriceData{Quota: 1000, UsePrice: true})
	assert.Equal(t, map[string]float64{"seconds": 4}, info.PriceData.OtherRatios())
}
