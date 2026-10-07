package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type subscriptionSettlementFixture struct {
	user   User
	token  Token
	sub    UserSubscription
	params SubscriptionBillingParams
}

func newSubscriptionSettlementFixture(t *testing.T, total int64, preConsumed int) subscriptionSettlementFixture {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	t.Cleanup(func() {
		require.NoError(t, DB.Where("1 = 1").Delete(&SubscriptionPreConsumeRecord{}).Error)
	})
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	user := createReserveTestUser(t, 1000)
	user.SiteId = 17
	require.NoError(t, DB.Model(&user).Update("site_id", user.SiteId).Error)
	plan := SubscriptionPlan{
		Title: "settlement-plan", DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		Enabled: true, TotalAmount: total, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	now := GetDBTimestamp()
	sub := UserSubscription{
		UserId: user.Id, PlanId: plan.Id, AmountTotal: total,
		StartTime: now - 60, EndTime: now + 3600, Status: "active", AllowWalletOverflow: true,
	}
	require.NoError(t, DB.Create(&sub).Error)
	requestID := "settlement-" + common.GetUUID()
	preConsumedResult, err := PreConsumeUserSubscription(requestID, user.Id, "test-model", 0, int64(preConsumed))
	require.NoError(t, err)
	require.Equal(t, sub.Id, preConsumedResult.UserSubscriptionId)
	token := Token{
		UserId: user.Id, SiteId: user.SiteId, Key: "settlement-token-" + common.GetUUID(),
		Status: common.TokenStatusEnabled, ExpiredTime: -1,
		RemainQuota: 1000 - preConsumed, UsedQuota: preConsumed,
	}
	require.NoError(t, DB.Create(&token).Error)
	return subscriptionSettlementFixture{
		user: user, token: token, sub: sub,
		params: SubscriptionBillingParams{
			RequestId: requestID, UserId: user.Id, SubscriptionId: sub.Id,
			TokenId: token.Id, TokenKey: token.Key,
			PreConsumedQuota: preConsumed, TokenConsumedQuota: preConsumed,
			ActualQuota: 160, AllowWalletOverflow: true,
		},
	}
}

func assertSubscriptionSettlementBalances(t *testing.T, fixture subscriptionSettlementFixture, used int64, wallet, tokenRemain, tokenUsed int) {
	t.Helper()
	var sub UserSubscription
	require.NoError(t, DB.First(&sub, fixture.sub.Id).Error)
	assert.Equal(t, used, sub.AmountUsed, "subscription consumption")
	assert.Equal(t, wallet, getUserQuotaFromDB(t, fixture.user.Id), "wallet balance")
	token := getTokenFromDB(t, fixture.token.Id)
	assert.Equal(t, tokenRemain, token.RemainQuota, "token balance")
	assert.Equal(t, tokenUsed, token.UsedQuota, "token consumption")
}

func TestSettleSubscriptionBillingSplitsOverflowAndIsIdempotent(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)

	receipt, err := SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.EqualValues(t, 40, receipt.SubscriptionDelta)
	assert.EqualValues(t, 60, receipt.WalletDelta)
	assert.Equal(t, 100, receipt.TokenDelta)
	assert.Equal(t, 160, receipt.ActualQuota)
	assert.EqualValues(t, 100, receipt.SubscriptionConsumed)
	assert.EqualValues(t, 60, receipt.WalletConsumed)
	assert.EqualValues(t, 100, receipt.SubscriptionUsedAfter)
	assert.EqualValues(t, 100, receipt.SubscriptionTotal)
	assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)

	repeated, err := SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	assert.Equal(t, receipt, repeated, "a retry returns the original funding split")
	assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)

	conflicting := fixture.params
	conflicting.ActualQuota++
	_, err = SettleSubscriptionBilling(conflicting)
	require.Error(t, err, "the same request cannot settle to another actual quota")
	require.NoError(t, RefundSubscriptionBilling(fixture.params), "a deferred refund cannot undo a successful settlement")
	assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
}

func TestSettleSubscriptionBillingHonorsOverflowPolicy(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requestAllows  bool
		selectedAllows bool
		otherStatus    string
		otherExpired   bool
		wantError      bool
	}{
		{name: "request disallows", selectedAllows: true, wantError: true},
		{name: "selected subscription disallows", requestAllows: true, wantError: true},
		{name: "another active subscription disallows", requestAllows: true, selectedAllows: true, otherStatus: "active", wantError: true},
		{name: "expired strict subscription does not block", requestAllows: true, selectedAllows: true, otherStatus: "active", otherExpired: true},
		{name: "cancelled strict subscription does not block", requestAllows: true, selectedAllows: true, otherStatus: "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			fixture.params.AllowWalletOverflow = tc.requestAllows
			require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", fixture.sub.Id).
				Update("allow_wallet_overflow", tc.selectedAllows).Error)
			if tc.otherStatus != "" {
				other := fixture.sub
				other.Id = 0
				other.Status = tc.otherStatus
				other.AllowWalletOverflow = false
				if tc.otherExpired {
					other.EndTime = GetDBTimestamp() - 1
				}
				require.NoError(t, DB.Create(&other).Error)
			}

			_, err := SettleSubscriptionBilling(fixture.params)
			if tc.wantError {
				require.Error(t, err)
				assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
				return
			}
			require.NoError(t, err)
			assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
		})
	}
}

