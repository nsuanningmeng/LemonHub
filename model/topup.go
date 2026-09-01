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
	Id            int     `json:"id"`
	SiteId        int     `json:"site_id" gorm:"type:int;default:0;index"` // white-label sub-site (0 = main site)
	UserId        int     `json:"user_id" gorm:"index"`
	Amount        int64   `json:"amount"`
	Money         float64 `json:"money"`
	TradeNo       string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod string  `json:"payment_method" gorm:"type:varchar(50)"`
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
	ClawedBackQuota int64 `json:"clawed_back_quota" gorm:"default:0"`
	CreateTime      int64 `json:"create_time" gorm:"index:idx_topup_provider_status_time,priority:3"`
	CompleteTime    int64 `json:"complete_time"`
	// EpayQueryTime is the nullable last scheduled reconciliation attempt. Keeping the
	// column nullable avoids a default-value table rewrite when upgrading large legacy
	// databases. Ordering by the
	// oldest attempt prevents a fixed batch of abandoned orders from starving paid
	// orders deeper in the pending queue.
	EpayQueryTime *int64 `json:"-"`
	// EpayCallbackQueryTime is a separate cross-instance callback cooldown lease.
	// Public callback traffic must not influence the reconciliation worker's fair
	// ordering, otherwise a forged signed callback could keep a paid order out of
	// the automatic recovery batch.
	EpayCallbackQueryTime *int64 `json:"-"`
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
	ErrPaymentMethodMismatch   = errors.New("payment method mismatch")
	ErrTopUpNotFound           = errors.New("topup not found")
	ErrTopUpStatusInvalid      = errors.New("topup status invalid")
	ErrTopUpAmountInvalid      = errors.New("topup clawback amount invalid")
	ErrInvalidTopUpQuota       = errors.New("invalid top-up quota")
	ErrTopUpQuotaLimitExceeded = errors.New("top-up quota limit exceeded")
)

