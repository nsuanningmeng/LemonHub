package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type subscriptionSessionFixture struct {
	ctx     *gin.Context
	info    *relaycommon.RelayInfo
	session *BillingSession
	user    model.User
	token   model.Token
	sub     model.UserSubscription
}

func newSubscriptionSessionFixture(t *testing.T, preference string, total int64, forcePreConsume bool) subscriptionSessionFixture {
	t.Helper()
	truncate(t)
	resetShutdownRefundFailures(t)
	for _, record := range []interface{}{&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}} {
		if !model.DB.Migrator().HasTable(record) {
			require.NoError(t, model.DB.AutoMigrate(record))
		}
	}
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SubscriptionPreConsumeRecord{}).Error)
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.SubscriptionPlan{}).Error)
	})
	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	t.Cleanup(func() { common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch })
	// Service TestMain installs the shared database directly. Initialize the
	// model's dialect-specific quoted columns through its normal DB setup path.
	t.Setenv("LOG_SQL_DSN", "")
	previousLogDB := model.LOG_DB
	require.NoError(t, model.InitLogDB())
	t.Cleanup(func() { model.LOG_DB = previousLogDB })

	user := model.User{
		Username: "subscription-session-" + common.GetUUID(), SiteId: 17,
		Quota: 1000, Status: common.UserStatusEnabled, Group: "default",
		AffCode: "sub-session-" + common.GetRandomString(8),
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId: user.Id, SiteId: user.SiteId, Key: "sub-session-token-" + common.GetUUID(),
		RemainQuota: 1000, Status: common.TokenStatusEnabled, ExpiredTime: -1,
	}
	require.NoError(t, model.DB.Create(&token).Error)
	plan := model.SubscriptionPlan{
		Title: "Session plan", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: total, QuotaResetPeriod: model.SubscriptionResetNever,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	model.InvalidateSubscriptionPlanCache(plan.Id)
	now := model.GetDBTimestamp()
	sub := model.UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: total, AllowWalletOverflow: true,
		Status: "active", StartTime: now - 60, EndTime: now + 3600,
	}
	require.NoError(t, model.DB.Create(&sub).Error)
	info := &relaycommon.RelayInfo{
		RequestId: "subscription-session-request-" + common.GetUUID(), OriginModelName: "test-model",
		UserId: user.Id, UserQuota: 1000, TokenId: token.Id, TokenKey: token.Key,
		ForcePreConsume: forcePreConsume, UserSetting: dto.UserSetting{BillingPreference: preference},
	}
	ctx := taskBillingTestContext()
	ctx.Set("token_quota", 1000)
	session, apiErr := NewBillingSession(ctx, info, 60)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	info.Billing = session
	require.Equal(t, BillingSourceSubscription, info.BillingSource)
	return subscriptionSessionFixture{ctx: ctx, info: info, session: session, user: user, token: token, sub: sub}
}

func assertSubscriptionSessionBalances(t *testing.T, fixture subscriptionSessionFixture, subscriptionUsed int64, wallet, tokenRemaining, tokenUsed int) {
	t.Helper()
	assert.Equal(t, subscriptionUsed, getSubscriptionUsed(t, fixture.sub.Id))
	assert.Equal(t, wallet, getUserQuota(t, fixture.user.Id))
	assert.Equal(t, tokenRemaining, getTokenRemainQuota(t, fixture.token.Id))
	assert.Equal(t, tokenUsed, getTokenUsedQuota(t, fixture.token.Id))
}

func waitSubscriptionSessionRefund(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, WaitBillingRefunds(ctx))
}

func TestSubscriptionSessionSettlesOverflowAndLogsFundingSplit(t *testing.T) {
	fixture := newSubscriptionSessionFixture(t, "subscription_first", 100, false)
	assertSubscriptionSessionBalances(t, fixture, 60, 1000, 940, 60)
	require.NoError(t, fixture.session.Settle(160))
	assertSubscriptionSessionBalances(t, fixture, 100, 940, 840, 160)
	assert.False(t, fixture.session.NeedsRefund())

	other := make(map[string]interface{})
	appendBillingInfo(fixture.info, other)
	assert.Equal(t, BillingSourceSubscription, other["billing_source"])
	assert.Equal(t, "subscription_first", other["billing_preference"])
	assert.Equal(t, fixture.sub.Id, other["subscription_id"])
	assert.EqualValues(t, 60, other["subscription_pre_consumed"])
	assert.EqualValues(t, 40, other["subscription_post_delta"])
	assert.EqualValues(t, 100, other["subscription_consumed"])
	assert.EqualValues(t, 100, other["subscription_total"])
	assert.EqualValues(t, 100, other["subscription_used"])
	assert.EqualValues(t, 0, other["subscription_remain"])
	assert.EqualValues(t, 60, other["wallet_quota_deducted"])

	require.NoError(t, fixture.session.Settle(160))
	require.ErrorIs(t, fixture.session.Settle(161), model.ErrSubscriptionBillingConflict)
	fixture.session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	assertSubscriptionSessionBalances(t, fixture, 100, 940, 840, 160)
	repeated := make(map[string]interface{})
	appendBillingInfo(fixture.info, repeated)
	assert.Equal(t, other, repeated, "idempotent re-entry cannot alter the funding split reported to the user")
}

