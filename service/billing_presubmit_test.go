package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPreConsumeBillingRetryReservesBeforeSubmitAndRefundsEveryHold(t *testing.T) {
	truncate(t)
	resetShutdownRefundFailures(t)
	seedUser(t, 830, 500)
	seedToken(t, 831, 830, "retry-token", 500)
	info := &relaycommon.RelayInfo{UserId: 830, TokenId: 831, TokenKey: "retry-token", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	ctx := taskBillingTestContext()
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	originalSession := info.Billing
	require.Nil(t, PreConsumeBilling(ctx, 250, info))
	require.Same(t, originalSession, info.Billing)
	assert.Equal(t, 250, getUserQuota(t, 830))
	assert.Equal(t, 250, getTokenRemainQuota(t, 831))
	require.Nil(t, PreConsumeBilling(ctx, 150, info))
	assert.Equal(t, 250, info.Billing.GetPreConsumedQuota(), "a cheaper retry waits for settlement to refund")
	info.Billing.Refund(ctx)
	require.NoError(t, WaitBillingRefunds(context.Background()))
	assert.Equal(t, 500, getUserQuota(t, 830))
	assert.Equal(t, 500, getTokenRemainQuota(t, 831))
	info.Billing.Refund(ctx)
	require.NoError(t, WaitBillingRefunds(context.Background()))
	assert.Equal(t, 500, getUserQuota(t, 830), "refund remains idempotent")
}

func TestPreConsumeBillingRetryRejectsInsufficientWalletWithoutChangingHold(t *testing.T) {
	truncate(t)
	seedUser(t, 832, 180)
	info := &relaycommon.RelayInfo{UserId: 832, IsPlayground: true, ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	ctx := taskBillingTestContext()
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	apiErr := PreConsumeBilling(ctx, 250, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, 80, getUserQuota(t, 832))
	assert.Equal(t, 100, info.Billing.GetPreConsumedQuota())
}

func TestPreConsumeBillingRetryTokenRejectionRollsBackOnlyTopUp(t *testing.T) {
	truncate(t)
	resetShutdownRefundFailures(t)
	seedUser(t, 833, 500)
	seedToken(t, 834, 833, "limited-retry-token", 180)
	info := &relaycommon.RelayInfo{UserId: 833, TokenId: 834, TokenKey: "limited-retry-token", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	ctx := taskBillingTestContext()
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	apiErr := PreConsumeBilling(ctx, 250, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodePreConsumeTokenQuotaFailed, apiErr.GetErrorCode())
	assert.Equal(t, 400, getUserQuota(t, 833))
	assert.Equal(t, 80, getTokenRemainQuota(t, 834))
	assert.Equal(t, 100, info.Billing.GetPreConsumedQuota())
	info.Billing.Refund(ctx)
	require.NoError(t, WaitBillingRefunds(context.Background()))
	assert.Equal(t, 500, getUserQuota(t, 833))
	assert.Equal(t, 180, getTokenRemainQuota(t, 834))
}

func TestPreConsumeBillingRetrySubscriptionPreservesReceiptAndRejectsOverflow(t *testing.T) {
	fixture := newSubscriptionSessionFixture(t, "subscription_only", 200, true)
	require.Nil(t, PreConsumeBilling(fixture.ctx, 150, fixture.info))
	assertSubscriptionSessionBalances(t, fixture, 150, 1000, 850, 150)
	apiErr := PreConsumeBilling(fixture.ctx, 250, fixture.info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assertSubscriptionSessionBalances(t, fixture, 150, 1000, 850, 150)
	fixture.session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)
}

func TestReserveBeforeSubmitRejectsOversizedQuotaWithoutBilling(t *testing.T) {
	truncate(t)
	seedUser(t, 835, 500)
	info := &relaycommon.RelayInfo{UserId: 835, IsPlayground: true, ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	require.Nil(t, PreConsumeBilling(taskBillingTestContext(), 100, info))
	require.Error(t, info.Billing.ReserveBeforeSubmit(common.MaxQuota+1))
	assert.Equal(t, 400, getUserQuota(t, 835))
	assert.Equal(t, 100, info.Billing.GetPreConsumedQuota())
}

func TestPlaygroundRetryRefundsWalletWithoutToken(t *testing.T) {
	truncate(t)
	resetShutdownRefundFailures(t)
	seedUser(t, 836, 500)
	info := &relaycommon.RelayInfo{UserId: 836, IsPlayground: true, ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	ctx := taskBillingTestContext()
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	require.Nil(t, PreConsumeBilling(ctx, 250, info))
	require.True(t, info.Billing.NeedsRefund())
	info.Billing.Refund(ctx)
	require.NoError(t, WaitBillingRefunds(context.Background()))
	assert.Equal(t, 500, getUserQuota(t, 836))
}

func TestRetryRollbackFailureFencesAutomaticRefundAndFurtherReservations(t *testing.T) {
	truncate(t)
	seedUser(t, 837, 500)
	seedToken(t, 838, 837, "rollback-uncertain-token", 180)
	info := &relaycommon.RelayInfo{UserId: 837, TokenId: 838, TokenKey: "rollback-uncertain-token", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	ctx := taskBillingTestContext()
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER fail_retry_wallet_refund BEFORE UPDATE ON users
 WHEN OLD.id=837 AND NEW.quota>OLD.quota BEGIN SELECT RAISE(ABORT, 'forced rollback failure'); END;`).Error)
	t.Cleanup(func() { model.DB.Exec("DROP TRIGGER IF EXISTS fail_retry_wallet_refund") })
	apiErr := PreConsumeBilling(ctx, 250, info)
	require.NotNil(t, apiErr)
	session := info.Billing.(*BillingSession)
	assert.True(t, session.fundingUncertain)
	snapshot := session.taskSubmissionBillingSnapshot(250, apiErr.Error())
	assert.Equal(t, 250, snapshot.FundingQuota)
	assert.Equal(t, 100, snapshot.TokenQuota)
	assert.True(t, snapshot.FundingUncertain)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_retry_wallet_refund").Error)
	session.Refund(ctx)
	require.Error(t, session.ReserveBeforeSubmit(300))
	require.Error(t, session.Settle(100))
	assert.Equal(t, 250, getUserQuota(t, 837))
	assert.Equal(t, 80, getTokenRemainQuota(t, 838))
	assert.False(t, session.refunded)
}

func TestRetryCommittedPersistenceErrorFencesUnknownFundingAndToken(t *testing.T) {
	for _, tc := range []struct {
		name, table                      string
		wallet, token                    int
		fundingUncertain, tokenUncertain bool
		postSubmit                       bool
	}{
		{"wallet commit response lost", "users", 250, 400, true, false, false},
		{"token commit response lost", "tokens", 400, 250, false, true, false},
		{"post-submit wallet commit response lost", "users", 250, 400, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 850, 500)
			seedToken(t, 851, 850, "uncertain-commit-token", 500)
			info := &relaycommon.RelayInfo{UserId: 850, TokenId: 851, TokenKey: "uncertain-commit-token", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
			ctx := taskBillingTestContext()
			require.Nil(t, PreConsumeBilling(ctx, 100, info))
			injected := errors.New("committed but response lost")
			const callback = "test:retry-uncertain-after-commit"
			require.NoError(t, model.DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == tc.table {
					tx.AddError(injected)
				}
			}))
			t.Cleanup(func() { model.DB.Callback().Update().Remove(callback) })
			var apiErr *types.NewAPIError
			if tc.postSubmit {
				require.ErrorAs(t, info.Billing.Reserve(250), &apiErr)
			} else {
				apiErr = PreConsumeBilling(ctx, 250, info)
			}
			require.NotNil(t, apiErr)
			assert.Equal(t, types.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
			require.NoError(t, model.DB.Callback().Update().Remove(callback))
			session := info.Billing.(*BillingSession)
			assert.Equal(t, tc.fundingUncertain, session.fundingUncertain)
			assert.Equal(t, tc.tokenUncertain, session.tokenUncertain)
			assert.Equal(t, tc.wallet, getUserQuota(t, 850))
			assert.Equal(t, tc.token, getTokenRemainQuota(t, 851))
			session.Refund(ctx)
			require.ErrorIs(t, session.ReserveBeforeSubmit(300), injected)
			require.ErrorIs(t, session.Settle(100), injected)
			assert.Equal(t, tc.wallet, getUserQuota(t, 850))
			assert.Equal(t, tc.token, getTokenRemainQuota(t, 851))
		})
	}
}