// preflightMySQLTopUpStatusNarrowing prevents MySQL's non-strict mode from
// silently truncating legacy LONGTEXT status values when AutoMigrate changes
// the column to VARCHAR(32) for the reconciliation index. The legacy column
// normally inherits the table character set; a non-standard explicit
// character set is rejected before DDL because a bare GORM MODIFY would reset
// it to the table default and could mutate unrepresentable text.
func preflightMySQLTopUpStatusNarrowing(db *gorm.DB) error {
	if db == nil || db.Dialector.Name() != "mysql" ||
		!db.Migrator().HasTable(&TopUp{}) || !db.Migrator().HasColumn(&TopUp{}, "status") {
		return nil
	}

	tableName := db.NamingStrategy.TableName("TopUp")
	type mysqlTopUpStatusColumn struct {
		DataType      string `gorm:"column:data_type"`
		MaximumLength int64  `gorm:"column:character_maximum_length"`
		CharacterSet  string `gorm:"column:character_set_name"`
		Collation     string `gorm:"column:collation_name"`
		Extra         string `gorm:"column:extra"`
	}
	var column mysqlTopUpStatusColumn
	result := db.Raw(
		`SELECT DATA_TYPE AS data_type,
       COALESCE(CHARACTER_MAXIMUM_LENGTH, 0) AS character_maximum_length,
       COALESCE(CHARACTER_SET_NAME, '') AS character_set_name,
       COALESCE(COLLATION_NAME, '') AS collation_name,
       EXTRA AS extra
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'status'`,
		tableName,
	).Scan(&column)
	if result.Error != nil {
		return fmt.Errorf("inspect MySQL top-up status column before narrowing: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("missing MySQL top-up status column before narrowing")
	}
	if column.Extra != "" {
		return fmt.Errorf("MySQL top-up status column has unsupported extra attributes %q", column.Extra)
	}
	switch strings.ToLower(column.DataType) {
	case "varchar", "tinytext", "text", "mediumtext", "longtext":
	default:
		return fmt.Errorf("MySQL top-up status column has unsupported type %q", column.DataType)
	}

	const targetLength = 32
	if !strings.EqualFold(column.DataType, "varchar") || column.MaximumLength != targetLength {
		var tableComparison struct {
			CharacterSet string `gorm:"column:character_set_name"`
			Collation    string `gorm:"column:collation_name"`
		}
		result = db.Raw(
			`SELECT c.CHARACTER_SET_NAME AS character_set_name,
       t.TABLE_COLLATION AS collation_name
FROM information_schema.TABLES t
JOIN information_schema.COLLATIONS c ON c.COLLATION_NAME = t.TABLE_COLLATION
WHERE t.TABLE_SCHEMA = DATABASE() AND t.TABLE_NAME = ?`,
			tableName,
		).Scan(&tableComparison)
		if result.Error != nil {
			return fmt.Errorf("inspect MySQL top-up table comparison before narrowing: %w", result.Error)
		}
		if result.RowsAffected != 1 || tableComparison.CharacterSet == "" || tableComparison.Collation == "" {
			return errors.New("missing MySQL top-up table comparison before narrowing")
		}
		if !strings.EqualFold(column.CharacterSet, tableComparison.CharacterSet) ||
			!strings.EqualFold(column.Collation, tableComparison.Collation) {
			return fmt.Errorf(
				"MySQL top-up status uses %s/%s but table default is %s/%s; migration stopped before status DDL",
				column.CharacterSet,
				column.Collation,
				tableComparison.CharacterSet,
				tableComparison.Collation,
			)
		}
	}

	var unsafeRow struct {
		Id           int   `gorm:"column:id"`
		StatusLength int64 `gorm:"column:status_length"`
	}
	result = db.Table(tableName).
		Select("id, CHAR_LENGTH(status) AS status_length").
		Where("status IS NOT NULL AND CHAR_LENGTH(status) > ?", targetLength).
		Order("id").
		Limit(1).
		Scan(&unsafeRow)
	if result.Error != nil {
		return fmt.Errorf("preflight MySQL top-up status values before narrowing: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return fmt.Errorf(
			"MySQL top-up row %d has a %d-character status; migration stopped before narrowing status to VARCHAR(%d)",
			unsafeRow.Id,
			unsafeRow.StatusLength,
			targetLength,
		)
	}
	return nil
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func topUpQuotaMaxCurrent(creditedQuota int) (int, error) {
	if creditedQuota <= 0 || creditedQuota >= common.MaxQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxQuota - 1 - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Quota > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota atomically enforces the int32 wallet ceiling while adding
// quota. Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int, updates map[string]interface{}) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	updateFields := make(map[string]interface{}, len(updates)+1)
	for key, value := range updates {
		updateFields[key] = value
	}
	updateFields["quota"] = gorm.Expr("quota + ?", creditedQuota)

	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", userId, maxCurrentQuota).
		Updates(updateFields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var count int64
	if err := tx.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrTopUpQuotaLimitExceeded
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
// [createdAfter, createdBefore]. Least-recently queried orders come first, with newest
// IDs breaking ties for never-queried rows. The worker stamps each selected batch
// before network I/O, so a finite backlog is drained fairly instead of querying the
// same abandoned first page forever.
func GetPendingEpayTopUps(createdAfter, createdBefore int64, limit int) ([]*TopUp, error) {
	var topups []*TopUp
	err := DB.Where("payment_provider = ? AND status = ? AND create_time >= ? AND create_time <= ?",
		PaymentProviderEpay, common.TopUpStatusPending, createdAfter, createdBefore).
		Order("COALESCE(epay_query_time, 0) asc").Order("id desc").Limit(limit).Find(&topups).Error
	if err != nil {
		return nil, err
	}
	return topups, nil
}

func MarkEpayTopUpQueryAttempts(ids []int, queryTime int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&TopUp{}).
		Where("id IN ? AND payment_provider = ? AND status = ?", ids, PaymentProviderEpay, common.TopUpStatusPending).
		Update("epay_query_time", queryTime).Error
}

// ClaimEpayTopUpQueryAttempt atomically rate-limits authoritative gateway queries
// for one pending order across application instances. A signed callback is public
// input and must not be able to fan out duplicate outbound requests.
func ClaimEpayTopUpQueryAttempt(id int, queryTime, previousAllowedAt int64) (bool, error) {
	if id <= 0 {
		return false, errors.New("invalid top-up order id")
	}
	result := DB.Model(&TopUp{}).
		Where(
			"id = ? AND payment_provider = ? AND status = ? AND (epay_callback_query_time IS NULL OR epay_callback_query_time <= ?)",
			id,
			PaymentProviderEpay,
			common.TopUpStatusPending,
			previousAllowedAt,
		).
		Update("epay_callback_query_time", queryTime)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
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

// RechargeEpay 原子完成易支付订单：订单行锁、状态校验、成功更新与用户额度增加
// 在同一个事务内完成，因此同一订单的并发/重复回调（包括多实例部署下）最多充值一次。
// alreadyDone=true 表示订单此前已完成，本次为幂等重复回调。
// 进程内的 LockOrder 只是优化，正确性由本函数的数据库行锁保证。
func RechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string) (alreadyDone bool, err error) {
	if tradeNo == "" {
		return false, errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if actualPaymentMethod != "" && topUp.PaymentMethod != actualPaymentMethod {
			topUp.PaymentMethod = actualPaymentMethod
		}
		var quotaErr error
		quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})
	if err != nil {
		if !errors.Is(err, ErrTopUpNotFound) && !errors.Is(err, ErrPaymentMethodMismatch) && !errors.Is(err, ErrTopUpStatusInvalid) {
			common.SysError("epay topup failed: " + err.Error())
		}
		return false, err
	}
	if alreadyDone {
		return true, nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "epay topup")

	common.SysLog(fmt.Sprintf("易支付充值成功 trade_no=%s user_id=%d quota_to_add=%d money=%.2f", topUp.TradeNo, topUp.UserId, quotaToAdd, topUp.Money))
	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quotaToAdd), PaymentProviderEpay); serr != nil {
		common.SysError("referral settlement failed (epay): " + serr.Error())
	}
	return false, nil
}

