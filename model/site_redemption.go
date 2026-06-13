package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

// GenerateRedemptions creates `count` redemption codes for a site inside a single
// transaction. For a sub-site (siteId > 0) it FIRST atomically debits the owning
// wallet by costPerCode*count (flow type=2); if the balance is insufficient the whole
// batch fails and nothing is written — code generation and the wallet debit are
// inseparable. Main-site codes (siteId = 0) are free (no wallet). Each code records its
// own cost_amount so a later void can refund exactly the原路 amount.
func GenerateRedemptions(siteId, userId int, name string, quota, count int, expiredTime int64, costPerCode int64) ([]string, error) {
	if count <= 0 {
		return nil, errors.New("数量必须为正")
	}
	if costPerCode < 0 {
		return nil, errors.New("成本不能为负")
	}
	keys := make([]string, 0, count)
	err := DB.Transaction(func(tx *gorm.DB) error {
		if siteId > 0 && costPerCode > 0 {
			total := costPerCode * int64(count)
			if err := DeductSiteWallet(tx, siteId, total, WalletLogTypeRedemptionGen, "", "生成兑换码: "+name, userId); err != nil {
				return err
			}
		}
		now := common.GetTimestamp()
		for i := 0; i < count; i++ {
			key := common.GetUUID()
			r := &Redemption{
				SiteId:      siteId,
				UserId:      userId,
				Name:        name,
				Key:         key,
				Status:      common.RedemptionCodeStatusEnabled,
				Quota:       quota,
				CostAmount:  costPerCode,
				CreatedTime: now,
				ExpiredTime: expiredTime,
			}
			if err := tx.Create(r).Error; err != nil {
				return err
			}
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// VoidRedemption disables an UNUSED redemption code and refunds its cost_amount back to
// the owning sub-site's wallet (flow type=3), atomically. Only codes in the Enabled
// state can be voided. siteScope (an EffectiveSiteScope value) enforces ownership: a
// scoped operator may only void codes belonging to their own site. operatorUserId is
// recorded on the refund flow.
func VoidRedemption(id int, siteScope int, operatorUserId int) error {
	if id == 0 {
		return errors.New("id 为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var r Redemption
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&r, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("兑换码不存在")
			}
			return err
		}
		// Ownership: a scoped operator can only touch their own site's codes.
		if siteScope != SiteScopeAll && r.SiteId != siteScope {
			return errors.New("无权操作其他子站的兑换码")
		}
		if r.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("只能作废未使用的兑换码")
		}
		// Mark disabled.
		if err := tx.Model(&Redemption{}).Where("id = ?", id).
			Update("status", common.RedemptionCodeStatusDisabled).Error; err != nil {
			return err
		}
		// Refund the original cost back to the sub-site wallet (原路退).
		if r.SiteId > 0 && r.CostAmount > 0 {
			if err := AddSiteWallet(tx, r.SiteId, r.CostAmount, WalletLogTypeRedemptionVoid,
				"redemption:"+strconv.Itoa(id), "作废兑换码退款: "+r.Name, operatorUserId); err != nil {
				return err
			}
		}
		return nil
	})
}

// RedeemForSite redeems a code while enforcing that the code belongs to the request's
// site. A code generated on site A is invalid on site B (and vice-versa); the error is
// deliberately the same "invalid code" message so existence does not leak across sites.
// It otherwise mirrors model.Redeem (credits the user's quota and marks the code used).
func RedeemForSite(key string, userId int, siteId int) (quota int, err error) {
	if key == "" {
		return 0, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return 0, errors.New("无效的 user id")
	}
	redemption := &Redemption{}
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error; err != nil {
			return errors.New("无效的兑换码")
		}
		// Cross-site isolation: the code must belong to the redeeming site.
		if redemption.SiteId != siteId {
			return errors.New("无效的兑换码")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error; err != nil {
			return err
		}
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		return tx.Save(redemption).Error
	})
	if err != nil {
		// Mirror model.Redeem: log the specific cause, return a generic error so the
		// reason (invalid / used / expired / cross-site) is not leaked to the client.
		common.SysError("redemption failed: " + err.Error())
		return 0, ErrRedeemFailed
	}
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return redemption.Quota, nil
}

// ReconcileResult is one site's wallet reconciliation outcome.
type ReconcileResult struct {
	SiteId      int   `json:"site_id"`
	Balance     int64 `json:"balance"`
	LedgerSum   int64 `json:"ledger_sum"`
	Consistent  bool  `json:"consistent"`
	Discrepancy int64 `json:"discrepancy"` // balance - ledger_sum
}

// ReconcileSiteWallets verifies, for every sub-site, that wallet_balance equals the sum
// of its wallet flow records (every balance change is ledger-backed, so they must match).
// Returns one result per site; callers can alert on any Consistent==false.
func ReconcileSiteWallets() ([]ReconcileResult, error) {
	var sites []Site
	if err := DB.Select("id, wallet_balance").Find(&sites).Error; err != nil {
		return nil, err
	}
	results := make([]ReconcileResult, 0, len(sites))
	for _, s := range sites {
		sum, err := SumSiteWalletLogAmount(s.Id)
		if err != nil {
			return nil, err
		}
		results = append(results, ReconcileResult{
			SiteId:      s.Id,
			Balance:     s.WalletBalance,
			LedgerSum:   sum,
			Consistent:  s.WalletBalance == sum,
			Discrepancy: s.WalletBalance - sum,
		})
	}
	return results, nil
}
