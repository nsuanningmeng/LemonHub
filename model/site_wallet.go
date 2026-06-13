package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// SiteWalletLog is the immutable ledger for a sub-site's "进货钱包" (procurement
// wallet). Every wallet balance change MUST be accompanied by exactly one of these
// records — balance is never mutated without a corresponding flow entry. Amount is in
// 厘 (0.001 CNY): positive = credit (入账), negative = debit (出账).
type SiteWalletLog struct {
	Id             int    `json:"id"`
	SiteId         int    `json:"site_id" gorm:"index"`
	Type           int    `json:"type" gorm:"index"`
	Amount         int64  `json:"amount"`        // signed, 厘
	BalanceAfter   int64  `json:"balance_after"` // wallet balance after this change, for reconciliation
	RelatedId      string `json:"related_id" gorm:"type:varchar(64);index"`
	Remark         string `json:"remark" gorm:"type:varchar(255)"`
	OperatorUserId int    `json:"operator_user_id"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint;index"`
}

// Wallet flow types.
const (
	WalletLogTypeRecharge       = 1 // 代理充值（平台给代理钱包加进货款）
	WalletLogTypeRedemptionGen  = 2 // 生成兑换码扣款
	WalletLogTypeRedemptionVoid = 3 // 作废兑换码退款
	WalletLogTypeTopupDeduct    = 4 // 用户在线充值扣货款
	WalletLogTypeManualAdjust   = 5 // 管理员手动调整
)

// ErrInsufficientWalletBalance is returned when a conditional wallet debit affects
// zero rows because the balance is below the requested amount.
var ErrInsufficientWalletBalance = errors.New("子站钱包余额不足")

// DeductSiteWallet atomically debits `amount` (厘, must be positive) from a sub-site's
// wallet and records a flow log, all within the caller's transaction `tx`.
//
// The debit is a single conditional UPDATE (`wallet_balance >= amount`); if it affects
// zero rows the balance is insufficient and ErrInsufficientWalletBalance is returned —
// there is no "read then write" race. The caller MUST run this inside a transaction so
// that the paired business action (e.g. inserting redemption codes) commits atomically
// with the debit. Negative balances are therefore impossible.
func DeductSiteWallet(tx *gorm.DB, siteId int, amount int64, logType int, relatedId, remark string, operatorUserId int) error {
	if amount <= 0 {
		return errors.New("扣款金额必须为正")
	}
	res := tx.Model(&Site{}).
		Where("id = ? AND wallet_balance >= ?", siteId, amount).
		Update("wallet_balance", gorm.Expr("wallet_balance - ?", amount))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInsufficientWalletBalance
	}
	balanceAfter, err := lockedWalletBalance(tx, siteId)
	if err != nil {
		return err
	}
	return tx.Create(&SiteWalletLog{
		SiteId:         siteId,
		Type:           logType,
		Amount:         -amount,
		BalanceAfter:   balanceAfter,
		RelatedId:      relatedId,
		Remark:         remark,
		OperatorUserId: operatorUserId,
		CreatedTime:    common.GetTimestamp(),
	}).Error
}

// AddSiteWallet atomically credits `amount` (厘, must be positive) to a sub-site's
// wallet and records a flow log, within the caller's transaction `tx`.
func AddSiteWallet(tx *gorm.DB, siteId int, amount int64, logType int, relatedId, remark string, operatorUserId int) error {
	if amount <= 0 {
		return errors.New("入账金额必须为正")
	}
	res := tx.Model(&Site{}).
		Where("id = ?", siteId).
		Update("wallet_balance", gorm.Expr("wallet_balance + ?", amount))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("子站不存在")
	}
	balanceAfter, err := lockedWalletBalance(tx, siteId)
	if err != nil {
		return err
	}
	return tx.Create(&SiteWalletLog{
		SiteId:         siteId,
		Type:           logType,
		Amount:         amount,
		BalanceAfter:   balanceAfter,
		RelatedId:      relatedId,
		Remark:         remark,
		OperatorUserId: operatorUserId,
		CreatedTime:    common.GetTimestamp(),
	}).Error
}