func Recharge(referenceId string, customerId string, paymentIntent string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
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

		quota, err = common.QuotaFromDecimalStrict(
			decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}
		return creditTopUpQuota(tx, topUp.UserId, quota, map[string]interface{}{
			"stripe_customer": customerId,
		})
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, "stripe topup")

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quota), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

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
		// Hydrate before opening the mutation transaction. Cache operations inside
		// the transaction must never open a second DB read (which can deadlock on
		// SQLite's single connection) and must fail closed on eviction/outage.
		var observed TopUp
		if err := DB.Where("payment_intent = ?", paymentIntent).First(&observed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if err := resolveUserQuotaCacheUncertainty(observed.UserId); err != nil {
			return err
		}
		observedDesired, _, observedErr := stripeDesiredClawback(&observed, refundedMinor, chargeMinor, isDispute)
		if observedErr != nil {
			return observedErr
		}
		if observedDesired > observed.ClawedBackQuota {
			if err := ensureUserQuotaCacheAvailable(observed.UserId); err != nil {
				return err
			}
		}

		clawedDelta = 0
		raceLost := false
		cacheApplied := false
		var cacheDelta int64
		var cacheUserId int
		var expectedClawed int64
		callbackCompleted := false
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

			desired, creditedQuota, err := stripeDesiredClawback(&topUp, refundedMinor, chargeMinor, isDispute)
			if err != nil {
				return err
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
				userId, paymentMethod, tradeNo, newStatus = topUp.UserId, topUp.PaymentMethod, topUp.TradeNo, target
				clawedBackTotal, creditedTotal = desired, creditedQuota
				callbackCompleted = true
				return nil // duplicate/idempotent: nothing to change
			}
			if delta > 0 {
				applied, err := applyPreparedUserQuotaCacheDelta(topUp.UserId, -delta)
				if err != nil {
					return err
				}
				cacheApplied = applied
				cacheDelta = -delta
				cacheUserId = topUp.UserId
				expectedClawed = desired
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
				userUpdate := tx.Model(&User{}).Where("id = ?", topUp.UserId).
					Update("quota", gorm.Expr("quota - ?", delta))
				if userUpdate.Error != nil {
					return userUpdate.Error
				}
				if userUpdate.RowsAffected != 1 {
					return gorm.ErrRecordNotFound
				}
			}
			clawedDelta = delta
			userId, paymentMethod, tradeNo, newStatus = topUp.UserId, topUp.PaymentMethod, topUp.TradeNo, target
			clawedBackTotal, creditedTotal = desired, creditedQuota
			callbackCompleted = true
			return nil
		})
		if err != nil && cacheApplied {
			if callbackCompleted {
				// The callback completed and only COMMIT reported an error. Re-read
				// the CAS marker: if it advanced, keep the conservative cache debit;
				// otherwise compensate the rolled-back transaction.
				var persisted TopUp
				checkErr := DB.Select("clawed_back_quota").Where("payment_intent = ?", paymentIntent).First(&persisted).Error
				switch {
				case checkErr == nil && persisted.ClawedBackQuota >= expectedClawed:
					common.SysError(fmt.Sprintf("Stripe clawback commit returned an error but durable state advanced: payment_intent=%s desired=%d error=%v", paymentIntent, expectedClawed, err))
					fenceErr := fenceUserQuotaCacheUncertainty(cacheUserId, "stripe_clawback_commit_recheck")
					if fenceErr != nil {
						common.SysError(fmt.Sprintf("failed to fence Stripe clawback cache after uncertain commit: payment_intent=%s error=%v", paymentIntent, fenceErr))
					} else if reconcileErr := resolveUserQuotaCacheUncertainty(cacheUserId); reconcileErr != nil {
						common.SysError(fmt.Sprintf("failed to reconcile Stripe clawback cache after uncertain commit: payment_intent=%s error=%v", paymentIntent, reconcileErr))
					} else {
						err = nil
					}
				case checkErr == nil:
					compensatePreparedUserQuotaCacheDelta(cacheUserId, cacheDelta, "Stripe clawback rollback")
				default:
					common.SysError(fmt.Sprintf("Stripe clawback commit outcome is ambiguous; retaining fail-closed cache debit: payment_intent=%s desired=%d tx_error=%v check_error=%v", paymentIntent, expectedClawed, err, checkErr))
					_ = fenceUserQuotaCacheUncertainty(cacheUserId, "stripe_clawback")
				}
			} else {
				compensatePreparedUserQuotaCacheDelta(cacheUserId, cacheDelta, "Stripe clawback rollback")
			}
		}
		if raceLost {
			continue // another delivery advanced the row; re-read and recompute
		}
		if err != nil {
			return err
		}

		if clawedDelta > 0 {
			reason := "退款"
			if isDispute {
				reason = "拒付(chargeback)"
			}
			RecordTopupLog(userId, fmt.Sprintf("Stripe %s 回扣额度 -%s（订单 %s，状态 %s）",
				reason, logger.FormatQuota(int(clawedDelta)), tradeNo, newStatus),
				callerIp, paymentMethod, PaymentMethodStripe)

		}
		// Retry referral reversal even for an idempotent duplicate delivery. The
		// wallet clawback must remain committed when referral Redis was temporarily
		// unavailable, but a later webhook must still converge the referral ledger.
		if clawedBackTotal > 0 {
			if rerr := ReverseReferralOnTopUpClawback(userId, tradeNo, clawedBackTotal, creditedTotal, callerIp); rerr != nil {
				common.SysError("referral reversal failed (stripe clawback): " + rerr.Error())
			}
		}
		return nil
	}
	return errTopUpRaceLost // exhausted retries under sustained contention (extremely unlikely)
}

