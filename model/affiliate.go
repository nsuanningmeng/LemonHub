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
	// AffiliateKindFirstBonusReversal marks that an invitee's first_bonus was reversed
	// because their activating top-up was fully refunded/disputed and they retain no
	// other net-credited top-up. Keyed by the same synthetic first-bonus trade_no, so the
	// composite unique index enforces "reversed at most once per invitee".
	AffiliateKindFirstBonusReversal = "first_bonus_reversal"
)

// AffiliateCommission is a per-event referral ledger. It powers idempotent settlement
// (composite unique index on trade_no+kind) and the per-user contribution leaderboard.
type AffiliateCommission struct {
	Id        int    `json:"id"`
	InviterId int    `json:"inviter_id" gorm:"index"`
	InviteeId int    `json:"invitee_id" gorm:"index"`
	TradeNo   string `json:"trade_no" gorm:"type:varchar(191);uniqueIndex:idx_aff_comm_trade_kind,priority:1"`
	Kind      string `json:"kind" gorm:"type:varchar(32);uniqueIndex:idx_aff_comm_trade_kind,priority:2"`
	// RechargeQuota is the quota credited to the invitee by this top-up. For a first_bonus row
	// it instead records the invitee's one-time sign-up reward, so that reward can be reversed
	// exactly if the activating top-up is later refunded (legacy first_bonus rows hold 0 and fall
	// back to the current config value).
	RechargeQuota int64 `json:"recharge_quota" gorm:"type:bigint;not null;default:0"`
	// CommissionQuota is the quota currently credited to the inviter for this ledger row.
	// It is REDUCED when the originating top-up is later refunded/disputed, so every SUM over
	// commission_quota (wallet stats, month, leaderboard, cash-owed) nets the reversal with no
	// query changes. The original grant is commission_quota + reversed_quota.
	CommissionQuota int64 `json:"commission_quota" gorm:"type:bigint;not null;default:0"`
	// ReversedQuota is the cumulative amount of this row's commission that has been clawed back
	// because the originating top-up was refunded/disputed. Kept separately (rather than only
	// shrinking CommissionQuota) so reversal is idempotent — the target reversal is recomputed
	// from the original grant (commission_quota + reversed_quota) on every webhook delivery.
	ReversedQuota int64 `json:"reversed_quota" gorm:"type:bigint;not null;default:0"`
	// CashSettled marks a recharge_commission row whose amount was NOT credited to the inviter's
	// platform wallet because the inviter was a cash-settled promoter when it settled — i.e. it is
	// owed as off-platform cash. false (the default, and the value of every pre-existing row) means
	// the commission was credited to the wallet as usual, so the cash-owed total can sum only the
	// CashSettled rows with no backfill, and an inviter toggled to cash mode mid-life only accrues
	// cash from that point on. Not meaningful on first_bonus rows (a suppressed bonus is never cash).
	CashSettled bool  `json:"cash_settled" gorm:"column:cash_settled"`
	CreatedAt   int64 `json:"created_at" gorm:"type:bigint;index"`
}

// AffiliateCashPayout records one off-platform cash settlement an operator paid to a cash-settled
// promoter, in the same quota unit as the commission ledger. It is the "已结清" watermark: the
// outstanding cash owed for an inviter is SUM(cash_settled recharge commission) - SUM(payouts), so
// the same commission is never settled twice and repeated payouts just accumulate here.
type AffiliateCashPayout struct {
	Id         int    `json:"id"`
	InviterId  int    `json:"inviter_id" gorm:"index"`
	Amount     int64  `json:"amount" gorm:"type:bigint;not null;default:0"` // quota settled this batch
	Note       string `json:"note" gorm:"type:varchar(255)"`
	OperatorId int    `json:"operator_id"` // admin user id who recorded the settlement (0 if unknown)
	CreatedAt  int64  `json:"created_at" gorm:"type:bigint;index"`
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
// When the inviter is a cash-settled promoter (User.AffCashSettled), the inviter-side first_bonus is
// suppressed (inviter reward 0; the invitee bonus still applies) and recharge_commission is still
// recorded in the ledger but NOT credited to the inviter's wallet — the ledger is then the basis for
// an off-platform cash payout.
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
	if err := DB.Select("id, aff_commission_percent, aff_cash_settled").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	inviterReward := int64(common.QuotaForInviter)
	inviteeReward := int64(common.QuotaForInvitee)
	// Cash-settled promoters are paid off-platform in cash (computed from the commission ledger),
	// so the platform first bonus to the inviter is suppressed. The invitee's own bonus still
	// applies — invitee acquisition incentive is independent of how the inviter is settled.
	if inviter.AffCashSettled {
		inviterReward = 0
	}

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
	// Cash-settled promoters: record the commission in the ledger (it is the off-platform cash basis)
	// but do NOT credit it to their platform wallet. Normal inviters: credit aff_quota as before.
	creditCommissionToWallet := !inviter.AffCashSettled
	commissionGranted, err := settleAffiliateRechargeCommission(inviterId, inviteeId, tradeNo, creditedQuota, commission, creditCommissionToWallet)
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
		if creditCommissionToWallet {
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请返佣，被邀请用户充值返还 %s", logger.LogQuota(int(commissionGranted))))
		} else {
			// Cash-settled promoter: the amount was recorded in the ledger as the cash basis, not
			// credited to the platform wallet. Make that explicit in the audit trail.
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("推广返佣（现金结算）记账 %s，未计入平台额度", logger.LogQuota(int(commissionGranted))))
		}
	}
	return nil
}