func TestSubscriptionSessionEnforcesOnlyAndForcedPreConsumePolicies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		preference string
		force      bool
	}{
		{name: "subscription only", preference: "subscription_only"},
		{name: "task forces preconsume", preference: "subscription_first", force: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSubscriptionSessionFixture(t, tc.preference, 100, tc.force)
			require.Error(t, fixture.session.Settle(160))
			assertSubscriptionSessionBalances(t, fixture, 60, 1000, 940, 60)
			assert.Zero(t, fixture.info.SubscriptionWalletQuota)
			assert.False(t, fixture.info.SubscriptionSettlementApplied)
			assert.True(t, fixture.session.NeedsRefund(), "a proven failed settlement still owns the original reservation")
			fixture.session.Refund(fixture.ctx)
			waitSubscriptionSessionRefund(t)
			fixture.session.Refund(fixture.ctx)
			waitSubscriptionSessionRefund(t)
			assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)
		})
	}
}

func TestSubscriptionSessionZeroSettlementReturnsEveryReservation(t *testing.T) {
	for _, reserveTarget := range []int{60, 120} {
		name := "initial reservation"
		if reserveTarget == 120 {
			name = "additional reservation"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newSubscriptionSessionFixture(t, "subscription_only", 200, false)
			if reserveTarget > 60 {
				require.NoError(t, fixture.session.Reserve(reserveTarget))
			}
			assert.Equal(t, reserveTarget, fixture.session.GetPreConsumedQuota())
			assertSubscriptionSessionBalances(t, fixture, int64(reserveTarget), 1000, 1000-reserveTarget, reserveTarget)
			require.NoError(t, fixture.session.Settle(0))
			require.NoError(t, fixture.session.Settle(0))
			fixture.session.Refund(fixture.ctx)
			waitSubscriptionSessionRefund(t)
			assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)

			other := make(map[string]interface{})
			appendBillingInfo(fixture.info, other)
			assert.EqualValues(t, reserveTarget, other["subscription_pre_consumed"])
			assert.EqualValues(t, -reserveTarget, other["subscription_post_delta"])
			assert.EqualValues(t, 0, other["subscription_consumed"])
			assert.EqualValues(t, 0, other["subscription_used"])
			assert.EqualValues(t, 200, other["subscription_remain"])
			assert.EqualValues(t, 0, other["wallet_quota_deducted"])
		})
	}
}

func TestSubscriptionSessionRefundsAdditionalReservationOnce(t *testing.T) {
	fixture := newSubscriptionSessionFixture(t, "subscription_only", 200, false)
	require.NoError(t, fixture.session.Reserve(120))
	fixture.session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	fixture.session.Refund(fixture.ctx)
	waitSubscriptionSessionRefund(t)
	assertSubscriptionSessionBalances(t, fixture, 0, 1000, 1000, 0)
}

func TestSubscriptionSessionTaskSettlementKeepsSingleFundingSource(t *testing.T) {
	fixture := newSubscriptionSessionFixture(t, "subscription_first", 100, true)
	task := makeTask(fixture.user.Id, 0, 60, fixture.token.Id, BillingSourceSubscription, fixture.sub.Id)
	task.TokenCharged = common.GetPointer(true)
	require.NoError(t, model.DB.Create(task).Error)

	RecalculateTaskQuota(context.Background(), task, 160, "subscription task exceeds remaining quota")
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, model.TaskBillingStatusManualReview, persisted.BillingStatus)
	assert.Equal(t, 60, persisted.Quota)
	assert.Equal(t, BillingSourceSubscription, persisted.PrivateData.BillingSource)
	assert.Equal(t, fixture.sub.Id, persisted.PrivateData.SubscriptionId)
	assertSubscriptionSessionBalances(t, fixture, 60, 1000, 940, 60)
	var stages int64
	require.NoError(t, model.DB.Model(&model.TaskBillingLedger{}).Where("task_record_id = ?", task.ID).Count(&stages).Error)
	assert.Zero(t, stages, "a task that cannot stay on its original funding source must not commit partial accounting")
}