func stripeDesiredClawback(topUp *TopUp, refundedMinor, chargeMinor int64, isDispute bool) (int64, int64, error) {
	if topUp == nil || topUp.PaymentProvider != PaymentProviderStripe {
		return 0, 0, ErrPaymentMethodMismatch
	}
	switch topUp.Status {
	case common.TopUpStatusSuccess, common.TopUpStatusRefunded, common.TopUpStatusDisputed:
	default:
		return 0, 0, ErrTopUpStatusInvalid
	}
	credited, err := common.QuotaFromDecimalStrict(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil {
		return 0, 0, err
	}
	creditedQuota := int64(credited)
	if creditedQuota <= 0 {
		return 0, 0, ErrTopUpStatusInvalid
	}

	var desired int64
	switch {
	case isDispute:
		desired = creditedQuota
	case chargeMinor <= 0 || refundedMinor < 0:
		return 0, 0, ErrTopUpAmountInvalid
	case refundedMinor >= chargeMinor:
		desired = creditedQuota
	default:
		desired = decimal.NewFromInt(creditedQuota).
			Mul(decimal.NewFromInt(refundedMinor)).
			Div(decimal.NewFromInt(chargeMinor)).
			Round(0).IntPart()
	}
	if desired > creditedQuota {
		desired = creditedQuota
	}
	return desired, creditedQuota, nil
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
		var quotaErr error
		if topUp.PaymentProvider == PaymentProviderStripe {
			quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(
				decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		} else {
			quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		}
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
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
	syncCreditUserQuotaCache(userId, quotaToAdd, "manual topup")
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
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

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota, err = common.QuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}

		return creditTopUpQuota(tx, topUp.UserId, quota, nil)
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, "creem topup")
	// Customer profile data is optional and must never roll back an already
	// verified payment. Bind only a normalized, same-site-unique email and only
	// while the account still has no email; otherwise leave the profile intact.
	if _, emailErr := BindUserEmailIfEmpty(topUp.UserId, customerEmail); emailErr != nil {
		common.SysLog("failed to attach Creem customer email after topup: " + emailErr.Error())
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quota), PaymentProviderCreem); serr != nil {
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

		quotaToAdd, err = common.QuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "waffo topup")

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

		quotaToAdd, err = common.QuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "waffo pancake topup")

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
		if serr := SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quotaToAdd), PaymentProviderWaffoPancake); serr != nil {
			common.SysError("referral settlement failed (waffo pancake): " + serr.Error())
		}
	}

	return nil
}