// lockedWalletBalance returns the wallet balance of a site as seen inside tx (i.e. the
// value just written by the preceding conditional UPDATE, whose row lock is held by tx).
func lockedWalletBalance(tx *gorm.DB, siteId int) (int64, error) {
	var site Site
	if err := tx.Select("wallet_balance").First(&site, "id = ?", siteId).Error; err != nil {
		return 0, err
	}
	return site.WalletBalance, nil
}

// RechargeSiteWallet credits a sub-site's procurement wallet (platform → agent, flow
// type=1). amount is in 厘 and must be positive. Used by the main-site admin.
func RechargeSiteWallet(siteId int, amount int64, remark string, operatorUserId int) error {
	if siteId <= 0 {
		return errors.New("无效的子站")
	}
	if amount <= 0 {
		return errors.New("充值金额必须为正")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return AddSiteWallet(tx, siteId, amount, WalletLogTypeRecharge, "", remark, operatorUserId)
	})
}

// AdjustSiteWallet applies a signed manual adjustment (flow type=5) to a sub-site's
// wallet: positive credits, negative debits (and fails closed if the debit exceeds the
// balance). remark is mandatory for audit. Used by the main-site admin.
func AdjustSiteWallet(siteId int, delta int64, remark string, operatorUserId int) error {
	if siteId <= 0 {
		return errors.New("无效的子站")
	}
	if delta == 0 {
		return errors.New("调整金额不能为 0")
	}
	if remark == "" {
		return errors.New("手动调整必须填写备注")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if delta > 0 {
			return AddSiteWallet(tx, siteId, delta, WalletLogTypeManualAdjust, "", remark, operatorUserId)
		}
		return DeductSiteWallet(tx, siteId, -delta, WalletLogTypeManualAdjust, "", remark, operatorUserId)
	})
}

// SetSiteWalletWarnThreshold updates a sub-site's low-balance alert threshold (厘).
func SetSiteWalletWarnThreshold(siteId int, threshold int64) error {
	if siteId <= 0 {
		return errors.New("无效的子站")
	}
	if threshold < 0 {
		return errors.New("警戒线不能为负")
	}
	if err := DB.Model(&Site{}).Where("id = ?", siteId).
		Update("wallet_warn_threshold", threshold).Error; err != nil {
		return err
	}
	reloadSiteCacheSoft()
	return nil
}

// GetSiteWalletBalance returns a sub-site's current wallet balance straight from the DB
// (never the domain cache, whose copy may be stale after wallet changes).
func GetSiteWalletBalance(siteId int) (int64, error) {
	var site Site
	if err := DB.Select("wallet_balance").First(&site, "id = ?", siteId).Error; err != nil {
		return 0, err
	}
	return site.WalletBalance, nil
}

// GetSiteWalletLogs returns a page of a sub-site's wallet flow records, newest first,
// optionally filtered by flow type (logType <= 0 means all types).
func GetSiteWalletLogs(siteId int, logType int, startIdx, num int) ([]*SiteWalletLog, int64, error) {
	query := DB.Model(&SiteWalletLog{}).Where("site_id = ?", siteId)
	if logType > 0 {
		query = query.Where("type = ?", logType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []*SiteWalletLog
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// SumSiteWalletLogAmount returns the sum of all flow amounts for a site, used by the
// reconciliation check (must equal the site's wallet_balance).
func SumSiteWalletLogAmount(siteId int) (int64, error) {
	var sum *int64
	if err := DB.Model(&SiteWalletLog{}).Where("site_id = ?", siteId).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum).Error; err != nil {
		return 0, err
	}
	if sum == nil {
		return 0, nil
	}
	return *sum, nil
}
