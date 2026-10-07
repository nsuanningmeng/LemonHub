package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiCachedAudioSettlementAndLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousQuotaPerUnit, previousExport := common.QuotaPerUnit, common.DataExportEnabled
	common.QuotaPerUnit, common.DataExportEnabled = 500000, false
	t.Cleanup(func() { common.QuotaPerUnit, common.DataExportEnabled = previousQuotaPerUnit, previousExport })
	for _, tt := range []struct {
		name        string
		cachedAudio int
		quota       int
		audioQuota  int
	}{
		{name: "partially cached audio", cachedAudio: 100, quota: 108, audioQuota: 90},
		{name: "fully cached audio", cachedAudio: 200, quota: 36, audioQuota: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			const id, balance = 7229, 1000000
			seedUser(t, id, balance)
			seedToken(t, id, id, "gemini-cached-audio", balance)
			seedChannel(t, id)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-flash:generateContent", nil)
			info := &relaycommon.RelayInfo{
				UserId: id, UserQuota: balance, TokenId: id, TokenKey: "gemini-cached-audio",
				OriginModelName: "gemini-2.5-flash", UsingGroup: "default", StartTime: time.Now(),
				RelayFormat: relaytypes.RelayFormatGemini, FinalRequestRelayFormat: relaytypes.RelayFormatGemini,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: id, ChannelType: constant.ChannelTypeGemini},
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1.5}},
				BillingSource: BillingSourceWallet,
			}
			info.PriceData.AddOtherRatio("site_markup", 1.2)
			info.Billing = &BillingSession{relayInfo: info, funding: &WalletFunding{userId: id}}
			metadata := dto.GeminiUsageMetadata{
				PromptTokenCount: 200, CachedContentTokenCount: tt.cachedAudio,
				PromptTokensDetails: []dto.GeminiPromptTokensDetails{{Modality: "AUDIO", TokenCount: 200}},
				CacheTokensDetails:  []dto.GeminiPromptTokensDetails{{Modality: "AUDIO", TokenCount: tt.cachedAudio}},
			}
			PostTextConsumeQuota(ctx, info, &dto.Usage{BillingUsage: dto.NewGeminiChatBillingUsage(&metadata)}, nil)

			var consumeLog model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ?", id).First(&consumeLog).Error)
			assert.Equal(t, tt.quota, consumeLog.Quota)
			assert.Equal(t, 200, consumeLog.PromptTokens)
			assert.Contains(t, consumeLog.Content, "Audio Input 花费 "+logger.LogQuota(tt.audioQuota))
			var other map[string]interface{}
			require.NoError(t, common.UnmarshalJsonStr(consumeLog.Other, &other))
			assert.Equal(t, float64(200), other["audio_input_token_count"], "raw inclusive media counts stay in logs")
			assert.Equal(t, float64(tt.cachedAudio), other["cache_tokens"])
			var user model.User
			require.NoError(t, model.DB.First(&user, id).Error)
			assert.Equal(t, balance-tt.quota, user.Quota)
			assert.Equal(t, tt.quota, user.UsedQuota)
			var token model.Token
			require.NoError(t, model.DB.First(&token, id).Error)
			assert.Equal(t, balance-tt.quota, token.RemainQuota)
			assert.Equal(t, tt.quota, token.UsedQuota)
		})
	}
}

func TestGeminiCachedMediaRatioBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	tests := []struct {
		name     string
		metadata string
		quota    int
	}{
		{
			name: "fully cached image without modality breakdown",
			metadata: `{"promptTokenCount":100,"cachedContentTokenCount":100,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":100}]}`,
			quota: 10,
		},
		{
			name: "partial image cache uses exact intersection",
			metadata: `{"promptTokenCount":1000,"cachedContentTokenCount":400,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600},{"modality":"TEXT","tokenCount":400}],
				"cacheTokensDetails":[{"modality":"IMAGE","tokenCount":300},{"modality":"TEXT","tokenCount":100}]}`,
			quota: 940, // 300 fresh text + 300 fresh images * 2 + 400 cache * 0.1.
		},
		{
			name: "exact media cache remains usable when prompt text details are omitted",
			metadata: `{"promptTokenCount":1000,"cachedContentTokenCount":400,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600}],
				"cacheTokensDetails":[{"modality":"IMAGE","tokenCount":300},{"modality":"TEXT","tokenCount":100}]}`,
			quota: 940,
		},
		{
			name: "partial cache without details is not assigned to images",
			metadata: `{"promptTokenCount":1000,"cachedContentTokenCount":400,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600},{"modality":"TEXT","tokenCount":400}]}`,
			quota: 1240,
		},
		{
			name: "cached text does not discount fresh images",
			metadata: `{"promptTokenCount":1000,"cachedContentTokenCount":400,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600},{"modality":"TEXT","tokenCount":400}],
				"cacheTokensDetails":[{"modality":"TEXT","tokenCount":400}]}`,
			quota: 1240,
		},
		{
			name: "fully cached prompt keeps tool use media fresh",
			metadata: `{"promptTokenCount":100,"toolUsePromptTokenCount":50,"cachedContentTokenCount":100,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":100}],
				"toolUsePromptTokensDetails":[{"modality":"IMAGE","tokenCount":30},{"modality":"TEXT","tokenCount":20}]}`,
			quota: 90,
		},
		{
			name: "cached images and audio are each billed once",
			metadata: `{"promptTokenCount":1000,"cachedContentTokenCount":400,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600},{"modality":"AUDIO","tokenCount":200},{"modality":"TEXT","tokenCount":200}],
				"cacheTokensDetails":[{"modality":"IMAGE","tokenCount":300},{"modality":"AUDIO","tokenCount":100}]}`,
			quota: 890, // 200 text + 300 images * 2 + 100 audio * 0.5 + 400 cache * 0.1.
		},
		{
			name:     "uncached images keep their full image price",
			metadata: `{"promptTokenCount":100,"promptTokensDetails":[{"modality":"IMAGE","tokenCount":100}]}`,
			quota:    200,
		},
	}
	for _, tt := range tests {
		for _, format := range []relaytypes.RelayFormat{relaytypes.RelayFormatGemini, relaytypes.RelayFormatOpenAI} {
			t.Run(tt.name+"/"+string(format), func(t *testing.T) {
				var metadata dto.GeminiUsageMetadata
				require.NoError(t, common.UnmarshalJsonStr(tt.metadata, &metadata))
				origin := &dto.Usage{BillingUsage: dto.NewGeminiChatBillingUsage(&metadata)}
				if format == relaytypes.RelayFormatOpenAI {
					origin = relayconvert.UsageFromGeminiMetadata(&metadata, 0)
				}
				usage := effectiveBillingUsage(origin)
				before, err := common.Marshal(usage)
				require.NoError(t, err)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{
					RelayFormat: format, OriginModelName: "gemini-2.5-flash", StartTime: time.Now(),
					PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, ImageRatio: 2, CacheRatio: 0.1,
						GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
				}
				summary := calculateTextQuotaSummary(ctx, info, usage)
				assert.Equal(t, tt.quota, summary.Quota)
				assert.Equal(t, usage.PromptTokensDetails.ImageTokens, summary.ImageTokens, "logs retain upstream inclusive image counts")
				assert.Equal(t, usage.PromptTokensDetails.AudioTokens, summary.AudioTokens)
				after, err := common.Marshal(usage)
				require.NoError(t, err)
				assert.JSONEq(t, string(before), string(after), "settlement must not mutate usage or its snapshot")
				info.PriceData.GroupRatioInfo.GroupRatio = 1.5
				info.PriceData.AddOtherRatio("site_markup", 1.2)
				assert.Equal(t, tt.quota*18/10, calculateTextQuotaSummary(ctx, info, usage).Quota)
			})
		}
	}
}

