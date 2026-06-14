package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

func newTieredSnapshot(groupRatio float64) *billingexpr.BillingSnapshot {
	exprStr := `tier("default", p + c)`
	return &billingexpr.BillingSnapshot{
		BillingMode:               "tiered_expr",
		ExprString:                exprStr,
		ExprHash:                  billingexpr.ExprHashString(exprStr),
		GroupRatio:                groupRatio,
		EstimatedQuotaBeforeGroup: 1000,
		EstimatedQuotaAfterGroup:  billingexpr.QuotaRound(1000 * groupRatio),
		QuotaPerUnit:              500_000,
	}
}

func TestBillingSnapshot_SyncGroupRatio(t *testing.T) {
	t.Run("updates ratio and after-group estimate", func(t *testing.T) {
		snap := newTieredSnapshot(1.0)
		snap.SyncGroupRatio(3.0)
		require.Equal(t, 3.0, snap.GroupRatio)
		require.Equal(t, billingexpr.QuotaRound(1000*3.0), snap.EstimatedQuotaAfterGroup)
	})

	t.Run("no-op when ratio unchanged", func(t *testing.T) {
		snap := newTieredSnapshot(2.0)
		before := snap.EstimatedQuotaAfterGroup
		snap.SyncGroupRatio(2.0)
		require.Equal(t, 2.0, snap.GroupRatio)
		require.Equal(t, before, snap.EstimatedQuotaAfterGroup)
	})

	t.Run("nil snapshot is safe", func(t *testing.T) {
		var snap *billingexpr.BillingSnapshot
		require.NotPanics(t, func() { snap.SyncGroupRatio(2.0) })
	})
}

// 端到端验证修复效果：失败转移把分组从倍率 1.0 切到 2.0 后，
// 分层结算（ComputeTieredQuota 内部 quotaBeforeGroup * snap.GroupRatio）应按新分组倍率计费。
func TestSyncGroupRatio_AffectsTieredSettlement(t *testing.T) {
	snap := newTieredSnapshot(1.0)
	params := billingexpr.TokenParams{P: 1000, C: 500} // exprOutput=1500; before-group = 1500/1M*500K = 750

	base, err := billingexpr.ComputeTieredQuota(snap, params)
	require.NoError(t, err)
	require.Equal(t, billingexpr.QuotaRound(750*1.0), base.ActualQuotaAfterGroup)

	// 模拟失败转移切换到 2.0 倍率分组
	snap.SyncGroupRatio(2.0)

	after, err := billingexpr.ComputeTieredQuota(snap, params)
	require.NoError(t, err)
	require.Equal(t, billingexpr.QuotaRound(750*2.0), after.ActualQuotaAfterGroup)
	require.Equal(t, base.ActualQuotaAfterGroup*2, after.ActualQuotaAfterGroup)
}