var errAffiliateReversalRaceLost = errors.New("affiliate reversal lost the compare-and-set")

const maxAffiliateReversalAttempts = 5

// ReverseReferralOnTopUpClawback reverses the referral rewards a top-up generated, when that
// top-up is later refunded or disputed. It mirrors SettleReferralOnTopUp:
//
//   - Recharge commission: reduced proportionally to how much of the top-up was clawed back
//     (target = floor(originalCommission * clawedBackTotal / rechargeQuota)). A wallet-credited
//     commission debits the inviter's aff_quota/aff_history by the delta (allowed to go negative,
//     matching the soft-quota model); a cash-settled promoter's commission only shrinks in the
//     ledger, which reduces the off-platform cash owed. Idempotent across partial/duplicate
//     deliveries via the row's reversed_quota (compare-and-set).
//   - First bonus: reversed only when the invitee is fully DEACTIVATED — this top-up is fully
//     clawed back AND the invitee retains no other net-credited top-up. The inviter's one-time
//     bonus + aff_count and the invitee's sign-up reward are reversed exactly once (guarded by a
//     first_bonus_reversal ledger row).
//
// clawedBackTotal is the cumulative quota clawed back from THIS top-up so far; creditedQuota is
// the quota it originally credited. Like SettleReferralOnTopUp, errors are returned for the caller
// to log — they must NOT roll back or block the clawback itself.
func ReverseReferralOnTopUpClawback(inviteeId int, tradeNo string, clawedBackTotal, creditedQuota int64, callerIp string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if inviteeId <= 0 || tradeNo == "" || clawedBackTotal <= 0 {
		return nil
	}

	if err := reverseAffiliateRechargeCommission(tradeNo, clawedBackTotal, creditedQuota); err != nil {
		return err
	}

	// The one-time first bonus only reverses when the invitee's activation is fully void.
	if clawedBackTotal < creditedQuota {
		return nil
	}
	stillCredited, err := inviteeHasNetCreditedTopUp(inviteeId)
	if err != nil {
		return err
	}
	if stillCredited {
		return nil
	}
	return reverseAffiliateFirstBonus(inviteeId)
}