func TestSubscriptionSessionReserveInsufficientQuotaPreservesHTTPContract(t *testing.T) {
	for _, tc := range []struct {
		name           string
		total          int64
		tokenRemaining int
		tokenUsed      int
		insufficient   error
		code           types.ErrorCode
	}{
		{name: "subscription limit", total: 100, tokenRemaining: 940, tokenUsed: 60,
			insufficient: model.ErrSubscriptionQuotaInsufficient, code: types.ErrorCodeInsufficientUserQuota},
		{name: "token limit", total: 200, tokenRemaining: 40, tokenUsed: 960,
			insufficient: model.ErrSubscriptionTokenQuotaInsufficient, code: types.ErrorCodePreConsumeTokenQuotaFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSubscriptionSessionFixture(t, "subscription_first", tc.total, false)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fixture.token.Id).
				Updates(map[string]interface{}{"remain_quota": tc.tokenRemaining, "used_quota": tc.tokenUsed}).Error)

			err := fixture.session.Reserve(120)
			require.ErrorIs(t, err, tc.insufficient)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
			assert.Equal(t, tc.code, apiErr.GetErrorCode())
			assert.True(t, types.IsSkipRetryError(apiErr))

			// A more expensive selected group asks the real routing boundary to
			// reserve 120; its outer error wrapper must preserve the original 403.
			fixture.info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
				BillingMode: "tiered_expr", GroupRatio: 1,
				EstimatedQuotaBeforeGroup: 60, EstimatedQuotaAfterGroup: 60,
			}
			fixture.info.PriceData.GroupRatioInfo.GroupRatio = 2
			routingErr := PrepareTieredBillingForSelectedGroup(fixture.ctx, fixture.info)
			require.NotNil(t, routingErr)
			assert.ErrorIs(t, routingErr, tc.insufficient)
			assert.Equal(t, http.StatusForbidden, routingErr.StatusCode)
			assert.Equal(t, tc.code, routingErr.GetErrorCode())
			assert.Equal(t, 60, fixture.session.GetPreConsumedQuota())
			assert.Equal(t, 60, fixture.info.FinalPreConsumedQuota)
			assertSubscriptionSessionBalances(t, fixture, 60, 1000, tc.tokenRemaining, tc.tokenUsed)
		})
	}
}

func TestSubscriptionSessionReserveDatabaseErrorsRemainServerErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "database failure", err: errors.New("database rejected reservation update")},
		{name: "outcome uncertain", err: errors.Join(model.ErrSubscriptionBillingUncertain, errors.New("transaction result unavailable"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSubscriptionSessionFixture(t, "subscription_first", 200, false)
			callbackName := "test:subscription_reserve_error_contract:" + common.GetUUID()
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == "user_subscriptions" {
					tx.AddError(tc.err)
				}
			}))
			t.Cleanup(func() { model.DB.Callback().Update().Remove(callbackName) })

			err := fixture.session.Reserve(120)
			require.ErrorIs(t, err, tc.err)
			var apiErr *types.NewAPIError
			assert.False(t, errors.As(err, &apiErr), "persistence errors must not be classified as insufficient balance")
			fixture.info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
				BillingMode: "tiered_expr", GroupRatio: 1,
				EstimatedQuotaBeforeGroup: 60, EstimatedQuotaAfterGroup: 60,
			}
			fixture.info.PriceData.GroupRatioInfo.GroupRatio = 2
			routingErr := PrepareTieredBillingForSelectedGroup(fixture.ctx, fixture.info)
			require.NotNil(t, routingErr)
			assert.ErrorIs(t, routingErr, tc.err)
			assert.Equal(t, http.StatusInternalServerError, routingErr.StatusCode)
			assert.Equal(t, types.ErrorCodeUpdateDataError, routingErr.GetErrorCode())
			assert.Equal(t, 60, fixture.session.GetPreConsumedQuota())
			assert.Equal(t, 60, fixture.info.FinalPreConsumedQuota)
			assertSubscriptionSessionBalances(t, fixture, 60, 1000, 940, 60)
		})
	}
}
