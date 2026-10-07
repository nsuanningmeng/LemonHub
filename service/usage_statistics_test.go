package service

import (
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCachedInputStatisticsPreservesBilling(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))
	require.NoError(t, model.DB.Exec("DELETE FROM quota_data").Error)
	previousExport := common.DataExportEnabled
	common.DataExportEnabled = true
	model.CacheQuotaDataLock.Lock()
	previousCache := model.CacheQuotaData
	model.CacheQuotaData = make(map[string]*model.QuotaData)
	model.CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		common.DataExportEnabled = previousExport
		model.CacheQuotaDataLock.Lock()
		model.CacheQuotaData = previousCache
		model.CacheQuotaDataLock.Unlock()
		require.NoError(t, model.DB.Exec("DELETE FROM quota_data").Error)
	})
	const id, balance = 7290, 1000000
	seedUser(t, id, balance)
	seedToken(t, id, id, "claude-statistics", balance)
	seedChannel(t, id)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Set("username", "test_user")
	info := &relaycommon.RelayInfo{
		UserId: id, UserQuota: balance, TokenId: id, TokenKey: "claude-statistics",
		OriginModelName: "anthropic-protocol", UsingGroup: "default", StartTime: time.Now(),
		RelayFormat: relaytypes.RelayFormatClaude, FinalRequestRelayFormat: relaytypes.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: id, ChannelType: constant.ChannelTypeAnthropic},
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1, CacheCreationRatio: 1.25,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
		BillingSource: BillingSourceWallet,
	}
	info.Billing = &BillingSession{relayInfo: info, funding: &WalletFunding{userId: id}}
	raw := &dto.ClaudeUsage{InputTokens: 2, CacheReadInputTokens: 589733, CacheCreationInputTokens: 1088, OutputTokens: 1117}
	usage := &dto.Usage{BillingUsage: dto.NewClaudeMessagesBillingUsage(raw)}
	snapshotBefore := dto.CloneBillingUsage(usage.BillingUsage)
	before, err := common.Marshal(usage)
	require.NoError(t, err)
	PostTextConsumeQuota(c, info, usage, nil)
	after, err := common.Marshal(usage)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
	assert.Equal(t, snapshotBefore, usage.BillingUsage)
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", id).First(&log).Error)
	require.NotNil(t, log.InputTokensTotal)
	assert.Equal(t, int64(590823), *log.InputTokensTotal)
	assert.Equal(t, 2, log.PromptTokens)
	assert.Equal(t, 1117, log.CompletionTokens)
	assert.Equal(t, 61452, log.Quota, "fresh/cache/output pricing is unchanged")
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, float64(589733), other["cache_tokens"])
	assert.Equal(t, float64(1088), other["cache_write_tokens"])
	var user model.User
	require.NoError(t, model.DB.First(&user, id).Error)
	assert.Equal(t, balance-log.Quota, user.Quota)
	stat, err := model.SumUserUsedQuota(id, model.LogTypeConsume, 0, 0, "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, int64(591940), stat.Tpm)
	assert.Equal(t, int64(591940), model.SumUsedToken(model.LogTypeConsume, 0, 0, "anthropic-protocol", "", ""))
	model.SaveQuotaDataCache()
	rows, err := model.GetRankingQuotaTotals(0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(591940), rows[0].TotalTokens)
}

