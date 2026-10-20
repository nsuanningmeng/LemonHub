package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestDeepSeekHostedToolsBillWithoutTextTokens(t *testing.T) {
	previousQuotaUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaUnit
		operation_setting.DeleteToolPriceForTest("deepseek_search")
		operation_setting.DeleteToolPriceForTest("deepseek_fetch")
	})
	for _, tc := range []struct {
		name                    string
		searchPrice, fetchPrice float64
		wantQuota, wantItems    int
	}{
		{"built-in prices", 0.75, 0.75, 750, 2},
		{"search explicitly free", 0, 0.75, 375, 1},
		{"both explicitly free", 0, 0, 0, 0},
		{"operator prices", 2, 1, 1500, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "built-in prices" {
				operation_setting.DeleteToolPriceForTest("deepseek_search")
				operation_setting.DeleteToolPriceForTest("deepseek_fetch")
			} else {
				operation_setting.SetToolPriceForTest("deepseek_search", tc.searchPrice)
				operation_setting.SetToolPriceForTest("deepseek_fetch", tc.fetchPrice)
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{
				OriginModelName: "deepseek-v4-pro",
				ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
					BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
						"deepseek_search": {CallCount: 1},
						"deepseek_fetch":  {CallCount: 1},
					},
				},
			}
			info.PriceData.GroupRatioInfo.GroupRatio = 1
			summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{})
			assert.Zero(t, summary.TotalTokens)
			assert.False(t, summary.BillableToolResult, "hosted calls bill by tool evidence, without the standalone flag")
			assert.Equal(t, tc.wantQuota, summary.Quota)
			assert.Len(t, summary.ToolSurchargeItems, tc.wantItems)
		})
	}
}
