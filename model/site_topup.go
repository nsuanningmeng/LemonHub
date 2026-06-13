package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TopUpStatusManualReview marks a PAID order that could not be auto-settled because the
// sub-site agent's procurement wallet was insufficient at callback time (a race after the
// pre-order check). The user has paid the agent, but the platform issues NO quota until an
// admin resolves it — so the platform never advances quota it wasn't paid for, and the
// user's payment is never silently lost (the order is on record for manual handling).
const TopUpStatusManualReview = "manual_review"

// ErrSiteTopUpUnresolved is returned when a sub-site order's owning site / wholesale cost
// cannot be determined at settlement time. Settlement then fails CLOSED (the user is never
// credited for free); the callback is retried so a later attempt can resolve the cost.
var ErrSiteTopUpUnresolved = errors.New("无法确定子站充值成本，结算暂缓")

// errTopUpRaceLost is an internal sentinel: another settlement concurrently claimed the
// order, so this transaction must roll back (undoing any wallet debit) and treat the
// outcome as an idempotent no-op.
var errTopUpRaceLost = errors.New("topup already settled concurrently")

// CompleteEpayTopUp settles a verified, paid epay top-up order exactly once. For a sub-site
// order it credits the user AND debits the agent's procurement wallet (flow type=4) in ONE
// transaction; insufficient wallet parks the order as manual_review (credits nothing).
//
// Idempotency / cross-node safety: the order is claimed via an atomic conditional UPDATE
// (`... WHERE trade_no = ? AND status = 'pending'`). Only the winner's RowsAffected==1; a
// concurrent or duplicate callback either reads a non-pending status up front, or loses the
// claim and rolls back (no double-credit, no double-debit) — this does NOT rely on
// SELECT ... FOR UPDATE (which GORM v2 does not emit via gorm:query_option, and which SQLite
// rejects). For a sub-site order, costMilli must be > 0 or settlement fails closed.
func CompleteEpayTopUp(tradeNo string, costMilli int64, operatorUserId int) (finalStatus string, quotaAdded int, err error) {
	var settledUserId int
	err = DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		if e := tx.Where("trade_no = ?", tradeNo).First(&topUp).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return e
		}
		if topUp.Status != common.TopUpStatusPending {
			finalStatus = topUp.Status // already settled / parked: idempotent no-op
			return nil
		}

		quota := int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())

		target := common.TopUpStatusSuccess
		if topUp.SiteId > 0 {
			// Fail closed: a sub-site order with no resolvable wholesale cost must NEVER be
			// credited for free (e.g. site cache miss yielding costMilli=0).
			if costMilli <= 0 {
				return ErrSiteTopUpUnresolved
			}
			if e := DeductSiteWallet(tx, topUp.SiteId, costMilli, WalletLogTypeTopupDeduct, tradeNo, "用户在线充值扣货款", operatorUserId); e != nil {
				if errors.Is(e, ErrInsufficientWalletBalance) {
					target = TopUpStatusManualReview
				} else {
					return e
				}
			}
		}

		// Atomic claim: flip pending -> target only if STILL pending. Losing this race means
		// another settlement already handled the order; roll back (incl. any wallet debit).
		claim := tx.Model(&TopUp{}).
			Where("trade_no = ? AND status = ?", tradeNo, common.TopUpStatusPending).
			Updates(map[string]interface{}{"status": target, "complete_time": common.GetTimestamp()})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			return errTopUpRaceLost
		}

		if target == common.TopUpStatusSuccess {
			if e := tx.Model(&User{}).Where("id = ?", topUp.UserId).
				Update("quota", gorm.Expr("quota + ?", quota)).Error; e != nil {
				return e
			}
			quotaAdded = quota
			settledUserId = topUp.UserId
		}
		finalStatus = target
		return nil
	})
	if errors.Is(err, errTopUpRaceLost) {
		// The concurrent winner committed; report its terminal status as an idempotent no-op.
		var cur TopUp
		if e := DB.Where("trade_no = ?", tradeNo).First(&cur).Error; e == nil {
			return cur.Status, 0, nil
		}
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	if quotaAdded > 0 && settledUserId > 0 {
		uid, q := settledUserId, quotaAdded
		gopool.Go(func() {
			if e := cacheIncrUserQuota(uid, int64(q)); e != nil {
				common.SysLog("failed to refresh user quota cache after topup: " + e.Error())
			}
		})
	}
	return finalStatus, quotaAdded, err
}

// RetryManualReviewTopUp lets a platform admin re-settle a parked (manual_review) order
// after the agent has topped up their wallet: it atomically returns the order to pending
// and re-runs CompleteEpayTopUp. If the wallet is still insufficient it parks again.
func RetryManualReviewTopUp(tradeNo string, costMilli int64, operatorUserId int) (finalStatus string, quotaAdded int, err error) {
	res := DB.Model(&TopUp{}).
		Where("trade_no = ? AND status = ?", tradeNo, TopUpStatusManualReview).
		Update("status", common.TopUpStatusPending)
	if res.Error != nil {
		return "", 0, res.Error
	}
	if res.RowsAffected == 0 {
		return "", 0, errors.New("订单不处于待人工处理状态")
	}
	return CompleteEpayTopUp(tradeNo, costMilli, operatorUserId)
}