func TestGeminiCachedMediaTieredVariableOptIn(t *testing.T) {
	var metadata dto.GeminiUsageMetadata
	require.NoError(t, common.UnmarshalJsonStr(`{"promptTokenCount":1000,"cachedContentTokenCount":400,
		"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600},{"modality":"AUDIO","tokenCount":200},{"modality":"TEXT","tokenCount":200}],
		"cacheTokensDetails":[{"modality":"IMAGE","tokenCount":300},{"modality":"AUDIO","tokenCount":100}]}`, &metadata))
	usage := effectiveBillingUsage(relayconvert.UsageFromGeminiMetadata(&metadata, 0))
	tests := []struct {
		expr          string
		p, img, audio float64
	}{
		{`tier("base", p)`, 1000, 600, 200},
		{`tier("base", p + cr)`, 600, 300, 100},
		{`tier("base", p + img)`, 400, 600, 200},
		{`tier("base", p + ai)`, 800, 600, 200},
		{`tier("base", p + img + ai)`, 200, 600, 200},
		{`tier("base", p + cr + img)`, 300, 300, 100},
		{`tier("base", p + cr + ai)`, 500, 300, 100},
		{`tier("base", p + cr + img + ai)`, 200, 300, 100},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(tt.expr))
			assert.Equal(t, tt.p, params.P)
			assert.Equal(t, tt.img, params.Img)
			assert.Equal(t, tt.audio, params.AI)
			assert.Equal(t, float64(400), params.CR)
			assert.Equal(t, float64(1000), params.Len)
			cost, _, err := billingexpr.RunExpr(tt.expr, params)
			require.NoError(t, err)
			assert.Equal(t, float64(1000), cost, "unit prices must bill every token exactly once")
		})
	}
}

func TestGeminiCachedMediaRejectsConflictingPromptMetadata(t *testing.T) {
	tests := []struct {
		name                                 string
		prompt, cached, image, text, toolUse int
	}{
		{name: "negative cache", prompt: 100, cached: -1, image: 60, text: 40},
		{name: "cache exceeds prompt", prompt: 100, cached: 101, image: 60, text: 40},
		{name: "negative prompt", prompt: -100, cached: 100, image: 60, text: 40},
		{name: "negative modality", prompt: 100, cached: 100, image: 60, text: -1},
		{name: "modality sum exceeds prompt", prompt: 100, cached: 100, image: 60, text: 41},
		{name: "negative tool use", prompt: 100, cached: 100, image: 60, text: 40, toolUse: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := effectiveBillingUsage(&dto.Usage{BillingUsage: dto.NewGeminiChatBillingUsage(&dto.GeminiUsageMetadata{
				PromptTokenCount: tt.prompt, CachedContentTokenCount: tt.cached, ToolUsePromptTokenCount: tt.toolUse,
				PromptTokensDetails: []dto.GeminiPromptTokensDetails{{Modality: "IMAGE", TokenCount: tt.image}, {Modality: "TEXT", TokenCount: tt.text}},
			})})
			params := BuildTieredTokenParams(usage, false, map[string]bool{"cr": true, "img": true})
			assert.Equal(t, float64(60), params.Img, "invalid totals cannot justify a media discount")
			assert.GreaterOrEqual(t, params.P, float64(0))
		})
	}
}

func TestGeminiInvalidCachedMediaDoesNotDiscountFreshTokens(t *testing.T) {
	tests := []string{
		`{"modality":"IMAGE","tokenCount":-1}`,
		`{"modality":"IMAGE","tokenCount":401}`,
		`{"modality":"IMAGE","tokenCount":9223372036854775807}`,
		`{"modality":"IMAGE","tokenCount":300},{"modality":"AUDIO","tokenCount":101}`,
		`{"modality":"AUDIO","tokenCount":201}`,
		`{"modality":"IMAGE","tokenCount":100},{"modality":"TEXT","tokenCount":201}`,
		`{"modality":"IMAGE","tokenCount":100},{"modality":"VIDEO","tokenCount":1}`,
		`{"modality":"AUDIO","tokenCount":101},{"modality":"AUDIO","tokenCount":100}`,
	}
	for _, details := range tests {
		t.Run(details, func(t *testing.T) {
			var metadata dto.GeminiUsageMetadata
			require.NoError(t, common.UnmarshalJsonStr(`{"promptTokenCount":1000,"cachedContentTokenCount":400,
				"promptTokensDetails":[{"modality":"IMAGE","tokenCount":600},{"modality":"AUDIO","tokenCount":200},{"modality":"TEXT","tokenCount":200}],
				"cacheTokensDetails":[`+details+`]}`, &metadata))
			usage := effectiveBillingUsage(relayconvert.UsageFromGeminiMetadata(&metadata, 0))
			params := BuildTieredTokenParams(usage, false, map[string]bool{"cr": true, "img": true, "ai": true})
			assert.Equal(t, float64(600), params.Img)
			assert.Equal(t, float64(200), params.AI)
			assert.GreaterOrEqual(t, params.P, float64(0))
		})
	}
}