func TestUsageInputTokensForStatistics(t *testing.T) {
	claude := func(raw *dto.ClaudeUsage) *dto.Usage {
		return &dto.Usage{PromptTokens: 999, BillingUsage: &dto.BillingUsage{
			Source: dto.BillingUsageSourceClaudeMessages, Semantic: dto.BillingUsageSemanticAnthropic, ClaudeUsage: raw,
		}}
	}
	for _, tt := range []struct {
		name  string
		info  *relaycommon.RelayInfo
		usage *dto.Usage
		want  int64
	}{
		{name: "native snapshot precedes converted fields", usage: claude(&dto.ClaudeUsage{InputTokens: 2, CacheReadInputTokens: 589733, CacheCreationInputTokens: 1088}), want: 590823},
		{name: "split only", usage: claude(&dto.ClaudeUsage{InputTokens: 2, CacheReadInputTokens: 10, CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: 30, Ephemeral1hInputTokens: 40}}), want: 82},
		{name: "aggregate and split overlap", usage: claude(&dto.ClaudeUsage{InputTokens: 2, CacheReadInputTokens: 10, CacheCreationInputTokens: 70, CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: 30, Ephemeral1hInputTokens: 40}}), want: 82},
		{name: "aggregate greater than split", usage: claude(&dto.ClaudeUsage{InputTokens: 2, CacheReadInputTokens: 10, CacheCreationInputTokens: 90, CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: 30, Ephemeral1hInputTokens: 40}}), want: 102},
		{name: "legacy snapshot split", usage: claude(&dto.ClaudeUsage{InputTokens: 2, ClaudeCacheCreation5mTokens: 30, ClaudeCacheCreation1hTokens: 40}), want: 72},
		{name: "negative cache is not a discount", usage: claude(&dto.ClaudeUsage{InputTokens: 2, CacheReadInputTokens: -50, CacheCreationInputTokens: -70, CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: -30, Ephemeral1hInputTokens: 40}}), want: 42},
		{name: "explicit zero snapshot", usage: claude(&dto.ClaudeUsage{}), want: 0},
		{name: "huge fresh and cache cannot wrap", usage: claude(&dto.ClaudeUsage{InputTokens: math.MaxInt64, CacheReadInputTokens: 1}), want: math.MaxInt64},
		{name: "huge cache splits cannot wrap", usage: claude(&dto.ClaudeUsage{CacheCreation: &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: math.MaxInt64, Ephemeral1hInputTokens: 1}}), want: math.MaxInt64},
		{name: "inclusive OpenAI cache", usage: &dto.Usage{PromptTokens: 70397, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 67584}}, want: 70397},
		{name: "OpenAI snapshot wins over downstream Claude protocol", info: &relaycommon.RelayInfo{RelayFormat: relaytypes.RelayFormatClaude}, usage: &dto.Usage{BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{PromptTokens: 70397, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 67584}})}, want: 70397},
		{name: "responses input snapshot", usage: &dto.Usage{BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIResponses, OpenAIUsage: &dto.Usage{InputTokens: 99}}}, want: 99},
		{name: "Gemini tool input counted once", usage: &dto.Usage{BillingUsage: dto.NewGeminiChatBillingUsage(&dto.GeminiUsageMetadata{PromptTokenCount: 100, ToolUsePromptTokenCount: 20, CachedContentTokenCount: 90})}, want: 120},
		{name: "OpenRouter Anthropic tag remains inclusive", info: &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter}}, usage: &dto.Usage{PromptTokens: 1000, UsageSemantic: dto.BillingUsageSemanticAnthropic, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 800, CachedCreationTokens: 100}}, want: 1000},
		{name: "legacy Claude converted usage", info: &relaycommon.RelayInfo{RelayFormat: relaytypes.RelayFormatOpenAI}, usage: &dto.Usage{PromptTokens: 2, ClaudeCacheCreation5mTokens: 30, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100}}, want: 132},
		{name: "unmarked cache is not assumed Anthropic", usage: &dto.Usage{PromptTokens: 100, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 80}}, want: 100},
		{name: "native Claude without snapshot", info: &relaycommon.RelayInfo{RelayFormat: relaytypes.RelayFormatClaude}, usage: &dto.Usage{PromptTokens: 2, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 80}}, want: 82},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := *tt.usage
			before.BillingUsage = dto.CloneBillingUsage(tt.usage.BillingUsage)
			got := UsageInputTokensForStatistics(tt.info, tt.usage)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
			assert.Equal(t, before, *tt.usage, "raw counters and internal snapshot remain untouched")
		})
	}
	assert.Nil(t, UsageInputTokensForStatistics(nil, nil))
}
