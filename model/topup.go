package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	SiteId          int     `json:"site_id" gorm:"type:int;default:0;index"` // white-label sub-site (0 = main site)
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	// Composite index idx_topup_provider_status_time backs the epay reconciliation
	// sweep's existence/list queries (payment_provider = ? AND status = ? AND
	// create_time BETWEEN ? AND ?): equality columns first, the create_time range last.
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);default:'';index:idx_topup_provider_status_time,priority:1"`
	// PaymentIntent is the Stripe payment_intent captured at fulfillment. Refund
	// and dispute webhook events are charge-level and carry payment_intent (not
	// the checkout client_reference_id / trade_no), so this is the join key used
	// to claw back quota. Empty for non-Stripe providers and pre-feature orders.
	PaymentIntent string `json:"payment_intent" gorm:"type:varchar(255);index;default:''"`
	// ClawedBackQuota is the cumulative quota already reversed by refunds/disputes.
	// It makes clawback idempotent across partial and duplicate webhook deliveries.
	ClawedBackQuota int64  `json:"clawed_back_quota" gorm:"default:0"`
	CreateTime      int64  `json:"create_time" gorm:"index:idx_topup_provider_status_time,priority:3"`
	CompleteTime    int64  `json:"complete_time"`
	// Explicit varchar type (not GORM's default longtext for an untyped string): the
	// composite index below indexes this column, and MySQL cannot index a TEXT/LONGTEXT
	// column without a prefix length. varchar(32) fits every status value.
	Status string `json:"status" gorm:"type:varchar(32);index:idx_topup_provider_status_time,priority:2"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
	ErrTopUpAmountInvalid    = errors.New("topup clawback amount invalid")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

// FindTopUpByTradeNo distinguishes a missing order from a transient DB failure, unlike
// GetTopUpByTradeNo which folds both into nil. Payment callbacks need the distinction:
// a DB hiccup must read as "retry later" (the gateway re-delivers), not "订单不存在".
// Returns (nil, nil) when no such order exists.
func FindTopUpByTradeNo(tradeNo string) (*TopUp, error) {
	var topUp TopUp
	err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &topUp, nil
}

// GetPendingEpayTopUps lists pending epay top-up orders created inside
// [createdAfter, createdBefore], NEWEST first — the reconciliation sweep's work list.
// Newest-first matters because epay orders never expire on their own: abandoned
// (never-paid) orders accumulate indefinitely inside the window, and an oldest-first
// batch would re-query that dead head every sweep and starve a genuinely-paid order
// whose notify+return were both lost. A lost-callback order is settled seconds after
// payment, i.e. close to its creation time, so newest-first reaches it on the next
// sweep regardless of how many stale orders pile up behind it.
func GetPendingEpayTopUps(createdAfter, createdBefore int64, limit int) ([]*TopUp, error) {
	var topups []*TopUp
	err := DB.Where("payment_provider = ? AND status = ? AND create_time >= ? AND create_time <= ?",
		PaymentProviderEpay, common.TopUpStatusPending, createdAfter, createdBefore).
		Order("id desc").Limit(limit).Find(&topups).Error
	if err != nil {
		return nil, err
	}
	return topups, nil
}

// HasPendingEpayTopUps reports whether at least one pending epay top-up exists in the
// window, so the reconcile task schedules no runs on an idle system.
func HasPendingEpayTopUps(createdAfter, createdBefore int64) bool {
	var ids []int
	err := DB.Model(&TopUp{}).Where("payment_provider = ? AND status = ? AND create_time >= ? AND create_time <= ?",
		PaymentProviderEpay, common.TopUpStatusPending, createdAfter, createdBefore).
		Limit(1).Pluck("id", &ids).Error
	return err == nil && len(ids) > 0
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, paymentIntent string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		// Record the Stripe payment_intent so a later refund/dispute (which is
		// keyed by payment_intent, not trade_no) can be linked back to this order.
		if paymentIntent != "" {
			topUp.PaymentIntent = paymentIntent
		}
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = int64(common.QuotaFromDecimal(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit))))
		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)}).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(int(quota)), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quota), PaymentProviderStripe); serr != nil {
		common.SysError("referral settlement failed (stripe): " + serr.Error())
	}

	return nil
}

// ReverseStripeTopUp claws back quota for a Stripe top-up that was refunded or
// disputed, keyed by the Stripe payment_intent captured at fulfillment.
//
// refundedMinor/chargeMinor are Stripe minor units (e.g. cents). For a refund the
// clawback is proportional to the cumulative refunded fraction; for a dispute the
// full remaining credited quota is reversed and the order is flagged
// TopUpStatusDisputed for admin review. Reversal is idempotent across partial and
// duplicate webhook deliveries because it tracks cumulative ClawedBackQuota and
// only ever debits the delta. The user balance is allowed to go negative, matching
// the project's soft-quota model. The referral bonus paid to an inviter is NOT
// reversed here (out of scope).
//
// Returns ErrTopUpNotFound when no credited Stripe order matches the payment_intent
// (an unrelated charge, or a pre-feature order without a stored payment_intent), so
// the caller can safely ignore the event.
func ReverseStripeTopUp(paymentIntent string, refundedMinor int64, chargeMinor int64, isDispute bool, callerIp string) error {
	paymentIntent = strings.TrimSpace(paymentIntent)
	if paymentIntent == "" {
		return ErrTopUpNotFound
	}

	var clawedDelta int64
	var newStatus string
	var userId int
	var paymentMethod, tradeNo string
	var clawedBackTotal, creditedTotal int64

	// Idempotency / cross-node safety is enforced by an atomic compare-and-set on
	// clawed_back_quota (UPDATE ... WHERE clawed_back_quota = <observed> + RowsAffected),
	// NOT by SELECT ... FOR UPDATE — gorm:query_option "FOR UPDATE" is a no-op in
	// GORM v2 and SQLite rejects it (see model/site_topup.go). A concurrent or
	// duplicate webhook that reads the same base loses the CAS and is retried, so no
	// refund/dispute is ever applied twice. Retries converge because every winner
	// advances clawed_back_quota monotonically toward creditedQuota.
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		clawedDelta = 0
		raceLost := false
		err := DB.Transaction(func(tx *gorm.DB) error {
			var topUp TopUp
			if err := tx.Where("payment_intent = ?", paymentIntent).First(&topUp).Error; err != nil {
				return ErrTopUpNotFound
			}
			if topUp.PaymentProvider != PaymentProviderStripe {
				return ErrPaymentMethodMismatch
			}
			// Only orders that were actually credited can be clawed back.
			switch topUp.Status {
			case common.TopUpStatusSuccess, common.TopUpStatusRefunded, common.TopUpStatusDisputed:
			default:
				return ErrTopUpStatusInvalid
			}

			creditedQuota := int64(common.QuotaFromDecimal(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit))))
			if creditedQuota <= 0 {
				// Bad/legacy row (Money <= 0): nothing was validly credited, so refuse
				// to mutate quota or status rather than produce nonsensical state.
				return ErrTopUpStatusInvalid
			}

			// Desired cumulative clawback given everything reversed so far.
			var desired int64
			switch {
			case isDispute:
				desired = creditedQuota // a chargeback reverses the whole charge
			case chargeMinor <= 0 || refundedMinor < 0:
				// Without a positive charge total we cannot compute a refund fraction.
				// Do NOT escalate to a full clawback (that would over-debit a paying
				// user on a partial refund); treat the event as non-actionable.
				return ErrTopUpAmountInvalid
			case refundedMinor >= chargeMinor:
				desired = creditedQuota // fully refunded
			default:
				desired = decimal.NewFromInt(creditedQuota).
					Mul(decimal.NewFromInt(refundedMinor)).
					Div(decimal.NewFromInt(chargeMinor)).
					Round(0).IntPart()
			}
			if desired > creditedQuota {
				desired = creditedQuota
			}

			prevClawed := topUp.ClawedBackQuota
			delta := desired - prevClawed
			if delta < 0 {
				delta = 0 // clawback is monotonic; never restore quota
			}

			target := topUp.Status
			if isDispute {
				target = common.TopUpStatusDisputed
			} else if topUp.Status != common.TopUpStatusDisputed && desired >= creditedQuota {
				// A refund marks the order refunded once fully clawed, but must never
				// downgrade an order already flagged disputed (losing its review flag).
				target = common.TopUpStatusRefunded
			}

			if delta == 0 && target == topUp.Status {
				return nil // duplicate/idempotent: nothing to change
			}

			// Atomic claim: only the transaction that still observes the same
			// clawed_back_quota wins; a concurrent winner makes RowsAffected == 0.
			claim := tx.Model(&TopUp{}).
				Where("payment_intent = ? AND clawed_back_quota = ?", paymentIntent, prevClawed).
				Updates(map[string]interface{}{"clawed_back_quota": desired, "status": target})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected == 0 {
				raceLost = true
				return errTopUpRaceLost
			}
			if delta > 0 {
				if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).
					Update("quota", gorm.Expr("quota - ?", delta)).Error; err != nil {
					return err
				}
			}
			clawedDelta = delta
			userId, paymentMethod, tradeNo, newStatus = topUp.UserId, topUp.PaymentMethod, topUp.TradeNo, target
			clawedBackTotal, creditedTotal = desired, creditedQuota
			return nil
		})
		if raceLost {
			continue // another delivery advanced the row; re-read and recompute
		}
		if err != nil {
			return err
		}

		if clawedDelta > 0 {
			// Force the spendable quota to reflect the debit immediately (the credit
			// path and this debit both write the DB directly; drop any cached value).
			_ = invalidateUserCache(userId)
			reason := "退款"
			if isDispute {
				reason = "拒付(chargeback)"
			}
			RecordTopupLog(userId, fmt.Sprintf("Stripe %s 回扣额度 -%s（订单 %s，状态 %s）",
				reason, logger.FormatQuota(int(clawedDelta)), tradeNo, newStatus),
				callerIp, paymentMethod, PaymentMethodStripe)

			// Reverse the referral rewards this top-up generated (commission proportionally,
			// first bonus when the invitee is fully deactivated). Best-effort: a failure must
			// not roll back or block the clawback itself — mirrors SettleReferralOnTopUp.
			if rerr := ReverseReferralOnTopUpClawback(userId, tradeNo, clawedBackTotal, creditedTotal, callerIp); rerr != nil {
				common.SysError("referral reversal failed (stripe clawback): " + rerr.Error())
			}
		}
		return nil
	}
	return errTopUpRaceLost // exhausted retries under sustained contention (extremely unlikely)
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo, siteScope int) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if siteScope != SiteScopeAll {
		query = query.Where("site_id = ?", siteScope)
	}

	if err = query.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo, siteScope int) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}
	if siteScope != SiteScopeAll {
		query = query.Where("site_id = ?", siteScope)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = common.QuotaFromDecimal(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit))
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = common.QuotaFromDecimal(dAmount.Mul(dQuotaPerUnit))
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数），饱和到 int32 quota 列的上限
		quota = int64(common.QuotaFromFloat(float64(topUp.Amount)))

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, quota, PaymentProviderCreem); serr != nil {
		common.SysError("referral settlement failed (creem): " + serr.Error())
	}

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = common.QuotaFromDecimal(dAmount.Mul(dQuotaPerUnit))
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
		if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quotaToAdd), PaymentProviderWaffo); serr != nil {
			common.SysError("referral settlement failed (waffo): " + serr.Error())
		}
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		quotaToAdd = common.QuotaFromDecimal(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
		if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quotaToAdd), PaymentProviderWaffoPancake); serr != nil {
			common.SysError("referral settlement failed (waffo pancake): " + serr.Error())
		}
	}

	return nil
}
