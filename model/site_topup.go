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

// CompleteEpayTopUp settles a verified, paid epay top-up order exactly once, in a single
// transaction:
//   - locks the order by trade_no; if it is not Pending, it no-ops (idempotent — repeated
//     callbacks for the same order are processed once);
//   - for a sub-site order (SiteId>0) it FIRST debits the agent's procurement wallet by
//     costMilli (flow type=4). If the wallet is insufficient it parks the order as
//     manual_review and credits NOTHING (returns finalStatus=manual_review);
//   - otherwise it credits the user's quota and marks the order Success, atomically with
//     the wallet debit.
//
// The user cache is refreshed after commit. costMilli is the agent's cost in 厘 for this
// recharge (充值面值 × discount_rate), computed by the caller; pass 0 for main-site orders.
func CompleteEpayTopUp(tradeNo string, costMilli int64, operatorUserId int) (finalStatus string, quotaAdded int, err error) {
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	var settledUserId int
	err = DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		if e := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(&topUp).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return e
		}
		// Idempotency: only a still-Pending order is settled; an already-Success or
		// already-parked order is a no-op so duplicate callbacks never double-credit.
		if topUp.Status != common.TopUpStatusPending {
			finalStatus = topUp.Status
			return nil
		}

		quota := int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())

		// Sub-site: debit the agent wallet first; insufficient -> park, credit nothing.
		if topUp.SiteId > 0 && costMilli > 0 {
			if e := DeductSiteWallet(tx, topUp.SiteId, costMilli, WalletLogTypeTopupDeduct, tradeNo, "用户在线充值扣货款", operatorUserId); e != nil {
				if errors.Is(e, ErrInsufficientWalletBalance) {
					topUp.Status = TopUpStatusManualReview
					topUp.CompleteTime = common.GetTimestamp()
					if e2 := tx.Save(&topUp).Error; e2 != nil {
						return e2
					}
					finalStatus = TopUpStatusManualReview
					return nil
				}
				return e
			}
		}

		// Credit the user's quota in the same transaction as the wallet debit.
		if e := tx.Model(&User{}).Where("id = ?", topUp.UserId).
			Update("quota", gorm.Expr("quota + ?", quota)).Error; e != nil {
			return e
		}
		topUp.Status = common.TopUpStatusSuccess
		topUp.CompleteTime = common.GetTimestamp()
		if e := tx.Save(&topUp).Error; e != nil {
			return e
		}
		finalStatus = common.TopUpStatusSuccess
		quotaAdded = quota
		settledUserId = topUp.UserId
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	// Mirror IncreaseUserQuota: refresh the Redis user-quota cache after the DB commit.
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
