package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type blockedShutdownWallet struct {
	*WalletFunding
	entered chan struct{}
	release chan struct{}
}

func resetShutdownRefundFailures(t *testing.T) {
	t.Helper()
	_ = WaitBillingRefunds(context.Background())
	billingRefundFailuresMu.Lock()
	previousCount, previousError := billingRefundFailures, billingRefundLastError
	billingRefundFailures, billingRefundLastError = 0, nil
	billingRefundFailuresMu.Unlock()
	t.Cleanup(func() {
		billingRefundFailuresMu.Lock()
		billingRefundFailures, billingRefundLastError = previousCount, previousError
		billingRefundFailuresMu.Unlock()
	})
}

func TestWaitBillingRefundsReportsPersistenceFailureWithoutRetryingCredit(t *testing.T) {
	truncate(t)
	resetShutdownRefundFailures(t)
	owner := model.User{Id: 1, Username: "failed-shutdown-refund", Quota: 987}
	require.NoError(t, model.DB.Create(&owner).Error)
	forced := errors.New("wallet refund persistence rejected")
	const callback = "test:shutdown-refund-failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(forced)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callback) })
	session := &BillingSession{relayInfo: &relaycommon.RelayInfo{UserId: 1, IsPlayground: true}, funding: &WalletFunding{userId: 1, consumed: 13}, preConsumedQuota: 13, tokenConsumed: 13}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	session.Refund(c)
	err := WaitBillingRefunds(context.Background())
	require.ErrorIs(t, err, ErrBillingRefundsFailed)
	require.ErrorIs(t, err, forced)
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	session.Refund(c)
	require.ErrorIs(t, WaitBillingRefunds(context.Background()), ErrBillingRefundsFailed)
	require.NoError(t, model.DB.First(&owner, owner.Id).Error)
	assert.Equal(t, 987, owner.Quota, "waiting/repeating Refund must not retry a non-idempotent failed credit")
}

func (w *blockedShutdownWallet) Refund() error {
	close(w.entered)
	<-w.release
	return w.WalletFunding.Refund()
}

func TestWaitBillingRefundsDrainsWalletAndTokenBeforeDatabaseClose(t *testing.T) {
	truncate(t)
	resetShutdownRefundFailures(t)
	owner := model.User{Id: 1, SiteId: 7, Username: "shutdown-owner", AffCode: "shutdown-owner-aff", Quota: 987}
	other := model.User{Id: 2, SiteId: 8, Username: "shutdown-other", AffCode: "shutdown-other-aff", Quota: 2000}
	require.NoError(t, model.DB.Create(&owner).Error)
	require.NoError(t, model.DB.Create(&other).Error)
	token := model.Token{Id: 1, SiteId: 7, UserId: 1, Key: "shutdown-token", RemainQuota: 987, UsedQuota: 13}
	require.NoError(t, model.DB.Create(&token).Error)
	funding := &blockedShutdownWallet{WalletFunding: &WalletFunding{userId: 1, consumed: 13}, entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(funding.release) }) }
	t.Cleanup(func() { release(); require.NoError(t, WaitBillingRefunds(context.Background())) })
	session := &BillingSession{relayInfo: &relaycommon.RelayInfo{UserId: 1, TokenId: 1, TokenKey: token.Key}, funding: funding, preConsumedQuota: 13, tokenConsumed: 13}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	session.Refund(c)
	<-funding.entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, WaitBillingRefunds(ctx), context.Canceled, "a running refund must not be reported as drained")
	release()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.NoError(t, WaitBillingRefunds(waitCtx))
	session.Refund(c)
	require.NoError(t, WaitBillingRefunds(waitCtx))
	require.NoError(t, model.DB.First(&owner, owner.Id).Error)
	require.NoError(t, model.DB.First(&other, other.Id).Error)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 1000, owner.Quota)
	assert.Equal(t, 2000, other.Quota, "refund must stay on the original site's account")
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}
