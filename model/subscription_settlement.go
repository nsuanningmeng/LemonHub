package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrSubscriptionBillingConflict = errors.New("subscription billing receipt conflict")
var ErrSubscriptionQuotaInsufficient = errors.New("subscription quota insufficient")
var ErrSubscriptionTokenQuotaInsufficient = errors.New("token quota insufficient for subscription reservation")

var ErrSubscriptionBillingUncertain = errors.New("subscription billing outcome requires reconciliation")

// SubscriptionBillingParams binds settlement to the durable request baseline.
// AllowWalletOverflow is the caller's policy permission; subscription snapshots
// and other active strict subscriptions are independently checked in the model.
type SubscriptionBillingParams struct {
	RequestId                                         string
	UserId, SubscriptionId, TokenId                   int
	TokenKey                                          string
	PreConsumedQuota, TokenConsumedQuota, ActualQuota int
	AllowWalletOverflow                               bool
}

type SubscriptionBillingReceipt struct {
	SubscriptionDelta, WalletDelta                                                 int64
	TokenDelta, ActualQuota                                                        int
	SubscriptionConsumed, WalletConsumed, SubscriptionUsedAfter, SubscriptionTotal int64
}

func SettleSubscriptionBilling(p SubscriptionBillingParams) (*SubscriptionBillingReceipt, error) {
	return changeSubscriptionBilling(p, "settle")
}

func ReserveSubscriptionBilling(p SubscriptionBillingParams) (*SubscriptionBillingReceipt, error) {
	return changeSubscriptionBilling(p, "reserve")
}

func RefundSubscriptionBilling(p SubscriptionBillingParams) error {
	_, err := changeSubscriptionBilling(p, "refund")
	return err
}

func subscriptionBillingReceipt(r *SubscriptionPreConsumeRecord) *SubscriptionBillingReceipt {
	reserved := r.ReservedQuota
	if reserved == 0 {
		reserved = r.PreConsumed
	}
	return &SubscriptionBillingReceipt{
		SubscriptionDelta: r.SubscriptionDelta, WalletDelta: r.WalletDelta,
		TokenDelta: r.TokenDelta, ActualQuota: r.SettledQuota,
		SubscriptionConsumed: reserved + r.SubscriptionDelta, WalletConsumed: r.WalletDelta,
		SubscriptionUsedAfter: r.SubscriptionUsedAfter, SubscriptionTotal: r.SubscriptionTotal,
	}
}

