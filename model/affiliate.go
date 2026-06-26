package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Affiliate commission ledger kinds.
const (
	// AffiliateKindFirstBonus is the one-time fixed reward granted when an invited
	// user makes their FIRST successful (real-payment) top-up. At most one per invitee.
	AffiliateKindFirstBonus = "first_bonus"
	// AffiliateKindRechargeCommission is the percentage commission granted to the
	// inviter on EVERY successful (real-payment) top-up the invitee makes. At most one
	// per trade_no.
	AffiliateKindRechargeCommission = "recharge_commission"
)

// AffiliateCommission is a per-event referral ledger. It powers idempotent settlement
// (composite unique index on trade_no+kind) and the per-user contribution leaderboard.
type AffiliateCommission struct {
	Id        int    `json:"id"`
	InviterId int    `json:"inviter_id" gorm:"index"`
	InviteeId int    `json:"invitee_id" gorm:"index"`
	TradeNo   string `json:"trade_no" gorm:"type:varchar(191);uniqueIndex:idx_aff_comm_trade_kind,priority:1"`
	Kind      string `json:"kind" gorm:"type:varchar(32);uniqueIndex:idx_aff_comm_trade_kind,priority:2"`
	// RechargeQuota is the quota credited to the invitee by this top-up (0 for first_bonus).
	RechargeQuota int64 `json:"recharge_quota" gorm:"type:bigint;not null;default:0"`
	// CommissionQuota is the quota credited to the inviter for this ledger row.
	CommissionQuota int64 `json:"commission_quota" gorm:"type:bigint;not null;default:0"`
	CreatedAt       int64 `json:"created_at" gorm:"type:bigint;index"`
}

// affiliateFirstBonusKey is the deterministic trade key used for a first_bonus ledger row.
// Keying first_bonus on the invitee (not the originating trade_no) lets the existing
// composite unique index (trade_no, kind) enforce "exactly one first_bonus per invitee" at
// the DB level, which a non-atomic existence COUNT cannot do when two concurrent first
// top-ups carry different trade_nos. The originating trade_no for the first qualifying
// recharge is still captured by that recharge's recharge_commission row.
func affiliateFirstBonusKey(inviteeId int) string {
	return fmt.Sprintf("first_bonus:%d", inviteeId)
}

// SettleReferralOnTopUp settles referral rewards for an invitee's successful, real-payment
// top-up. It is safe to call after the gateway's own crediting transaction has committed,
// and it is idempotent (webhook retries / concurrent callbacks never double-pay):
//   - first_bonus: the fixed QuotaForInviter (to inviter aff_quota) + QuotaForInvitee (to
//     invitee real quota) are granted exactly once, on the invitee's first qualifying top-up.
//   - recharge_commission: the inviter's effective commission rate (a per-user override on the
//     inviter when set, otherwise the global AffRechargeCommissionPercent) of the credited quota
//     is granted to the inviter on every qualifying top-up (including the first).
//
// creditedQuota is the quota actually added to the invitee by this top-up. Returns an error
// only on unexpected DB failures; callers should log and continue (a settlement failure must
// not roll back or block the top-up itself).
func SettleReferralOnTopUp(inviteeId int, tradeNo string, creditedQuota int64, paymentProvider string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if inviteeId <= 0 || tradeNo == "" || creditedQuota <= 0 {
		return nil
	}

	var invitee User
	if err := DB.Select("id, inviter_id").Where("id = ?", inviteeId).First(&invitee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	inviterId := invitee.InviterId
	// No inviter, or self-invite guard.
	if inviterId <= 0 || inviterId == inviteeId {
		return nil
	}

	// Payouts only when the operator has confirmed the compliance terms. Mirrors the
	// gate used by the legacy registration rewards (model/user.go) and the option layer.
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return nil
	}

	// Inviter must still exist (could have been deleted after the relationship was set).
	// Fetching the inviter row here doubles as that existence check and loads the optional
	// per-user commission-rate override. By design a missing inviter voids the referral pair
	// (the fixed first bonus is a pair reward), so we skip settlement entirely.
	var inviter User
	if err := DB.Select("id, aff_commission_percent").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	inviterReward := int64(common.QuotaForInviter)
	inviteeReward := int64(common.QuotaForInvitee)

	firstBonusGranted, err := settleAffiliateFirstBonus(inviterId, inviteeId, inviterReward, inviteeReward)
	if err != nil {
		return err
	}

	// Resolve the effective recharge-commission rate: a per-inviter override takes precedence;
	// a nil override (the common case) inherits the global common.AffRechargeCommissionPercent.
	commissionPercent := common.AffRechargeCommissionPercent
	if inviter.AffCommissionPercent != nil {
		commissionPercent = *inviter.AffCommissionPercent
	}
	commission := affiliateCommissionQuota(creditedQuota, commissionPercent)
	commissionGranted, err := settleAffiliateRechargeCommission(inviterId, inviteeId, tradeNo, creditedQuota, commission)
	if err != nil {
		return err
	}

	// Side effects after all DB writes commit: refresh the invitee quota cache (we credited
	// real quota inside the first-bonus transaction) and record the audit logs.
	if firstBonusGranted {
		if inviteeReward > 0 {
			gopool.Go(func() {
				if cerr := cacheIncrUserQuota(inviteeId, inviteeReward); cerr != nil {
					common.SysLog("failed to refresh invitee quota cache after referral first bonus: " + cerr.Error())
				}
			})
			RecordLog(inviteeId, LogTypeSystem, fmt.Sprintf("首次充值，使用邀请码赠送 %s", logger.LogQuota(int(inviteeReward))))
		}
		if inviterReward > 0 {
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请用户首次充值，获得邀请奖励 %s", logger.LogQuota(int(inviterReward))))
		}
	}
	if commissionGranted > 0 {
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请返佣，被邀请用户充值返还 %s", logger.LogQuota(int(commissionGranted))))
	}
	return nil
}

