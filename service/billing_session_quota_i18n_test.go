package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertB19QuotaErrorContract(t *testing.T, err *types.NewAPIError, legacy, translated string) {
	t.Helper()
	require.NotNil(t, err)
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, err.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(err))
	assert.False(t, types.IsRecordErrorLog(err))
	assert.Contains(t, err.Error(), legacy, "old downstream keyword matching must still see the complete diagnostic")
	want := legacy
	if translated != "" {
		want = translated + " (" + legacy + ")"
	}
	assert.Equal(t, want, err.Error())
	assert.Equal(t, want, err.ToOpenAIError().Message)
}

func TestB19WalletQuotaMessagesPreserveAmountsAndLanguageSelection(t *testing.T) {
	for _, tc := range []struct {
		name, language string
		quota          int
		translation    string
	}{
		{name: "empty wallet English", language: "en-US", translation: "Insufficient user quota, remaining quota: %s"},
		{name: "empty wallet Simplified Chinese", language: "zh-CN"},
		{name: "empty wallet Traditional Chinese", language: "zh-TW", translation: "使用者額度不足, 剩餘額度: %s"},
		{name: "preconsume English", language: "en", quota: 5, translation: "Quota pre-consumption failed, remaining user quota: %s, required quota: %s"},
		{name: "preconsume Simplified Chinese", language: "zh-CN", quota: 5},
		{name: "preconsume Traditional Chinese", language: "zh-TW", quota: 5, translation: "預扣費額度失敗, 使用者剩餘額度: %s, 需要預扣費額度: %s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const userID, tokenID = 19001, 19001
			seedUser(t, userID, tc.quota)
			seedToken(t, tokenID, userID, "b19-wallet-token", 1000)
			c := taskBillingTestContext()
			c.Request.Header.Set("Accept-Language", tc.language)
			info := &relaycommon.RelayInfo{
				UserId: userID, TokenId: tokenID, TokenKey: "b19-wallet-token", ForcePreConsume: true,
				UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
			}
			session, err := NewBillingSession(c, info, 10)
			require.Nil(t, session)
			legacy := fmt.Sprintf("用户额度不足, 剩余额度: %s", logger.FormatQuota(tc.quota))
			translated := tc.translation
			if tc.quota > 0 {
				legacy = fmt.Sprintf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(tc.quota), logger.FormatQuota(10))
				if translated != "" {
					translated = fmt.Sprintf(translated, logger.FormatQuota(tc.quota), logger.FormatQuota(10))
				}
			} else if translated != "" {
				translated = fmt.Sprintf(translated, logger.FormatQuota(tc.quota))
			}
			assertB19QuotaErrorContract(t, err, legacy, translated)
			assert.Equal(t, tc.quota, getUserQuota(t, userID))
			assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
			assert.Zero(t, getTokenUsedQuota(t, tokenID))
		})
	}
}

func TestB19AtomicWalletFailureRollsBackTokenAndKeepsLocalizedDiagnostic(t *testing.T) {
	truncate(t)
	const userID, tokenID = 19002, 19002
	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "b19-atomic-wallet", 1000)
	c := taskBillingTestContext()
	c.Request.Header.Set("Accept-Language", "en")
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId: userID, TokenId: tokenID, TokenKey: "b19-atomic-wallet", ForcePreConsume: true,
		},
		funding: &WalletFunding{userId: userID},
	}
	err := session.preConsume(c, 10)
	assertB19QuotaErrorContract(t, err,
		fmt.Sprintf("用户额度不足, 剩余额度: %s", logger.FormatQuota(0)),
		fmt.Sprintf("Insufficient user quota, remaining quota: %s", logger.FormatQuota(0)))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	assert.Zero(t, getUserQuota(t, userID))
	assert.False(t, session.NeedsRefund())
	session.Refund(c)
	waitSubscriptionSessionRefund(t)
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID), "a failed pre-consume cannot be credited twice")
}

func TestB19SubscriptionReservationKeepsRequestLanguageAndTypedCause(t *testing.T) {
	fixture := newSubscriptionSessionFixture(t, "subscription_only", 100, true)
	fixture.session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)

	fixture.info.RequestId = "b19-reserve-" + common.GetUUID()
	common.SetContextKey(fixture.ctx, constant.ContextKeyUserSetting, dto.UserSetting{Language: "zh-TW"})
	session, err := NewBillingSession(fixture.ctx, fixture.info, 60)
	require.Nil(t, err)
	require.NotNil(t, session)
	// Reservation has no gin.Context and must keep the language selected when
	// this request created the session, even if a caller later reuses its context.
	common.SetContextKey(fixture.ctx, constant.ContextKeyUserSetting, dto.UserSetting{Language: "en"})
	reserveErr := session.Reserve(120)
	require.ErrorIs(t, reserveErr, model.ErrSubscriptionQuotaInsufficient)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, reserveErr, &apiErr)
	assertB19QuotaErrorContract(t, apiErr,
		"订阅额度不足或未配置订阅: subscription quota insufficient: subscription used exceeds total, used=60 total=100",
		"訂閱額度不足或未設定訂閱: subscription quota insufficient: subscription used exceeds total, used=60 total=100")
	assert.Equal(t, 60, session.GetPreConsumedQuota())
	assertSubscriptionSessionBalances(t, fixture, 60, 1000, 940, 60)
	session.Refund(fixture.ctx)
	session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)
	var receipt model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", fixture.info.RequestId).First(&receipt).Error)
	assert.Equal(t, "refunded", receipt.Status)
}

func TestB19LegacySubscriptionFundingReservationKeepsErrorContract(t *testing.T) {
	fixture := newSubscriptionSessionFixture(t, "subscription_only", 100, true)
	err := fixture.session.reserveFunding(60)
	require.Error(t, err)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Contains(t, apiErr.Error(), "Insufficient subscription quota or no active subscription:")
	assert.Contains(t, apiErr.Error(), "订阅额度不足或未配置订阅:")
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.False(t, types.IsRecordErrorLog(apiErr))
	assertSubscriptionSessionBalances(t, fixture, 60, 1000, 940, 60)
	fixture.session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)
}