// reverseAffiliateRechargeCommission shrinks the recharge_commission row for tradeNo so it reflects
// the invitee's clawed-back top-up, debiting the inviter wallet by the reversed delta (unless the
// commission was cash-settled, where only the ledger amount shrinks). The reversal target is
// recomputed from the ORIGINAL grant (commission_quota + reversed_quota) each call, so partial and
// duplicate refund deliveries converge without double-reversing.
func reverseAffiliateRechargeCommission(tradeNo string, clawedBackTotal, creditedFallback int64) error {
	for attempt := 0; attempt < maxAffiliateReversalAttempts; attempt++ {
		raceLost := false
		var inviterId int
		var delta int64
		var cashSettled bool
		err := DB.Transaction(func(tx *gorm.DB) error {
			var row AffiliateCommission
			if e := tx.Where("trade_no = ? AND kind = ?", tradeNo, AffiliateKindRechargeCommission).First(&row).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return nil // no commission was paid for this trade (no inviter / 0%)
				}
				return e
			}
			// RechargeQuota is the commission's proportion base. A corrupt/legacy row may hold 0;
			// fall back to the order's credited quota so a positive commission is still reversed.
			base := row.RechargeQuota
			if base <= 0 {
				base = creditedFallback
			}
			if base <= 0 {
				return nil
			}
			clawed := clawedBackTotal
			if clawed > base {
				clawed = base
			}
			original := row.CommissionQuota + row.ReversedQuota
			target := decimal.NewFromInt(original).Mul(decimal.NewFromInt(clawed)).Div(decimal.NewFromInt(base)).Floor().IntPart()
			d := target - row.ReversedQuota
			if d <= 0 {
				return nil
			}
			claim := tx.Model(&AffiliateCommission{}).
				Where("id = ? AND reversed_quota = ?", row.Id, row.ReversedQuota).
				Updates(map[string]interface{}{
					"commission_quota": gorm.Expr("commission_quota - ?", d),
					"reversed_quota":   gorm.Expr("reversed_quota + ?", d),
				})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected == 0 {
				raceLost = true
				return errAffiliateReversalRaceLost
			}
			if !row.CashSettled {
				if e := tx.Model(&User{}).Where("id = ?", row.InviterId).Updates(map[string]interface{}{
					"aff_quota":   gorm.Expr("aff_quota - ?", d),
					"aff_history": gorm.Expr("aff_history - ?", d),
				}).Error; e != nil {
					return e
				}
			}
			inviterId, delta, cashSettled = row.InviterId, d, row.CashSettled
			return nil
		})
		if raceLost {
			continue
		}
		if err != nil {
			return err
		}
		if delta > 0 {
			if !cashSettled {
				_ = invalidateUserCache(inviterId)
			}
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请返佣回扣 -%s（订单 %s 退款/拒付）", logger.LogQuota(int(delta)), tradeNo))
		}
		return nil
	}
	return errAffiliateReversalRaceLost
}