func changeSubscriptionBilling(p SubscriptionBillingParams, operation string) (*SubscriptionBillingReceipt, error) {
	if strings.TrimSpace(p.RequestId) == "" || p.UserId <= 0 || p.SubscriptionId <= 0 ||
		p.PreConsumedQuota <= 0 || p.PreConsumedQuota > common.MaxQuota ||
		p.ActualQuota < 0 || p.ActualQuota > common.MaxQuota ||
		p.TokenConsumedQuota < 0 || p.TokenConsumedQuota > common.MaxQuota ||
		(p.TokenId > 0 && (p.TokenKey == "" || p.TokenConsumedQuota != p.PreConsumedQuota)) ||
		(p.TokenId <= 0 && p.TokenConsumedQuota != 0) {
		return nil, fmt.Errorf("%w: invalid request/baseline", ErrSubscriptionBillingConflict)
	}
	now := GetDBTimestamp()
	prepared := prepareSubscriptionBillingCaches(p, operation)
	priorRequestFence := false
	var mutations []*subscriptionBillingCacheMutation
	tx := DB.Session(&gorm.Session{SkipDefaultTransaction: true}).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	var record SubscriptionPreConsumeRecord
	var receipt *SubscriptionBillingReceipt
	changed := false
	previousReconciliationRequired := false
	err := func() error {
		if err := lockForUpdate(tx).Where("request_id = ?", p.RequestId).First(&record).Error; err != nil {
			return err
		}
		priorRequestFence = subscriptionBillingHasRequestFence(p)
		if record.UserId != p.UserId || record.UserSubscriptionId != p.SubscriptionId ||
			(record.TokenBound && record.TokenId != p.TokenId) {
			return ErrSubscriptionBillingConflict
		}
		// Receipt lookups are also owner/key checks, including zero-delta and
		// terminal re-entry. IDs alone do not authorize another site's token.
		var identityUser User
		if err := tx.Select("id", "site_id").Where("id = ?", p.UserId).First(&identityUser).Error; err != nil {
			return err
		}
		if p.TokenId > 0 {
			var identityToken Token
			if err := tx.Select("id", "user_id", "site_id", "key").Where("id = ?", p.TokenId).First(&identityToken).Error; err != nil {
				return err
			}
			if identityToken.UserId != p.UserId || identityToken.SiteId != identityUser.SiteId || identityToken.Key != p.TokenKey {
				return ErrSubscriptionBillingConflict
			}
		}
		if record.Status == "settled" {
			if operation == "refund" {
				receipt = subscriptionBillingReceipt(&record)
				return nil
			}
			if operation != "settle" || record.SettledQuota != p.ActualQuota || record.TokenConsumedQuota != p.TokenConsumedQuota {
				return ErrSubscriptionBillingConflict
			}
			receipt = subscriptionBillingReceipt(&record)
			return nil
		}
		if record.Status == "refunded" {
			if operation != "refund" {
				return ErrSubscriptionBillingConflict
			}
			receipt = subscriptionBillingReceipt(&record)
			return nil
		}
		if record.Status != "consumed" {
			return ErrSubscriptionBillingConflict
		}
		reserved := record.ReservedQuota
		if reserved == 0 {
			reserved = record.PreConsumed
		}
		if operation == "reserve" && record.TokenBound && reserved == int64(p.ActualQuota) && int64(p.PreConsumedQuota) <= reserved {
			receipt = subscriptionBillingReceipt(&record)
			receipt.ActualQuota = int(reserved)
			return nil
		}
		if reserved != int64(p.PreConsumedQuota) {
			return fmt.Errorf("%w: reserved quota changed", ErrSubscriptionBillingConflict)
		}
		var sub UserSubscription
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", p.SubscriptionId, p.UserId).First(&sub).Error; err != nil {
			return err
		}
		if sub.AmountUsed < 0 {
			return errors.New("invalid subscription amount used")
		}
		actual := p.ActualQuota
		if operation == "refund" {
			actual = 0
		}
		subDelta := int64(actual) - reserved
		if operation == "reserve" && subDelta <= 0 {
			receipt = &SubscriptionBillingReceipt{ActualQuota: p.PreConsumedQuota, SubscriptionConsumed: reserved, SubscriptionUsedAfter: sub.AmountUsed, SubscriptionTotal: sub.AmountTotal}
			return nil
		}
		if subDelta < 0 && sub.LastResetTime > record.CreatedAt {
			return errors.New("subscription period changed; refund requires reconciliation")
		}
		walletDelta := int64(0)
		if subDelta > 0 && sub.AmountTotal > 0 && subDelta > sub.AmountTotal-sub.AmountUsed {
			allow := operation == "settle" && p.AllowWalletOverflow && sub.AllowWalletOverflow
			if allow {
				var strict int64
				if err := tx.Model(&UserSubscription{}).Where("user_id = ? AND status = ? AND end_time > ? AND allow_wallet_overflow = ?", p.UserId, "active", now, false).Count(&strict).Error; err != nil {
					return err
				}
				allow = strict == 0
			}
			if !allow {
				return fmt.Errorf("%w: subscription used exceeds total, used=%d total=%d", ErrSubscriptionQuotaInsufficient, sub.AmountUsed, sub.AmountTotal)
			}
			remaining := sub.AmountTotal - sub.AmountUsed
			if remaining < 0 {
				remaining = 0
			}
			walletDelta = subDelta - remaining
			subDelta = remaining
		}
		if subDelta > 0 && sub.AmountUsed > int64(^uint64(0)>>1)-subDelta {
			return errors.New("subscription quota overflow")
		}
		newUsed := sub.AmountUsed + subDelta
		if newUsed < 0 {
			return errors.New("subscription refund exceeds current usage; reconciliation required")
		}
		// Keep token before wallet lock ordering compatible with batched accounting.
		var token Token
		tokenDelta := 0
		if p.TokenId > 0 {
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", p.TokenId, p.UserId).First(&token).Error; err != nil {
				return err
			}
			if token.Key != p.TokenKey {
				return ErrSubscriptionBillingConflict
			}
			tokenDelta = actual - p.TokenConsumedQuota
			if operation == "reserve" && tokenDelta > 0 && !token.UnlimitedQuota && token.RemainQuota < tokenDelta {
				return ErrSubscriptionTokenQuotaInsufficient
			}
			if err := validateSubscriptionTokenDelta(token, tokenDelta); err != nil {
				return err
			}
		}
		var user User
		if walletDelta != 0 {
			if err := lockForUpdate(tx).Where("id = ?", p.UserId).First(&user).Error; err != nil {
				return err
			}
			if int64(user.Quota) < -int64(common.MaxWalletQuota)+walletDelta {
				return common.ErrWalletQuotaOutOfRange
			}
			if p.TokenId > 0 && token.SiteId != user.SiteId {
				return ErrSubscriptionBillingConflict
			}
		}
		// Cache debits remain on the live generation. A fence prevents a cache
		// miss from recreating spendable balances while these SQL writes run.
		for _, target := range []struct {
			kind  string
			delta int64
		}{{taskBillingCacheUser, -walletDelta}, {taskBillingCacheToken, -int64(tokenDelta)}} {
			if target.delta == 0 {
				continue
			}
			mutation, err := prepared.begin(target.kind, target.delta, p.RequestId, operation == "reserve" && target.kind == taskBillingCacheToken && !token.UnlimitedQuota)
			if mutation != nil {
				mutations = append(mutations, mutation)
			}
			if err != nil {
				return err
			}
		}
		if subDelta != 0 {
			if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Update("amount_used", newUsed).Error; err != nil {
				return err
			}
		}
		if walletDelta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("quota", int64(user.Quota)-walletDelta).Error; err != nil {
				return err
			}
		}
		if tokenDelta != 0 {
			if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{"remain_quota": token.RemainQuota - tokenDelta, "used_quota": token.UsedQuota + tokenDelta, "accessed_time": common.GetTimestamp()}).Error; err != nil {
				return err
			}
		}
		record.TokenBound, record.TokenId = true, p.TokenId
		record.TokenConsumedQuota = p.TokenConsumedQuota
		record.ReservedQuota = reserved
		record.SubscriptionUsedAfter, record.SubscriptionTotal = newUsed, sub.AmountTotal
		if operation == "reserve" {
			record.ReservedQuota = int64(actual)
			record.TokenConsumedQuota = actual
			if p.TokenId <= 0 {
				record.TokenConsumedQuota = 0
			}
		} else {
			record.Status = "settled"
			if operation == "refund" {
				record.Status = "refunded"
			}
			record.SettledQuota = actual
			record.SubscriptionDelta, record.WalletDelta, record.TokenDelta = subDelta, walletDelta, tokenDelta
		}
		previousReconciliationRequired = record.ReconciliationRequired || priorRequestFence
		record.ReconciliationRequired = previousReconciliationRequired || common.RedisEnabled
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		changed = true
		receipt = subscriptionBillingReceipt(&record)
		if operation == "reserve" {
			receipt.ActualQuota = actual
			receipt.SubscriptionDelta = subDelta
			receipt.TokenDelta = tokenDelta
		}
		return nil
	}()
	if err != nil {
		rollbackErr := tx.Rollback().Error
		if rollbackErr != nil {
			return nil, errors.Join(err, ErrSubscriptionBillingUncertain, rollbackErr)
		}
		for _, mutation := range mutations {
			mutation.finish(false)
		}
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		// Missing immediate readback is not evidence of rollback. Preserve every
		// cache fence; a terminal receipt can be inspected without replaying money.
		common.SysError(fmt.Sprintf("subscription billing commit uncertain: request=%s operation=%s user=%d subscription=%d token=%d error=%v", p.RequestId, operation, p.UserId, p.SubscriptionId, p.TokenId, err))
		return nil, errors.Join(ErrSubscriptionBillingUncertain, err)
	}
	cacheConfirmed := true
	for _, mutation := range mutations {
		if !mutation.finish(true) {
			cacheConfirmed = false
		}
	}
	if changed && !previousReconciliationRequired && record.ReconciliationRequired && cacheConfirmed {
		// Cache confirmations are separate from the money transaction. Failure
		// to clear this retention flag keeps evidence, never retries a debit.
		if err := DB.Model(&SubscriptionPreConsumeRecord{}).Where("id = ? AND status = ? AND reserved_quota = ? AND token_consumed_quota = ? AND settled_quota = ?", record.Id, record.Status, record.ReservedQuota, record.TokenConsumedQuota, record.SettledQuota).Update("reconciliation_required", false).Error; err != nil {
			common.SysError(fmt.Sprintf("subscription receipt retention flag requires reconciliation: request=%s error=%v", p.RequestId, err))
		}
	}
	return receipt, nil
}

func validateSubscriptionTokenDelta(token Token, delta int) error {
	maxQuota := int64(^uint(0) >> 1)
	minQuota := -maxQuota - 1
	remain, used, d := int64(token.RemainQuota), int64(token.UsedQuota), int64(delta)
	if remain < minQuota || used < 0 || (d > 0 && (remain < minQuota+d || used > maxQuota-d)) || (d < 0 && (remain > maxQuota+d || used < -d)) {
		return errors.New("token quota delta out of range")
	}
	return nil
}