// affiliateCommissionQuota computes floor(creditedQuota * percent / 100) using decimal to
// avoid float drift. percent is the effective rate already resolved by the caller (a per-inviter
// override or the global default). Returns 0 when percent is non-positive; a percent above 100 is
// clamped defensively (write-time validation already bounds overrides to 0-100).
func affiliateCommissionQuota(creditedQuota int64, percent float64) int64 {
	if percent <= 0 || creditedQuota <= 0 {
		return 0
	}
	if percent > 100 {
		percent = 100
	}
	return decimal.NewFromInt(creditedQuota).
		Mul(decimal.NewFromFloat(percent)).
		Div(decimal.NewFromInt(100)).
		Floor().
		IntPart()
}

// settleAffiliateFirstBonus grants the one-time fixed reward exactly once per invitee.
// The first_bonus ledger row (unique per invitee via trade_no+kind, plus an explicit
// existence check) is the idempotency guard. Returns true only when THIS call performed
// the grant (so the caller credits the cache / writes logs once).
func settleAffiliateFirstBonus(inviterId, inviteeId int, inviterReward, inviteeReward int64) (bool, error) {
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&AffiliateCommission{}).
			Where("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonus).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return errAffiliateAlreadySettled
		}
		// TradeNo is the per-invitee synthetic key (not the originating trade_no) so the
		// unique index — not the non-atomic COUNT above — is the integrity boundary that
		// blocks a concurrent second first_bonus carrying a different trade_no.
		row := &AffiliateCommission{
			InviterId:       inviterId,
			InviteeId:       inviteeId,
			TradeNo:         affiliateFirstBonusKey(inviteeId),
			Kind:            AffiliateKindFirstBonus,
			RechargeQuota:   0,
			CommissionQuota: inviterReward,
			CreatedAt:       common.GetTimestamp(),
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		// Always count the activated (paying) invitee, even if the fixed reward is 0.
		inviterUpdates := map[string]interface{}{
			"aff_count": gorm.Expr("aff_count + ?", 1),
		}
		if inviterReward > 0 {
			inviterUpdates["aff_quota"] = gorm.Expr("aff_quota + ?", inviterReward)
			inviterUpdates["aff_history"] = gorm.Expr("aff_history + ?", inviterReward)
		}
		if err := tx.Model(&User{}).Where("id = ?", inviterId).Updates(inviterUpdates).Error; err != nil {
			return err
		}
		if inviteeReward > 0 {
			if err := tx.Model(&User{}).Where("id = ?", inviteeId).
				Update("quota", gorm.Expr("quota + ?", inviteeReward)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return resolveAffiliateSettleResult(err, func() bool {
		return affiliateLedgerExists("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonus)
	})
}

// settleAffiliateRechargeCommission grants the percentage commission exactly once per
// trade_no. Returns the commission amount actually granted by THIS call (0 if skipped).
func settleAffiliateRechargeCommission(inviterId, inviteeId int, tradeNo string, creditedQuota, commission int64) (int64, error) {
	if commission <= 0 {
		return 0, nil
	}
	granted, err := func() (bool, error) {
		txErr := DB.Transaction(func(tx *gorm.DB) error {
			var existing int64
			if err := tx.Model(&AffiliateCommission{}).
				Where("trade_no = ? AND kind = ?", tradeNo, AffiliateKindRechargeCommission).
				Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				return errAffiliateAlreadySettled
			}
			row := &AffiliateCommission{
				InviterId:       inviterId,
				InviteeId:       inviteeId,
				TradeNo:         tradeNo,
				Kind:            AffiliateKindRechargeCommission,
				RechargeQuota:   creditedQuota,
				CommissionQuota: commission,
				CreatedAt:       common.GetTimestamp(),
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			return tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]interface{}{
				"aff_quota":   gorm.Expr("aff_quota + ?", commission),
				"aff_history": gorm.Expr("aff_history + ?", commission),
			}).Error
		})
		return resolveAffiliateSettleResult(txErr, func() bool {
			return affiliateLedgerExists("trade_no = ? AND kind = ?", tradeNo, AffiliateKindRechargeCommission)
		})
	}()
	if err != nil {
		return 0, err
	}
	if !granted {
		return 0, nil
	}
	return commission, nil
}

var errAffiliateAlreadySettled = errors.New("affiliate already settled")

// resolveAffiliateSettleResult interprets a settlement transaction outcome. A clean commit
// means this call performed the grant. errAffiliateAlreadySettled means a prior row existed.
// Any other error is treated as a possible concurrent unique-index conflict: if the row now
// exists, the peer settled it (no error, not granted); otherwise the error is real.
func resolveAffiliateSettleResult(err error, exists func() bool) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errAffiliateAlreadySettled) {
		return false, nil
	}
	if exists() {
		return false, nil
	}
	return false, err
}