func TestSettleSubscriptionBillingPreservesUnlimitedAndWalletDebtSemantics(t *testing.T) {
	t.Run("unlimited subscription", func(t *testing.T) {
		fixture := newSubscriptionSettlementFixture(t, 0, 60)
		fixture.params.AllowWalletOverflow = false
		receipt, err := SettleSubscriptionBilling(fixture.params)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		assert.EqualValues(t, 100, receipt.SubscriptionDelta)
		assert.Zero(t, receipt.WalletDelta)
		assertSubscriptionSettlementBalances(t, fixture, 160, 1000, 840, 160)
	})
	t.Run("final usage may make wallet negative", func(t *testing.T) {
		fixture := newSubscriptionSettlementFixture(t, 100, 60)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", fixture.user.Id).Update("quota", 20).Error)
		_, err := SettleSubscriptionBilling(fixture.params)
		require.NoError(t, err)
		assertSubscriptionSettlementBalances(t, fixture, 100, -40, 840, 160)
	})
}

func TestSubscriptionBillingRejectsOwnerAndRequestBindingMismatch(t *testing.T) {
	for _, mismatch := range []string{"request user", "request subscription", "subscription owner", "token owner", "token key", "token site", "preconsume amount"} {
		t.Run(mismatch, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			otherUser := createReserveTestUser(t, 500)
			switch mismatch {
			case "request user":
				fixture.params.UserId = otherUser.Id
			case "request subscription":
				other := fixture.sub
				other.Id = 0
				require.NoError(t, DB.Create(&other).Error)
				fixture.params.SubscriptionId = other.Id
			case "subscription owner":
				require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", fixture.sub.Id).Update("user_id", otherUser.Id).Error)
			case "token owner":
				require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).Update("user_id", otherUser.Id).Error)
			case "token key":
				fixture.params.TokenKey = "not-the-owned-token"
			case "token site":
				require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).Update("site_id", fixture.user.SiteId+1).Error)
			case "preconsume amount":
				fixture.params.PreConsumedQuota++
			}

			_, err := SettleSubscriptionBilling(fixture.params)
			require.Error(t, err)
			require.Error(t, RefundSubscriptionBilling(fixture.params))
			assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
			assert.Equal(t, 500, getUserQuotaFromDB(t, otherUser.Id))
		})
	}
}

func TestSettleSubscriptionBillingZeroReturnsPreConsumeOnlyOnce(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	fixture.params.ActualQuota = 0
	receipt, err := SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.EqualValues(t, -60, receipt.SubscriptionDelta)
	assert.Zero(t, receipt.WalletDelta)
	assert.Equal(t, -60, receipt.TokenDelta)
	assert.Zero(t, receipt.SubscriptionConsumed)
	assert.Zero(t, receipt.WalletConsumed)
	assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)

	repeated, err := SettleSubscriptionBilling(fixture.params)
	require.NoError(t, err)
	assert.Equal(t, receipt, repeated)
	require.NoError(t, RefundSubscriptionBilling(fixture.params))
	assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
}

func TestSubscriptionBillingTerminalReceiptStillValidatesTokenKey(t *testing.T) {
	for _, terminal := range []string{"settled", "refunded"} {
		t.Run(terminal, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			if terminal == "settled" {
				_, err := SettleSubscriptionBilling(fixture.params)
				require.NoError(t, err)
			} else {
				require.NoError(t, RefundSubscriptionBilling(fixture.params))
			}
			fixture.params.TokenKey = "wrong-key-for-a-terminal-request"
			_, err := SettleSubscriptionBilling(fixture.params)
			require.Error(t, err)
			require.Error(t, RefundSubscriptionBilling(fixture.params), "an idempotent terminal lookup must still authenticate the token")
			if terminal == "settled" {
				assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
			} else {
				assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
			}
		})
	}
}

func TestRefundSubscriptionBillingReturnsPreConsumeOnlyOnce(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	require.NoError(t, RefundSubscriptionBilling(fixture.params))
	assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
	require.NoError(t, RefundSubscriptionBilling(fixture.params))
	assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
	_, err := SettleSubscriptionBilling(fixture.params)
	require.Error(t, err, "a refunded request cannot later be settled")
	assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
}