// reverseAffiliateFirstBonus reverses an invitee's one-time first bonus exactly once, when their
// activation is fully void. It debits the inviter's aff_quota/aff_history/aff_count and the
// invitee's sign-up reward, guarded by an inserted first_bonus_reversal ledger row (the composite
// unique index blocks a second reversal).
func reverseAffiliateFirstBonus(inviteeId int) error {
	var inviterId int
	var inviterReward, inviteeReward int64
	granted, err := func() (bool, error) {
		txErr := DB.Transaction(func(tx *gorm.DB) error {
			var fb AffiliateCommission
			if e := tx.Where("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonus).First(&fb).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return errAffiliateAlreadySettled // no first bonus was ever granted: nothing to reverse
				}
				return e
			}
			var existing int64
			if e := tx.Model(&AffiliateCommission{}).
				Where("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonusReversal).
				Count(&existing).Error; e != nil {
				return e
			}
			if existing > 0 {
				return errAffiliateAlreadySettled // already reversed
			}

			inviterReward = fb.CommissionQuota
			// Only reverse the invitee sign-up reward that was actually recorded at grant time.
			// Legacy first_bonus rows (and intentionally-0 grants) carry RechargeQuota<=0, so we
			// reverse nothing rather than guess with the current common.QuotaForInvitee (which may
			// have changed since and would over/under-debit the invitee).
			inviteeReward = fb.RechargeQuota
			if inviteeReward < 0 {
				inviteeReward = 0
			}

			// The reversal marker's unique index (trade_no, kind) is the hard idempotency boundary
			// that blocks a concurrent second reversal the non-atomic COUNT above cannot. It is dated
			// at the ORIGINAL grant time so the kind-agnostic month-commission SUMs (user + admin
			// dashboards) attribute the negative to the month the bonus was earned, not "now".
			marker := &AffiliateCommission{
				InviterId:       fb.InviterId,
				InviteeId:       inviteeId,
				TradeNo:         affiliateFirstBonusKey(inviteeId),
				Kind:            AffiliateKindFirstBonusReversal,
				RechargeQuota:   -inviteeReward,
				CommissionQuota: -inviterReward, // negative so kind-agnostic SUMs net the bonus out
				CreatedAt:       fb.CreatedAt,
			}
			if e := tx.Create(marker).Error; e != nil {
				return e
			}

			inviterUpdates := map[string]interface{}{
				"aff_count": gorm.Expr("aff_count - ?", 1),
			}
			if inviterReward > 0 {
				inviterUpdates["aff_quota"] = gorm.Expr("aff_quota - ?", inviterReward)
				inviterUpdates["aff_history"] = gorm.Expr("aff_history - ?", inviterReward)
			}
			if e := tx.Model(&User{}).Where("id = ?", fb.InviterId).Updates(inviterUpdates).Error; e != nil {
				return e
			}
			if inviteeReward > 0 {
				if e := tx.Model(&User{}).Where("id = ?", inviteeId).
					Update("quota", gorm.Expr("quota - ?", inviteeReward)).Error; e != nil {
					return e
				}
			}
			inviterId = fb.InviterId
			return nil
		})
		return resolveAffiliateSettleResult(txErr, func() bool {
			return affiliateLedgerExists("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonusReversal)
		})
	}()
	if err != nil {
		return err
	}
	if granted {
		_ = invalidateUserCache(inviterId)
		_ = invalidateUserCache(inviteeId)
		RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请首充奖励回扣 -%s（被邀请用户激活充值已全额退款/拒付）", logger.LogQuota(int(inviterReward))))
		if inviteeReward > 0 {
			RecordLog(inviteeId, LogTypeSystem, fmt.Sprintf("首充邀请赠送回扣 -%s（激活充值已全额退款/拒付）", logger.LogQuota(int(inviteeReward))))
		}
	}
	return nil
}

// inviteeHasNetCreditedTopUp reports whether the invitee still has at least one settled top-up
// (any provider) that retains net-credited quota (credited minus clawed-back > 0). Used to decide
// whether a fully-refunded activating top-up fully DEACTIVATES the invitee.
func inviteeHasNetCreditedTopUp(inviteeId int) (bool, error) {
	var rows []TopUp
	if err := DB.Select("payment_provider, amount, money, clawed_back_quota").
		Where("user_id = ? AND status IN ?", inviteeId,
			[]string{common.TopUpStatusSuccess, common.TopUpStatusRefunded, common.TopUpStatusDisputed}).
		Find(&rows).Error; err != nil {
		return false, err
	}
	for _, r := range rows {
		if topUpCreditedQuota(r)-r.ClawedBackQuota > 0 {
			return true, nil
		}
	}
	return false, nil
}

// topUpCreditedQuota returns the user quota a settled top-up actually credited, using the SAME
// per-provider formula as the Recharge* settlement functions (Stripe: Money*QuotaPerUnit; Creem:
// Amount; Waffo/WaffoPancake/epay: Amount*QuotaPerUnit). Recomputing from Money alone is
// Stripe-only and would misjudge Creem promo (Money=0, Amount>0) or subscription-generated rows
// (Amount=0, Money>0, empty provider, credit no balance) — both of which flip the deactivation gate.
func topUpCreditedQuota(t TopUp) int64 {
	switch t.PaymentProvider {
	case PaymentProviderStripe:
		return decimal.NewFromFloat(t.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	case PaymentProviderCreem:
		return t.Amount
	case PaymentProviderWaffo, PaymentProviderWaffoPancake, PaymentProviderEpay:
		return decimal.NewFromInt(t.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	default:
		if t.Amount <= 0 {
			return 0 // subscription/balance rows credit no top-up quota
		}
		return decimal.NewFromInt(t.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
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
		// If a prior activation was fully refunded, its first_bonus was reversed and a
		// first_bonus_reversal marker written. A genuine re-activation (a fresh paid top-up)
		// should re-earn the bonus, so clear the stale reversed cycle (both rows) first — the
		// unique index would otherwise block re-inserting the first_bonus row.
		var reversedMarkers int64
		if err := tx.Model(&AffiliateCommission{}).
			Where("invitee_id = ? AND kind = ?", inviteeId, AffiliateKindFirstBonusReversal).
			Count(&reversedMarkers).Error; err != nil {
			return err
		}
		if reversedMarkers > 0 {
			if err := tx.Where("invitee_id = ? AND kind IN ?", inviteeId,
				[]string{AffiliateKindFirstBonus, AffiliateKindFirstBonusReversal}).
				Delete(&AffiliateCommission{}).Error; err != nil {
				return err
			}
		}

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
			RechargeQuota:   inviteeReward, // stored so it can be reversed exactly on refund
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

// settleAffiliateRechargeCommission records the percentage commission exactly once per trade_no.
// When creditPlatformQuota is true the commission is also credited to the inviter's aff_quota/
// aff_history (normal inviters); when false the ledger row is still written but the wallet is left
// untouched (cash-settled promoters: the ledger row is the off-platform cash basis). Returns the
// commission amount RECORDED by THIS call (0 if skipped or already settled).
func settleAffiliateRechargeCommission(inviterId, inviteeId int, tradeNo string, creditedQuota, commission int64, creditPlatformQuota bool) (int64, error) {
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
				// Owed as cash exactly when it was not credited to the platform wallet.
				CashSettled: !creditPlatformQuota,
				CreatedAt:   common.GetTimestamp(),
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			// Cash-settled promoter: ledger-only. The row above is the cash basis; do not credit the
			// platform wallet (avoids double-paying cash + platform quota).
			if !creditPlatformQuota {
				return nil
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
	// CommissionPercent is the effective recharge-commission rate (0-100) applied to this
	// user's invitees: the per-inviter aff_commission_percent override when set, otherwise the
	// global common.AffRechargeCommissionPercent. Surfaced so the referral dashboard can show
	// "you earn X% on every top-up" without the client guessing the rate.
	CommissionPercent float64 `json:"commission_percent"`
	// IsCashSettled marks this user as a cash-settled promoter (aff_cash_settled). When true the
	// dashboard must render the cash-settlement variant: PendingQuota/TotalEarnedQuota are
	// structurally 0 (commission is never credited to the wallet) and the cash fields below carry
	// the real off-platform balance instead. Normal inviters get false and all-zero cash fields.
	IsCashSettled bool `json:"is_cash_settled"`
	// CashCommissionTotal/Paid/Owed are only meaningful for cash-settled promoters (0 otherwise):
	// the gross recharge commission recorded as off-platform cash, the amount already settled, and
	// the outstanding balance (total - paid, clamped >= 0). Mirrors the admin leaderboard's cash math.
	CashCommissionTotal int64 `json:"cash_commission_total"`
	CashCommissionPaid  int64 `json:"cash_commission_paid"`
	CashCommissionOwed  int64 `json:"cash_commission_owed"`
}

// GetAffStats returns the referral summary for a user.
func GetAffStats(userId int) (*AffStats, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var user User
	if err := DB.Select("aff_code, aff_quota, aff_history, aff_count, aff_commission_percent, aff_cash_settled, aff_cash_paid").
		Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	// Effective recharge-commission rate: per-inviter override wins, else the global default.
	// Mirrors the resolution in SettleReferralOnTopUp so the dashboard shows the real rate.
	commissionPercent := common.AffRechargeCommissionPercent
	if user.AffCommissionPercent != nil {
		commissionPercent = *user.AffCommissionPercent
	}
	// Cash-settled promoters: surface their off-platform cash balance so the dashboard can replace
	// the always-zero pending/earned wallet tiles with the real owed/paid figures. Mirrors the admin
	// leaderboard math (cash basis = cash_settled ledger rows; owed = total - paid, clamped >= 0).
	var cashTotal, cashPaid, cashOwed int64
	if user.AffCashSettled {
		// The cash balance is a secondary stat. If its aggregation fails — e.g. a database whose
		// affiliate_commissions.cash_settled column has not been migrated yet (a slave node skips
		// migrations) — degrade to a zero balance and keep serving the dashboard rather than 500ing
		// the whole referral page. ensureAffiliateCashSettledColumn restores the real figures on the
		// next migrating startup.
		if total, terr := affiliateCashCommissionTotal(DB, userId); terr != nil {
			common.SysLog("GetAffStats: cash commission aggregation failed, serving zero cash balance: " + terr.Error())
		} else {
			cashTotal = total
			cashPaid = user.AffCashPaid
			cashOwed = cashTotal - cashPaid
			if cashOwed < 0 {
				cashOwed = 0
			}
		}
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
		CommissionPercent:    commissionPercent,
		IsCashSettled:        user.AffCashSettled,
		CashCommissionTotal:  cashTotal,
		CashCommissionPaid:   cashPaid,
		CashCommissionOwed:   cashOwed,
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

// AffAdminSummary is the site-wide referral overview surfaced on the admin global
// leaderboard. It is admin-only (the calling route is guarded by AdminAuth). The inviter
// running totals are read straight off the users table (aff_history / aff_quota / aff_count),
// while the current-month figure comes from the per-event ledger.
type AffAdminSummary struct {
	TotalCommissionPaid  int64 `json:"total_commission_paid"`  // SUM(aff_history) over all users (lifetime payout)
	TotalPendingQuota    int64 `json:"total_pending_quota"`    // SUM(aff_quota) over all users (un-transferred)
	TotalActivated       int64 `json:"total_activated"`        // SUM(aff_count) over all users (activated invitees)
	InviterCount         int64 `json:"inviter_count"`          // distinct users who invited at least one (live) person
	MonthCommissionQuota int64 `json:"month_commission_quota"` // commission credited this calendar month
}

// GetAffAdminSummary returns the site-wide referral overview for the admin dashboard.
func GetAffAdminSummary() (*AffAdminSummary, error) {
	summary := &AffAdminSummary{}

	var totals struct {
		Paid      int64
		Pending   int64
		Activated int64
	}
	if err := DB.Model(&User{}).
		Select("COALESCE(SUM(aff_history), 0) AS paid, " +
			"COALESCE(SUM(aff_quota), 0) AS pending, " +
			"COALESCE(SUM(aff_count), 0) AS activated").
		Scan(&totals).Error; err != nil {
		return nil, err
	}
	summary.TotalCommissionPaid = totals.Paid
	summary.TotalPendingQuota = totals.Pending
	summary.TotalActivated = totals.Activated

	// Number of inviters = distinct live users referenced as someone's inviter_id. Scoped the
	// same way as GetAffAdminLeaderboard's base set (id IN subquery) so this "Inviters" card
	// never disagrees with the rows the leaderboard can page through: a soft-deleted inviter
	// that still has live invitees is excluded from both, not counted here only.
	inviterIds := DB.Model(&User{}).Distinct("inviter_id").Where("inviter_id > 0")
	if err := DB.Model(&User{}).
		Where("id IN (?)", inviterIds).
		Count(&summary.InviterCount).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	if err := DB.Model(&AffiliateCommission{}).
		Where("created_at >= ?", monthStart).
		Select("COALESCE(SUM(commission_quota), 0)").
		Scan(&summary.MonthCommissionQuota).Error; err != nil {
		return nil, err
	}
	return summary, nil
}

// AffAdminLeaderboardItem is one inviter row of the site-wide referral leaderboard.
// Unlike the per-user board, usernames are NOT masked (this is an admin-only view).
type AffAdminLeaderboardItem struct {
	InviterId            int    `json:"inviter_id"`
	Username             string `json:"username"`
	DisplayName          string `json:"display_name"`
	TotalEarnedQuota     int64  `json:"total_earned_quota"`     // aff_history
	PendingQuota         int64  `json:"pending_quota"`          // aff_quota
	ActivatedCount       int    `json:"activated_count"`        // aff_count
	TotalInvited         int64  `json:"total_invited"`          // count of (live) users with inviter_id = this user
	MonthCommissionQuota int64  `json:"month_commission_quota"` // commission credited this calendar month
	CashCommissionTotal  int64  `json:"cash_commission_total"`  // lifetime recharge commission recorded as off-platform cash (uncredited); 0 for normal inviters
	CashCommissionPaid   int64  `json:"cash_commission_paid"`   // total cash already settled to this inviter (sum of payouts)
	CashCommissionOwed   int64  `json:"cash_commission_owed"`   // outstanding cash owed = total - paid (clamped >= 0)
	IsCashSettled        bool   `json:"is_cash_settled"`        // cash-settled promoter: payouts handled off-platform
	LastAt               int64  `json:"last_at"`                // latest ledger event time (0 if none)
}

// AffAdminLeaderboardQuery parameterizes the admin global leaderboard read.
type AffAdminLeaderboardQuery struct {
	Page     int
	PageSize int
	Keyword  string // username / display_name LIKE filter
	Sort     string // one of affAdminSortColumns keys; anything else falls back to total_earned
	Order    string // "asc" | "desc" (default desc)
}

// AffAdminLeaderboardResult is the paginated admin leaderboard payload.
type AffAdminLeaderboardResult struct {
	Items    []AffAdminLeaderboardItem `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

// affAdminSortColumns whitelists the user-native columns the admin leaderboard may sort on,
// mapping a stable API token to a physical column. Anything outside this map falls back to
// aff_history, which keeps the ORDER BY clause free of caller-controlled text (no injection).
var affAdminSortColumns = map[string]string{
	"total_earned": "aff_history",
	"pending":      "aff_quota",
	"activated":    "aff_count",
	"username":     "username",
}

// GetAffAdminLeaderboard returns the site-wide inviter leaderboard, paginated and optionally
// filtered by username/display_name. Inviters are all users referenced as someone's inviter_id
// (so inviters with no settled commission yet still appear). Server-side sorting covers the
// user-native columns in affAdminSortColumns (default: total earned, descending); the derived
// columns (total_invited / month / last_at) are enriched per page via two grouped queries so
// there is no N+1.
func GetAffAdminLeaderboard(q AffAdminLeaderboardQuery) (*AffAdminLeaderboardResult, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	sortCol, ok := affAdminSortColumns[q.Sort]
	if !ok {
		sortCol = "aff_history"
	}
	order := "DESC"
	if strings.EqualFold(q.Order, "asc") {
		order = "ASC"
	}

	// Candidate inviters: users referenced as someone's inviter_id. The subquery and the outer
	// query both run under GORM's default soft-delete scope, so deleted users are excluded on
	// both sides (we neither credit nor list deleted inviters/invitees).
	inviterIds := DB.Model(&User{}).Distinct("inviter_id").Where("inviter_id > 0")
	base := DB.Model(&User{}).Where("id IN (?)", inviterIds)
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + kw + "%"
		base = base.Where("username LIKE ? OR display_name LIKE ?", like, like)
	}

	result := &AffAdminLeaderboardResult{Page: page, PageSize: pageSize, Items: []AffAdminLeaderboardItem{}}
	if err := base.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	if result.Total == 0 {
		return result, nil
	}

	var users []User
	if err := base.
		Select("id, username, display_name, aff_history, aff_quota, aff_count, aff_cash_settled, aff_cash_paid").
		Order(sortCol + " " + order).
		Order("id ASC"). // stable tiebreaker -> deterministic pagination
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return result, nil
	}

	ids := make([]int, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}

	// Enrich (1/2): total invited per inviter — one grouped query over the page ids.
	invitedMap := make(map[int]int64, len(ids))
	{
		type invitedRow struct {
			InviterId int   `gorm:"column:inviter_id"`
			Cnt       int64 `gorm:"column:cnt"`
		}
		var rows []invitedRow
		if err := DB.Model(&User{}).
			Select("inviter_id, COUNT(*) AS cnt").
			Where("inviter_id IN ?", ids).
			Group("inviter_id").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			invitedMap[r.InviterId] = r.Cnt
		}
	}

	// Enrich (2/2): this-month commission + last activity per inviter — one grouped query.
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	type ledgerAgg struct {
		Month  int64
		Owed   int64
		LastAt int64
	}
	ledgerMap := make(map[int]ledgerAgg, len(ids))
	{
		type ledgerRow struct {
			InviterId int   `gorm:"column:inviter_id"`
			Month     int64 `gorm:"column:month"`
			Owed      int64 `gorm:"column:owed"`
			LastAt    int64 `gorm:"column:last_at"`
		}
		var rows []ledgerRow
		// owed = lifetime recharge commission recorded as cash (cash_settled rows) and therefore not
		// credited to the platform wallet. Summing only cash_settled rows means an inviter toggled to
		// cash mode mid-life never has their earlier (already wallet-credited) commission counted as
		// cash owed, and pre-existing rows (cash_settled=false) are correctly excluded.
		if err := DB.Model(&AffiliateCommission{}).
			Select("inviter_id, "+
				"COALESCE(SUM(CASE WHEN created_at >= ? THEN commission_quota ELSE 0 END), 0) AS month, "+
				"COALESCE(SUM(CASE WHEN kind = ? AND cash_settled = "+commonTrueVal+" THEN commission_quota ELSE 0 END), 0) AS owed, "+
				"COALESCE(MAX(created_at), 0) AS last_at", monthStart, AffiliateKindRechargeCommission).
			Where("inviter_id IN ?", ids).
			Group("inviter_id").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			ledgerMap[r.InviterId] = ledgerAgg{Month: r.Month, Owed: r.Owed, LastAt: r.LastAt}
		}
	}

	items := make([]AffAdminLeaderboardItem, 0, len(users))
	for _, u := range users {
		lg := ledgerMap[u.Id]
		// Paid is read from the authoritative per-user counter (advanced atomically by
		// RecordAffiliateCashPayout); outstanding = gross cash commission - paid, clamped >= 0.
		cashTotal := lg.Owed
		cashPaid := u.AffCashPaid
		cashOwed := cashTotal - cashPaid
		if cashOwed < 0 {
			cashOwed = 0
		}
		items = append(items, AffAdminLeaderboardItem{
			InviterId:            u.Id,
			Username:             u.Username,
			DisplayName:          u.DisplayName,
			TotalEarnedQuota:     int64(u.AffHistoryQuota),
			PendingQuota:         int64(u.AffQuota),
			ActivatedCount:       u.AffCount,
			TotalInvited:         invitedMap[u.Id],
			MonthCommissionQuota: lg.Month,
			CashCommissionTotal:  cashTotal,
			CashCommissionPaid:   cashPaid,
			CashCommissionOwed:   cashOwed,
			IsCashSettled:        u.AffCashSettled,
			LastAt:               lg.LastAt,
		})
	}
	result.Items = items
	return result, nil
}

// affiliateCashCommissionTotal returns the lifetime recharge commission recorded as off-platform
// cash (cash_settled rows) for an inviter — the gross amount owed before any settlement.
func affiliateCashCommissionTotal(tx *gorm.DB, inviterId int) (int64, error) {
	var total int64
	if err := tx.Model(&AffiliateCommission{}).
		Where("inviter_id = ? AND kind = ? AND cash_settled = "+commonTrueVal, inviterId, AffiliateKindRechargeCommission).
		Select("COALESCE(SUM(commission_quota), 0)").
		Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// RecordAffiliateCashPayout records an off-platform cash settlement to an inviter. amount is in the
// same quota unit as the commission ledger; it must be positive and not exceed the current
// outstanding owed (total cash-settled commission - already-settled). The outstanding is recomputed
// inside the transaction so concurrent settlements cannot over-pay. Returns the persisted payout row.
func RecordAffiliateCashPayout(inviterId int, amount int64, note string, operatorId int) (*AffiliateCashPayout, error) {
	if inviterId <= 0 {
		return nil, errors.New("invalid inviter id")
	}
	if amount <= 0 {
		return nil, errors.New("settlement amount must be positive")
	}
	note = strings.TrimSpace(note)
	if r := []rune(note); len(r) > 255 {
		note = string(r[:255])
	}
	var payout *AffiliateCashPayout
	err := DB.Transaction(func(tx *gorm.DB) error {
		var inviter User
		if err := tx.Select("id, aff_cash_paid").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("inviter not found")
			}
			return err
		}
		total, err := affiliateCashCommissionTotal(tx, inviterId)
		if err != nil {
			return err
		}
		// Advance the authoritative paid counter with a CAPPED conditional UPDATE: the WHERE re-checks
		// the inviter's CURRENT aff_cash_paid (under the row write-lock the UPDATE takes), so two
		// concurrent settlements for the same inviter serialize and can never push paid past the gross
		// owed. This is the cross-DB-safe substitute for SELECT ... FOR UPDATE (which SQLite rejects);
		// new commissions only raise total, so a stale total can only under-credit, never over-pay.
		// The cap is expressed as `aff_cash_paid <= total - amount` (computed in Go, both >= 0 here so
		// no overflow) rather than `aff_cash_paid + amount <= total`, to avoid a signed-int64 overflow
		// in the DB-side addition on corrupted/extreme data; a negative cap (amount > total) simply
		// matches no row and is rejected below.
		payCap := total - amount
		res := tx.Model(&User{}).
			Where("id = ? AND aff_cash_paid <= ?", inviterId, payCap).
			UpdateColumn("aff_cash_paid", gorm.Expr("aff_cash_paid + ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			outstanding := total - inviter.AffCashPaid
			if outstanding < 0 {
				outstanding = 0
			}
			return fmt.Errorf("结算金额超过未结返佣，当前未结 %s", logger.LogQuota(int(outstanding)))
		}
		row := &AffiliateCashPayout{
			InviterId:  inviterId,
			Amount:     amount,
			Note:       note,
			OperatorId: operatorId,
			CreatedAt:  common.GetTimestamp(),
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		payout = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("推广返佣现金结算 %s（线下）", logger.LogQuota(int(amount))))
	return payout, nil
}

// GetAffiliateCashPayouts returns an inviter's recorded cash settlements, newest first.
func GetAffiliateCashPayouts(inviterId, limit int) ([]AffiliateCashPayout, error) {
	if inviterId <= 0 {
		return nil, errors.New("invalid inviter id")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := []AffiliateCashPayout{}
	if err := DB.Where("inviter_id = ?", inviterId).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