func affiliateLedgerExists(query string, args ...interface{}) bool {
	var cnt int64
	if err := DB.Model(&AffiliateCommission{}).Where(query, args...).Count(&cnt).Error; err != nil {
		return false
	}
	return cnt > 0
}

// AffStats is the aggregate referral summary surfaced on the user referral dashboard.
type AffStats struct {
	AffCode              string `json:"aff_code"`
	PendingQuota         int64  `json:"pending_quota"`          // aff_quota (transferable)
	TotalEarnedQuota     int64  `json:"total_earned_quota"`     // aff_history (lifetime)
	ActivatedCount       int    `json:"activated_count"`        // invitees who made a first top-up
	TotalInvited         int64  `json:"total_invited"`          // all users who registered with this code
	MonthCommissionQuota int64  `json:"month_commission_quota"` // commission earned this calendar month
}

// GetAffStats returns the referral summary for a user.
func GetAffStats(userId int) (*AffStats, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var user User
	if err := DB.Select("aff_code, aff_quota, aff_history, aff_count").
		Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	var totalInvited int64
	if err := DB.Model(&User{}).Where("inviter_id = ?", userId).Count(&totalInvited).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	var monthCommission int64
	if err := DB.Model(&AffiliateCommission{}).
		Where("inviter_id = ? AND created_at >= ?", userId, monthStart).
		Select("COALESCE(SUM(commission_quota), 0)").
		Scan(&monthCommission).Error; err != nil {
		return nil, err
	}
	return &AffStats{
		AffCode:              user.AffCode,
		PendingQuota:         int64(user.AffQuota),
		TotalEarnedQuota:     int64(user.AffHistoryQuota),
		ActivatedCount:       user.AffCount,
		TotalInvited:         totalInvited,
		MonthCommissionQuota: monthCommission,
	}, nil
}

// AffLeaderboardItem is one row of the personal contribution leaderboard.
type AffLeaderboardItem struct {
	InviteeId       int    `json:"invitee_id"`
	Username        string `json:"username"` // masked
	CommissionQuota int64  `json:"commission_quota"`
	RechargeCount   int    `json:"recharge_count"`
	LastAt          int64  `json:"last_at"`
}

// GetAffLeaderboard returns the current user's invitees ranked by the total commission they
// contributed. Usernames are masked for privacy.
func GetAffLeaderboard(userId int, limit int) ([]AffLeaderboardItem, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	type aggRow struct {
		InviteeId int   `gorm:"column:invitee_id"`
		Total     int64 `gorm:"column:total"`
		Cnt       int   `gorm:"column:cnt"`
		LastAt    int64 `gorm:"column:last_at"`
	}
	var rows []aggRow
	if err := DB.Model(&AffiliateCommission{}).
		Select("invitee_id, "+
			"COALESCE(SUM(commission_quota), 0) AS total, "+
			"SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END) AS cnt, "+
			"COALESCE(MAX(created_at), 0) AS last_at", AffiliateKindRechargeCommission).
		Where("inviter_id = ?", userId).
		Group("invitee_id").
		Order("total DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []AffLeaderboardItem{}, nil
	}
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.InviteeId)
	}
	var users []User
	if err := DB.Select("id, username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	nameMap := make(map[int]string, len(users))
	for _, u := range users {
		nameMap[u.Id] = u.Username
	}
	items := make([]AffLeaderboardItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, AffLeaderboardItem{
			InviteeId:       r.InviteeId,
			Username:        maskUsername(nameMap[r.InviteeId]),
			CommissionQuota: r.Total,
			RechargeCount:   r.Cnt,
			LastAt:          r.LastAt,
		})
	}
	return items, nil
}

// maskUsername hides the middle of a username for the public-facing leaderboard.
func maskUsername(name string) string {
	r := []rune(strings.TrimSpace(name))
	switch n := len(r); {
	case n == 0:
		return "***"
	case n <= 2:
		return string(r[0]) + "***"
	case n <= 4:
		return string(r[0:1]) + "***" + string(r[n-1:])
	default:
		return string(r[0:2]) + "***" + string(r[n-1:])
	}
}