func TestSubscriptionBillingReturnsAdditionalReservationWhenNoUsageIsBilled(t *testing.T) {
	for _, operation := range []string{"settle zero", "refund"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 200, 60)
			fixture.params.ActualQuota = 120
			reserved, err := ReserveSubscriptionBilling(fixture.params)
			require.NoError(t, err)
			require.NotNil(t, reserved)
			assert.EqualValues(t, 60, reserved.SubscriptionDelta)
			assert.Equal(t, 60, reserved.TokenDelta)
			assert.EqualValues(t, 120, reserved.SubscriptionConsumed)
			assert.Zero(t, reserved.WalletConsumed)
			assertSubscriptionSettlementBalances(t, fixture, 120, 1000, 880, 120)

			fixture.params.PreConsumedQuota = 120
			fixture.params.TokenConsumedQuota = 120
			fixture.params.ActualQuota = 0
			if operation == "settle zero" {
				receipt, err := SettleSubscriptionBilling(fixture.params)
				require.NoError(t, err)
				require.NotNil(t, receipt)
				assert.EqualValues(t, -120, receipt.SubscriptionDelta)
				assert.Equal(t, -120, receipt.TokenDelta)
				assert.Zero(t, receipt.SubscriptionConsumed)
				_, err = SettleSubscriptionBilling(fixture.params)
				require.NoError(t, err)
			} else {
				require.NoError(t, RefundSubscriptionBilling(fixture.params))
			}
			require.NoError(t, RefundSubscriptionBilling(fixture.params))
			assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
		})
	}
}

func TestRefundSubscriptionBillingReceiptFailureRollsBackCredits(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 60)
	forcedErr := errors.New("forced refund receipt write failure")
	callbackName := "test:subscription_refund_receipt_failure:" + common.GetUUID()
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "subscription_pre_consume_records" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { DB.Callback().Update().Remove(callbackName) })

	require.ErrorIs(t, RefundSubscriptionBilling(fixture.params), forcedErr)
	assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
	require.NoError(t, DB.Callback().Update().Remove(callbackName))
	require.NoError(t, RefundSubscriptionBilling(fixture.params))
	assertSubscriptionSettlementBalances(t, fixture, 0, 1000, 1000, 0)
}

func TestSettleSubscriptionBillingWriteFailuresRollBackEveryBalance(t *testing.T) {
	for _, table := range []string{"user_subscriptions", "users", "tokens", "subscription_pre_consume_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newSubscriptionSettlementFixture(t, 100, 60)
			forcedErr := errors.New("forced settlement write failure")
			callbackName := "test:subscription_settlement_failure:" + common.GetUUID()
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == table {
					tx.AddError(forcedErr)
				}
			}))
			t.Cleanup(func() { DB.Callback().Update().Remove(callbackName) })

			_, err := SettleSubscriptionBilling(fixture.params)
			require.ErrorIs(t, err, forcedErr)
			assertSubscriptionSettlementBalances(t, fixture, 60, 1000, 940, 60)
			var record SubscriptionPreConsumeRecord
			require.NoError(t, DB.Where("request_id = ?", fixture.params.RequestId).First(&record).Error)
			assert.Equal(t, "consumed", record.Status, "a failed transaction must not publish a settled receipt")

			require.NoError(t, DB.Callback().Update().Remove(callbackName))
			_, err = SettleSubscriptionBilling(fixture.params)
			require.NoError(t, err, "a proven rollback leaves the same request retryable")
			assertSubscriptionSettlementBalances(t, fixture, 100, 940, 840, 160)
		})
	}
}

func TestConcurrentSubscriptionSettlementsShareRemainingQuotaOnce(t *testing.T) {
	fixture := newSubscriptionSettlementFixture(t, 100, 30)
	second := fixture.params
	second.RequestId = "second-settlement-" + common.GetUUID()
	_, err := PreConsumeUserSubscription(second.RequestId, fixture.user.Id, "test-model", 0, 30)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.token.Id).
		Updates(map[string]interface{}{"remain_quota": 940, "used_quota": 60}).Error)
	fixture.params.ActualQuota = 100
	second.ActualQuota = 100

	type settlementResult struct {
		receipt *SubscriptionBillingReceipt
		err     error
	}
	results := make(chan settlementResult, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, params := range []SubscriptionBillingParams{fixture.params, second} {
		workers.Add(1)
		go func(params SubscriptionBillingParams) {
			defer workers.Done()
			<-start
			receipt, err := SettleSubscriptionBilling(params)
			results <- settlementResult{receipt: receipt, err: err}
		}(params)
	}
	close(start)
	workers.Wait()
	close(results)
	var subscriptionDelta, walletDelta int64
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.receipt)
		subscriptionDelta += result.receipt.SubscriptionDelta
		walletDelta += result.receipt.WalletDelta
	}
	assert.EqualValues(t, 40, subscriptionDelta, "the two requests share the same remaining 40 subscription quota")
	assert.EqualValues(t, 100, walletDelta)
	assertSubscriptionSettlementBalances(t, fixture, 100, 900, 800, 200)
}
