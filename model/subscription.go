package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Subscription duration units
const (
	SubscriptionDurationYear   = "year"
	SubscriptionDurationMonth  = "month"
	SubscriptionDurationDay    = "day"
	SubscriptionDurationHour   = "hour"
	SubscriptionDurationCustom = "custom"
)

// Subscription quota reset period
const (
	SubscriptionResetNever   = "never"
	SubscriptionResetDaily   = "daily"
	SubscriptionResetWeekly  = "weekly"
	SubscriptionResetMonthly = "monthly"
	SubscriptionResetCustom  = "custom"
)

var (
	ErrSubscriptionOrderNotFound          = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid     = errors.New("subscription order status invalid")
	ErrSubscriptionOrderPlanChanged       = errors.New("subscription order plan changed after checkout")
	ErrSubscriptionOrderPlanSnapshot      = errors.New("subscription order plan snapshot is invalid")
	ErrSubscriptionPurchaseLimitReached   = errors.New("已达到该套餐购买上限或存在待支付订单")
	ErrSubscriptionBalanceCommitUncertain = errors.New("subscription balance purchase commit outcome is uncertain")
)

const (
	subscriptionPlanCacheNamespace     = "new-api:subscription_plan:v1"
	subscriptionPlanInfoCacheNamespace = "new-api:subscription_plan_info:v1"
)

var (
	subscriptionPlanCacheOnce     sync.Once
	subscriptionPlanInfoCacheOnce sync.Once

	subscriptionPlanCache     *cachex.HybridCache[SubscriptionPlan]
	subscriptionPlanInfoCache *cachex.HybridCache[SubscriptionPlanInfo]
)

func subscriptionPlanCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_TTL", 300)
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanInfoCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_TTL", 120)
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_CAP", 5000)
	if capacity <= 0 {
		capacity = 5000
	}
	return capacity
}

func subscriptionPlanInfoCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getSubscriptionPlanCache() *cachex.HybridCache[SubscriptionPlan] {
	subscriptionPlanCacheOnce.Do(func() {
		ttl := subscriptionPlanCacheTTL()
		subscriptionPlanCache = cachex.NewHybridCache[SubscriptionPlan](cachex.HybridCacheConfig[SubscriptionPlan]{
			Namespace: cachex.Namespace(subscriptionPlanCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlan]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlan] {
				return hot.NewHotCache[string, SubscriptionPlan](hot.LRU, subscriptionPlanCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanCache
}

func getSubscriptionPlanInfoCache() *cachex.HybridCache[SubscriptionPlanInfo] {
	subscriptionPlanInfoCacheOnce.Do(func() {
		ttl := subscriptionPlanInfoCacheTTL()
		subscriptionPlanInfoCache = cachex.NewHybridCache[SubscriptionPlanInfo](cachex.HybridCacheConfig[SubscriptionPlanInfo]{
			Namespace: cachex.Namespace(subscriptionPlanInfoCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlanInfo]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlanInfo] {
				return hot.NewHotCache[string, SubscriptionPlanInfo](hot.LRU, subscriptionPlanInfoCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanInfoCache
}

func subscriptionPlanCacheKey(id int) string {
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(id)
}

func InvalidateSubscriptionPlanCache(planId int) {
	if planId <= 0 {
		return
	}
	cache := getSubscriptionPlanCache()
	_, _ = cache.DeleteMany([]string{subscriptionPlanCacheKey(planId)})
	infoCache := getSubscriptionPlanInfoCache()
	_ = infoCache.Purge()
}

// Subscription plan
type SubscriptionPlan struct {
	Id int `json:"id"`

	Title    string `json:"title" gorm:"type:varchar(128);not null"`
	Subtitle string `json:"subtitle" gorm:"type:varchar(255);default:''"`

	// Display money amount (follow existing code style: float64 for money)
	PriceAmount float64 `json:"price_amount" gorm:"type:decimal(10,6);not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`

	DurationUnit  string `json:"duration_unit" gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue int    `json:"duration_value" gorm:"type:int;not null;default:1"`
	CustomSeconds int64  `json:"custom_seconds" gorm:"type:bigint;not null;default:0"`

	Enabled   bool `json:"enabled" gorm:"default:true"`
	SortOrder int  `json:"sort_order" gorm:"type:int;default:0"`

	AllowBalancePay *bool `json:"allow_balance_pay"`

	// Allow falling back to wallet balance after subscription quota is exhausted (empty = true)
	AllowWalletOverflow *bool `json:"allow_wallet_overflow"`

	StripePriceId         string `json:"stripe_price_id" gorm:"type:varchar(128);default:''"`
	CreemProductId        string `json:"creem_product_id" gorm:"type:varchar(128);default:''"`
	WaffoPancakeProductId string `json:"waffo_pancake_product_id" gorm:"type:varchar(128);default:''"`

	// Max purchases per user (0 = unlimited)
	MaxPurchasePerUser int `json:"max_purchase_per_user" gorm:"type:int;default:0"`

	// Upgrade user group after purchase (empty = no change)
	UpgradeGroup string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`

	// Downgrade user group on expiry (empty = revert to the group held before purchase)
	DowngradeGroup string `json:"downgrade_group" gorm:"type:varchar(64);default:''"`

	// Total quota (amount in quota units, 0 = unlimited)
	TotalAmount int64 `json:"total_amount" gorm:"type:bigint;not null;default:0"`

	// Quota reset period for plan
	QuotaResetPeriod        string `json:"quota_reset_period" gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds" gorm:"type:bigint;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (p *SubscriptionPlan) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (p *SubscriptionPlan) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = common.GetTimestamp()
	return nil
}

func (p *SubscriptionPlan) NormalizeDefaults() {
	if p.AllowBalancePay == nil {
		p.AllowBalancePay = common.GetPointer(true)
	}
	if p.AllowWalletOverflow == nil {
		p.AllowWalletOverflow = common.GetPointer(true)
	}
}

// Subscription order (payment -> webhook -> create UserSubscription)
type SubscriptionOrder struct {
	Id     int     `json:"id"`
	UserId int     `json:"user_id" gorm:"index"`
	PlanId int     `json:"plan_id" gorm:"index"`
	Money  float64 `json:"money"`

	TradeNo       string `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod string `json:"payment_method" gorm:"type:varchar(50)"`
	// The provider/time index bounds reconciliation scans without indexing Status,
	// whose legacy database type may be TEXT on existing MySQL installations.
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);default:'';index:idx_subscription_order_provider_time,priority:1"`
	Status          string `json:"status"`
	CreateTime      int64  `json:"create_time" gorm:"index:idx_subscription_order_provider_time,priority:2"`
	CompleteTime    int64  `json:"complete_time"`
	// EpayQueryTime provides fair, persistent rotation through pending reconciliation
	// batches, including across process restarts. It stays nullable so adding the column
	// does not require backfilling every legacy order during migration.
	EpayQueryTime *int64 `json:"-"`
	// EpayCallbackQueryTime is deliberately independent from reconciliation
	// fairness. An untrusted callback may consume its own cooldown lease, but it
	// cannot push a genuinely paid order out of the scheduled recovery batch.
	EpayCallbackQueryTime *int64 `json:"-"`

	// PlanSnapshot is the immutable fulfillment contract captured before an
	// external checkout can accept payment. It intentionally stays private in
	// API responses and has no TEXT default for MySQL 5.7 compatibility.
	PlanSnapshot string `json:"-" gorm:"type:text"`
	// PurchaseLimitReserved records that this exact order already consumed a
	// MaxPurchasePerUser slot before payment. It is durable settlement evidence,
	// not inferred from provider or status, and remains private in API responses.
	PurchaseLimitReserved bool `json:"-"`

	ProviderPayload string `json:"provider_payload" gorm:"type:text"`
}

const subscriptionOrderPlanSnapshotVersion = 1

type subscriptionOrderPlanSnapshot struct {
	Version                     int    `json:"version"`
	PlanId                      int    `json:"plan_id"`
	Title                       string `json:"title"`
	PriceAmount                 string `json:"price_amount"`
	Currency                    string `json:"currency"`
	DurationUnit                string `json:"duration_unit"`
	DurationValue               int    `json:"duration_value"`
	CustomSeconds               int64  `json:"custom_seconds"`
	TotalAmount                 int64  `json:"total_amount"`
	QuotaResetPeriod            string `json:"quota_reset_period"`
	QuotaResetCustomSeconds     int64  `json:"quota_reset_custom_seconds"`
	UpgradeGroup                string `json:"upgrade_group"`
	DowngradeGroup              string `json:"downgrade_group"`
	AllowWalletOverflow         bool   `json:"allow_wallet_overflow"`
	MaxPurchasePerUser          int    `json:"max_purchase_per_user"`
	SourceSubscriptionUpdatedAt int64  `json:"source_subscription_updated_at"`
}

func (o *SubscriptionOrder) SetPlanSnapshot(plan *SubscriptionPlan) error {
	if o == nil || plan == nil || plan.Id <= 0 || o.PlanId != plan.Id {
		return fmt.Errorf("%w: plan id mismatch", ErrSubscriptionOrderPlanSnapshot)
	}
	if math.IsNaN(o.Money) || math.IsInf(o.Money, 0) || math.IsNaN(plan.PriceAmount) || math.IsInf(plan.PriceAmount, 0) {
		return fmt.Errorf("%w: money must be finite", ErrSubscriptionOrderPlanSnapshot)
	}
	orderMoney := decimal.NewFromFloat(o.Money)
	planMoney := decimal.NewFromFloat(plan.PriceAmount)
	if orderMoney.IsNegative() || !orderMoney.Equal(planMoney) {
		return fmt.Errorf("%w: order money does not match plan", ErrSubscriptionOrderPlanSnapshot)
	}
	if plan.TotalAmount < 0 || plan.MaxPurchasePerUser < 0 {
		return fmt.Errorf("%w: negative plan limit", ErrSubscriptionOrderPlanSnapshot)
	}
	if _, err := calcPlanEndTime(time.Unix(0, 0), plan); err != nil {
		return fmt.Errorf("%w: %v", ErrSubscriptionOrderPlanSnapshot, err)
	}
	resetPeriod := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if resetPeriod == SubscriptionResetCustom && plan.QuotaResetCustomSeconds <= 0 {
		return fmt.Errorf("%w: invalid custom reset period", ErrSubscriptionOrderPlanSnapshot)
	}
	allowWalletOverflow := true
	if plan.AllowWalletOverflow != nil {
		allowWalletOverflow = *plan.AllowWalletOverflow
	}
	snapshot := subscriptionOrderPlanSnapshot{
		Version:                     subscriptionOrderPlanSnapshotVersion,
		PlanId:                      plan.Id,
		Title:                       plan.Title,
		PriceAmount:                 planMoney.StringFixed(6),
		Currency:                    plan.Currency,
		DurationUnit:                plan.DurationUnit,
		DurationValue:               plan.DurationValue,
		CustomSeconds:               plan.CustomSeconds,
		TotalAmount:                 plan.TotalAmount,
		QuotaResetPeriod:            resetPeriod,
		QuotaResetCustomSeconds:     plan.QuotaResetCustomSeconds,
		UpgradeGroup:                strings.TrimSpace(plan.UpgradeGroup),
		DowngradeGroup:              strings.TrimSpace(plan.DowngradeGroup),
		AllowWalletOverflow:         allowWalletOverflow,
		MaxPurchasePerUser:          plan.MaxPurchasePerUser,
		SourceSubscriptionUpdatedAt: plan.UpdatedAt,
	}
	encoded, err := common.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSubscriptionOrderPlanSnapshot, err)
	}
	o.PlanSnapshot = string(encoded)
	return nil
}

func subscriptionPlanFromOrderSnapshot(order *SubscriptionOrder) (*SubscriptionPlan, error) {
	if order == nil || strings.TrimSpace(order.PlanSnapshot) == "" {
		return nil, fmt.Errorf("%w: snapshot is missing", ErrSubscriptionOrderPlanSnapshot)
	}
	if math.IsNaN(order.Money) || math.IsInf(order.Money, 0) {
		return nil, fmt.Errorf("%w: money must be finite", ErrSubscriptionOrderPlanSnapshot)
	}
	var snapshot subscriptionOrderPlanSnapshot
	if err := common.UnmarshalJsonStr(order.PlanSnapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("%w: malformed snapshot", ErrSubscriptionOrderPlanSnapshot)
	}
	if snapshot.Version != subscriptionOrderPlanSnapshotVersion || snapshot.PlanId <= 0 || snapshot.PlanId != order.PlanId {
		return nil, fmt.Errorf("%w: unsupported version or plan id", ErrSubscriptionOrderPlanSnapshot)
	}
	price, err := decimal.NewFromString(snapshot.PriceAmount)
	if err != nil || price.IsNegative() || !price.Equal(decimal.NewFromFloat(order.Money)) {
		return nil, fmt.Errorf("%w: price mismatch", ErrSubscriptionOrderPlanSnapshot)
	}
	if snapshot.TotalAmount < 0 || snapshot.MaxPurchasePerUser < 0 {
		return nil, fmt.Errorf("%w: negative plan limit", ErrSubscriptionOrderPlanSnapshot)
	}
	allowWalletOverflow := snapshot.AllowWalletOverflow
	plan := &SubscriptionPlan{
		Id:                      snapshot.PlanId,
		Title:                   snapshot.Title,
		PriceAmount:             price.InexactFloat64(),
		Currency:                snapshot.Currency,
		DurationUnit:            snapshot.DurationUnit,
		DurationValue:           snapshot.DurationValue,
		CustomSeconds:           snapshot.CustomSeconds,
		MaxPurchasePerUser:      snapshot.MaxPurchasePerUser,
		UpgradeGroup:            snapshot.UpgradeGroup,
		DowngradeGroup:          snapshot.DowngradeGroup,
		TotalAmount:             snapshot.TotalAmount,
		QuotaResetPeriod:        snapshot.QuotaResetPeriod,
		QuotaResetCustomSeconds: snapshot.QuotaResetCustomSeconds,
		AllowWalletOverflow:     &allowWalletOverflow,
		UpdatedAt:               snapshot.SourceSubscriptionUpdatedAt,
	}
	if _, err := calcPlanEndTime(time.Unix(0, 0), plan); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSubscriptionOrderPlanSnapshot, err)
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) != plan.QuotaResetPeriod ||
		(plan.QuotaResetPeriod == SubscriptionResetCustom && plan.QuotaResetCustomSeconds <= 0) {
		return nil, fmt.Errorf("%w: invalid reset period", ErrSubscriptionOrderPlanSnapshot)
	}
	return plan, nil
}

func ensureSubscriptionPurchaseCapacityTx(tx *gorm.DB, userId, planId, maxPurchase int) error {
	if maxPurchase <= 0 {
		return nil
	}
	var purchased int64
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&purchased).Error; err != nil {
		return err
	}
	var pendingReservations int64
	if err := tx.Model(&SubscriptionOrder{}).
		Where(
			"user_id = ? AND plan_id = ? AND status = ? AND purchase_limit_reserved = ?",
			userId,
			planId,
			common.TopUpStatusPending,
			true,
		).
		Count(&pendingReservations).Error; err != nil {
		return err
	}
	if purchased+pendingReservations >= int64(maxPurchase) {
		return ErrSubscriptionPurchaseLimitReached
	}
	return nil
}

// InsertWithPlanSnapshot atomically snapshots the authoritative checkout contract
// before an external gateway can accept payment. The caller may have obtained plan
// from a stale process-local cache, so the transaction locks and reloads the plan row
// before deciding whether the checkout is still allowed. On success plan is replaced
// with that authoritative row so the controller sends matching product/price data to
// the gateway. Epay additionally reserves a limited-plan slot because its signed
// checkout can be regenerated from the same pending order.
func (o *SubscriptionOrder) InsertWithPlanSnapshot(plan *SubscriptionPlan) error {
	if o == nil {
		return errors.New("subscription order is nil")
	}
	if plan == nil || plan.Id <= 0 || o.PlanId != plan.Id {
		return fmt.Errorf("%w: plan id mismatch", ErrSubscriptionOrderPlanSnapshot)
	}
	if o.Status != common.TopUpStatusPending {
		return errors.New("external subscription order must start pending")
	}
	o.PurchaseLimitReserved = false
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	var authoritativePlan SubscriptionPlan
	err := DB.Transaction(func(tx *gorm.DB) error {
		var currentPlan SubscriptionPlan
		if err := lockForUpdate(tx).Where("id = ?", o.PlanId).First(&currentPlan).Error; err != nil {
			return err
		}
		currentPlan.NormalizeDefaults()
		if !currentPlan.Enabled {
			return fmt.Errorf("%w: plan is disabled", ErrSubscriptionOrderPlanChanged)
		}
		switch o.PaymentProvider {
		case PaymentProviderEpay:
			if currentPlan.PriceAmount < 0.01 {
				return fmt.Errorf("%w: epay amount is below minimum", ErrSubscriptionOrderPlanChanged)
			}
		}

		// The local order and the gateway request must use the same locked row.
		o.Money = currentPlan.PriceAmount
		if err := o.SetPlanSnapshot(&currentPlan); err != nil {
			return err
		}
		if currentPlan.MaxPurchasePerUser <= 0 {
			if err := tx.Create(o).Error; err != nil {
				return err
			}
			authoritativePlan = currentPlan
			return nil
		}

		var userRow User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", o.UserId).First(&userRow).Error; err != nil {
			return err
		}
		if err := ensureSubscriptionPurchaseCapacityTx(tx, o.UserId, o.PlanId, currentPlan.MaxPurchasePerUser); err != nil {
			return err
		}
		// Only Epay currently has a locally reproducible checkout. Reserving other
		// gateways here could permanently block a user if their one-time session URL
		// is lost. Their settlement therefore rechecks the limit instead.
		o.PurchaseLimitReserved = o.PaymentProvider == PaymentProviderEpay
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		authoritativePlan = currentPlan
		return nil
	})
	if err != nil {
		return err
	}
	*plan = authoritativePlan
	return nil
}

func (o *SubscriptionOrder) Insert() error {
	if o == nil {
		return errors.New("subscription order is nil")
	}
	if o.Status == common.TopUpStatusPending {
		return fmt.Errorf("%w: pending order must use InsertWithPlanSnapshot", ErrSubscriptionOrderPlanSnapshot)
	}
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	return DB.Create(o).Error
}

func (o *SubscriptionOrder) Update() error {
	return DB.Save(o).Error
}

func GetSubscriptionOrderByTradeNo(tradeNo string) *SubscriptionOrder {
	if tradeNo == "" {
		return nil
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil
	}
	return &order
}

// GetReusablePendingSubscriptionOrder returns a previously reserved checkout for
// the same gateway. It prefers the requested method, then falls back to another
// method already bound to the order. Epay can regenerate that original signed form
// without changing an order that the gateway may already have seen.
func GetReusablePendingSubscriptionOrder(userId, planId int, paymentProvider, paymentMethod string) (*SubscriptionOrder, error) {
	if userId <= 0 || planId <= 0 || paymentProvider == "" || paymentMethod == "" {
		return nil, errors.New("invalid pending subscription order query")
	}
	var order SubscriptionOrder
	result := DB.Where(
		"user_id = ? AND plan_id = ? AND payment_provider = ? AND payment_method = ? AND status = ? AND plan_snapshot <> '' AND purchase_limit_reserved = ?",
		userId, planId, paymentProvider, paymentMethod, common.TopUpStatusPending, true,
	).Order("id desc").Limit(1).Find(&order)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		return &order, nil
	}
	result = DB.Where(
		"user_id = ? AND plan_id = ? AND payment_provider = ? AND status = ? AND plan_snapshot <> '' AND purchase_limit_reserved = ?",
		userId, planId, paymentProvider, common.TopUpStatusPending, true,
	).Order("id desc").Limit(1).Find(&order)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &order, nil
}

// GetPendingEpaySubscriptionOrders lists recent pending epay subscription orders in
// least-recently-queried order. The bounded batch therefore advances through a finite
// abandoned-order backlog instead of repeatedly selecting the same newest page.
func GetPendingEpaySubscriptionOrders(createdAfter, createdBefore int64, limit int) ([]*SubscriptionOrder, error) {
	var orders []*SubscriptionOrder
	err := DB.Where("payment_provider = ? AND status = ? AND create_time >= ? AND create_time <= ?",
		PaymentProviderEpay, common.TopUpStatusPending, createdAfter, createdBefore).
		Order("COALESCE(epay_query_time, 0) asc").Order("id desc").Limit(limit).Find(&orders).Error
	if err != nil {
		return nil, err
	}
	return orders, nil
}

func MarkEpaySubscriptionOrderQueryAttempts(ids []int, queryTime int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&SubscriptionOrder{}).
		Where("id IN ? AND payment_provider = ? AND status = ?", ids, PaymentProviderEpay, common.TopUpStatusPending).
		Update("epay_query_time", queryTime).Error
}

// ClaimEpaySubscriptionOrderQueryAttempt atomically rate-limits authoritative
// gateway queries for one pending order across application instances.
func ClaimEpaySubscriptionOrderQueryAttempt(id int, queryTime, previousAllowedAt int64) (bool, error) {
	if id <= 0 {
		return false, errors.New("invalid subscription order id")
	}
	result := DB.Model(&SubscriptionOrder{}).
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

// HasPendingEpaySubscriptionOrders reports whether the reconciliation window
// contains any work, allowing the scheduler to stay idle when no order is stuck.
func HasPendingEpaySubscriptionOrders(createdAfter, createdBefore int64) bool {
	var ids []int
	err := DB.Model(&SubscriptionOrder{}).
		Where("payment_provider = ? AND status = ? AND create_time >= ? AND create_time <= ?",
			PaymentProviderEpay, common.TopUpStatusPending, createdAfter, createdBefore).
		Limit(1).Pluck("id", &ids).Error
	return err == nil && len(ids) > 0
}

// User subscription instance
type UserSubscription struct {
	Id     int `json:"id"`
	UserId int `json:"user_id" gorm:"index;index:idx_user_sub_active,priority:1"`
	PlanId int `json:"plan_id" gorm:"index"`

	AmountTotal int64 `json:"amount_total" gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `json:"amount_used" gorm:"type:bigint;not null;default:0"`

	StartTime int64  `json:"start_time" gorm:"bigint"`
	EndTime   int64  `json:"end_time" gorm:"bigint;index;index:idx_user_sub_active,priority:3"`
	Status    string `json:"status" gorm:"type:varchar(32);index;index:idx_user_sub_active,priority:2"` // active/expired/cancelled

	Source string `json:"source" gorm:"type:varchar(32);default:'order'"` // order/admin

	LastResetTime int64 `json:"last_reset_time" gorm:"type:bigint;default:0"`
	NextResetTime int64 `json:"next_reset_time" gorm:"type:bigint;default:0;index"`

	UpgradeGroup  string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`
	PrevUserGroup string `json:"prev_user_group" gorm:"type:varchar(64);default:''"`

	// Downgrade target group on expiry (snapshot from plan; empty = revert to PrevUserGroup)
	DowngradeGroup string `json:"downgrade_group" gorm:"type:varchar(64);default:''"`

	// Whether wallet fallback is allowed after this subscription's quota is exhausted (snapshot from plan)
	AllowWalletOverflow bool `json:"allow_wallet_overflow"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (s *UserSubscription) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (s *UserSubscription) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

type SubscriptionSummary struct {
	Subscription *UserSubscription `json:"subscription"`
}

type SubscriptionResetResult struct {
	PlanId           int    `json:"plan_id"`
	MatchedCount     int    `json:"matched_count"`
	ResetCount       int    `json:"reset_count"`
	UserCount        int    `json:"user_count"`
	AdvanceResetTime bool   `json:"advance_reset_time"`
	PlanTitle        string `json:"-"`
	AffectedUserIds  []int  `json:"-"`
}

func calcPlanEndTime(start time.Time, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("plan is nil")
	}
	if plan.DurationValue <= 0 && plan.DurationUnit != SubscriptionDurationCustom {
		return 0, errors.New("duration_value must be > 0")
	}
	switch plan.DurationUnit {
	case SubscriptionDurationYear:
		return start.AddDate(plan.DurationValue, 0, 0).Unix(), nil
	case SubscriptionDurationMonth:
		return start.AddDate(0, plan.DurationValue, 0).Unix(), nil
	case SubscriptionDurationDay:
		return start.Add(time.Duration(plan.DurationValue) * 24 * time.Hour).Unix(), nil
	case SubscriptionDurationHour:
		return start.Add(time.Duration(plan.DurationValue) * time.Hour).Unix(), nil
	case SubscriptionDurationCustom:
		if plan.CustomSeconds <= 0 {
			return 0, errors.New("custom_seconds must be > 0")
		}
		return start.Add(time.Duration(plan.CustomSeconds) * time.Second).Unix(), nil
	default:
		return 0, fmt.Errorf("invalid duration_unit: %s", plan.DurationUnit)
	}
}

func NormalizeResetPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly, SubscriptionResetCustom:
		return strings.TrimSpace(period)
	default:
		return SubscriptionResetNever
	}
}

func calcNextResetTime(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	if plan == nil {
		return 0
	}
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if period == SubscriptionResetNever {
		return 0
	}
	var next time.Time
	switch period {
	case SubscriptionResetDaily:
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, 1)
	case SubscriptionResetWeekly:
		// Align to next Monday 00:00
		weekday := int(base.Weekday()) // Sunday=0
		// Convert to Monday=1..Sunday=7
		if weekday == 0 {
			weekday = 7
		}
		daysUntil := 8 - weekday
		next = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).
			AddDate(0, 0, daysUntil)
	case SubscriptionResetMonthly:
		// Align to first day of next month 00:00
		next = time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, base.Location()).
			AddDate(0, 1, 0)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds <= 0 {
			return 0
		}
		next = base.Add(time.Duration(plan.QuotaResetCustomSeconds) * time.Second)
	default:
		return 0
	}
	if endUnix > 0 && next.Unix() > endUnix {
		return 0
	}
	return next.Unix()
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	return getSubscriptionPlanByIdTx(nil, id)
}

func getSubscriptionPlanByIdTx(tx *gorm.DB, id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("invalid plan id")
	}
	key := subscriptionPlanCacheKey(id)
	if key != "" {
		if cached, found, err := getSubscriptionPlanCache().Get(key); err == nil && found {
			cached.NormalizeDefaults()
			return &cached, nil
		}
	}
	var plan SubscriptionPlan
	query := DB
	if tx != nil {
		query = tx
	}
	if err := query.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	plan.NormalizeDefaults()
	_ = getSubscriptionPlanCache().SetWithTTL(key, plan, subscriptionPlanCacheTTL())
	return &plan, nil
}

func CountUserSubscriptionsByPlan(userId int, planId int) (int64, error) {
	if userId <= 0 || planId <= 0 {
		return 0, errors.New("invalid userId or planId")
	}
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func getUserGroupByIdTx(tx *gorm.DB, userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid userId")
	}
	if tx == nil {
		tx = DB
	}
	var group string
	if err := lockForUpdate(tx).Model(&User{}).Where("id = ?", userId).Select(commonGroupCol).Find(&group).Error; err != nil {
		return "", err
	}
	return group, nil
}

// resolveOriginalUserGroupTx recovers the original (pre-upgrade) group for a user
// who is currently sitting in upgradedGroup. The baseline MUST be derived from the
// whole subscription chain, not a single row: a renewal or a stacked plan bought
// while the user is already in upgradedGroup is created with an empty
// prev_user_group (see CreateUserSubscriptionFromPlanTx), so trusting one row would
// lose the baseline. We pick the earliest subscription that recorded a real,
// different previous group. Returns "" when no baseline is recoverable.
func resolveOriginalUserGroupTx(tx *gorm.DB, userId int, upgradedGroup string) (string, error) {
	upgradedGroup = strings.TrimSpace(upgradedGroup)
	if tx == nil || userId <= 0 || upgradedGroup == "" {
		return "", nil
	}
	var origin UserSubscription
	q := tx.Where("user_id = ? AND upgrade_group = ? AND prev_user_group <> '' AND prev_user_group <> ?",
		userId, upgradedGroup, upgradedGroup).
		Order("start_time asc, id asc").
		Limit(1).
		Find(&origin)
	if q.Error != nil {
		return "", q.Error
	}
	if q.RowsAffected == 0 {
		return "", nil
	}
	return strings.TrimSpace(origin.PrevUserGroup), nil
}

func downgradeUserGroupForSubscriptionTx(tx *gorm.DB, sub *UserSubscription, now int64) (string, error) {
	if tx == nil || sub == nil {
		return "", errors.New("invalid downgrade args")
	}
	downgradeGroup := strings.TrimSpace(sub.DowngradeGroup)
	upgradeGroup := strings.TrimSpace(sub.UpgradeGroup)
	// Nothing to do if neither an explicit downgrade target nor an upgrade snapshot exists.
	if downgradeGroup == "" && upgradeGroup == "" {
		return "", nil
	}
	currentGroup, err := getUserGroupByIdTx(tx, sub.UserId)
	if err != nil {
		return "", err
	}
	// If another active upgraded subscription exists, keep the current group.
	var activeSub UserSubscription
	activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND id <> ? AND upgrade_group <> ''",
		sub.UserId, "active", now, sub.Id).
		Order("end_time desc, id desc").
		Limit(1).
		Find(&activeSub)
	if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
		return "", nil
	}
	// Determine the downgrade target: an explicit downgrade group takes precedence,
	// otherwise revert to the group held before purchase. When no baseline was recorded
	// (e.g. a renewal created while the user was already in upgradeGroup), recover the
	// original group from the chain instead of silently bailing out.
	target := downgradeGroup
	if target == "" {
		prevGroup := strings.TrimSpace(sub.PrevUserGroup)
		if prevGroup == "" {
			resolved, err := resolveOriginalUserGroupTx(tx, sub.UserId, upgradeGroup)
			if err != nil {
				return "", err
			}
			prevGroup = resolved
		}
		target = prevGroup
	}
	if target == "" || target == currentGroup {
		return "", nil
	}
	if err := tx.Model(&User{}).Where("id = ?", sub.UserId).
		Update("group", target).Error; err != nil {
		return "", err
	}
	return target, nil
}

func CreateUserSubscriptionFromPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if plan == nil || plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if err := ensureSubscriptionPurchaseCapacityTx(tx, userId, plan.Id, plan.MaxPurchasePerUser); err != nil {
		return nil, err
	}
	nowUnix := GetDBTimestampTx(tx)
	now := time.Unix(nowUnix, 0)
	endUnix, err := calcPlanEndTime(now, plan)
	if err != nil {
		return nil, err
	}
	resetBase := now
	nextReset := calcNextResetTime(resetBase, plan, endUnix)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = now.Unix()
	}
	upgradeGroup := strings.TrimSpace(plan.UpgradeGroup)
	prevGroup := ""
	if upgradeGroup != "" {
		currentGroup, err := getUserGroupByIdTx(tx, userId)
		if err != nil {
			return nil, err
		}
		if currentGroup != upgradeGroup {
			prevGroup = currentGroup
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", upgradeGroup).Error; err != nil {
				return nil, err
			}
		} else {
			// User is already in upgradeGroup (renewal, or stacking another plan that
			// grants the same group). currentGroup is NOT the original baseline, so
			// inherit it from the existing subscription chain; otherwise this later-
			// ending row would store an empty prev_user_group and the expiry revert
			// would have no baseline to fall back to.
			inherited, err := resolveOriginalUserGroupTx(tx, userId, upgradeGroup)
			if err != nil {
				return nil, err
			}
			prevGroup = inherited
		}
	}
	allowWalletOverflow := true
	if plan.AllowWalletOverflow != nil {
		allowWalletOverflow = *plan.AllowWalletOverflow
	}
	sub := &UserSubscription{
		UserId:              userId,
		PlanId:              plan.Id,
		AmountTotal:         plan.TotalAmount,
		AmountUsed:          0,
		StartTime:           now.Unix(),
		EndTime:             endUnix,
		Status:              "active",
		Source:              source,
		LastResetTime:       lastReset,
		NextResetTime:       nextReset,
		UpgradeGroup:        upgradeGroup,
		PrevUserGroup:       prevGroup,
		DowngradeGroup:      strings.TrimSpace(plan.DowngradeGroup),
		AllowWalletOverflow: allowWalletOverflow,
		CreatedAt:           common.GetTimestamp(),
		UpdatedAt:           common.GetTimestamp(),
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

func refreshSubscriptionUserGroupCache(userId int, operation string) {
	if err := RefreshUserGroupCache(userId); err != nil {
		common.SysError(fmt.Sprintf("failed to refresh user group cache after %s for user %d: %v", operation, userId, err))
	}
}

// Complete a subscription order (idempotent). Creates a UserSubscription snapshot from the plan.
// expectedPaymentProvider guards against cross-gateway callback attacks (empty skips the check).
// actualPaymentMethod updates the order's PaymentMethod to reflect the real payment type used (empty skips update).
func CompleteSubscriptionOrder(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) error {
	_, err := CompleteSubscriptionOrderWithResult(tradeNo, providerPayload, expectedPaymentProvider, actualPaymentMethod)
	return err
}

// CompleteSubscriptionOrderWithResult has the same idempotent contract and also
// reports whether this call performed the pending-to-success transition. Callers that
// expose reconciliation metrics use it to distinguish their work from a racing winner.
func CompleteSubscriptionOrderWithResult(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) (bool, error) {
	if tradeNo == "" {
		return false, errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	// Read immutable routing fields without a lock first so every mutating path can
	// acquire locks in the same user -> order order. The locked row is fully
	// revalidated inside the transaction before any entitlement is granted.
	var initialOrder SubscriptionOrder
	if err := DB.Select("user_id", "plan_id", "payment_provider", "status", "plan_snapshot", "purchase_limit_reserved").
		Where(refCol+" = ?", tradeNo).First(&initialOrder).Error; err != nil {
		return false, ErrSubscriptionOrderNotFound
	}
	if expectedPaymentProvider != "" && initialOrder.PaymentProvider != expectedPaymentProvider {
		return false, ErrPaymentMethodMismatch
	}
	if initialOrder.Status == common.TopUpStatusSuccess {
		return false, nil
	}
	if initialOrder.Status != common.TopUpStatusPending {
		return false, ErrSubscriptionOrderStatusInvalid
	}

	var logUserId int
	var logPlanTitle string
	var logMoney float64
	var logPaymentMethod string
	var upgradeGroup string
	completed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var currentPlan SubscriptionPlan
		legacyOrder := strings.TrimSpace(initialOrder.PlanSnapshot) == ""
		if legacyOrder || !initialOrder.PurchaseLimitReserved {
			// Lock the current plan before the user row, matching balance purchases.
			// Legacy orders need it for compatibility validation; snapshot orders that
			// did not reserve a slot use only its current purchase limit, so an old
			// unlimited-plan form cannot bypass a limit added after checkout.
			if err := lockForUpdate(tx).Where("id = ?", initialOrder.PlanId).First(&currentPlan).Error; err != nil {
				return ErrSubscriptionOrderPlanChanged
			}
			currentPlan.NormalizeDefaults()
		}

		var userRow User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", initialOrder.UserId).First(&userRow).Error; err != nil {
			return err
		}

		var order SubscriptionOrder
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if order.UserId != initialOrder.UserId || order.PlanId != initialOrder.PlanId ||
			order.PurchaseLimitReserved != initialOrder.PurchaseLimitReserved {
			return ErrSubscriptionOrderStatusInvalid
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return ErrSubscriptionOrderStatusInvalid
		}

		var plan *SubscriptionPlan
		var err error
		if strings.TrimSpace(order.PlanSnapshot) != "" {
			plan, err = subscriptionPlanFromOrderSnapshot(&order)
			if err != nil {
				return err
			}
		} else {
			if !legacyOrder || currentPlan.Id != order.PlanId || currentPlan.UpdatedAt >= order.CreateTime ||
				math.IsNaN(order.Money) || math.IsInf(order.Money, 0) ||
				math.IsNaN(currentPlan.PriceAmount) || math.IsInf(currentPlan.PriceAmount, 0) ||
				!decimal.NewFromFloat(currentPlan.PriceAmount).Equal(decimal.NewFromFloat(order.Money)) {
				return ErrSubscriptionOrderPlanChanged
			}
			plan = &currentPlan
		}

		// Only an order carrying a durable reservation may skip the purchase-limit
		// check at settlement. Unreserved gateways keep the legacy settlement check;
		// treating every external pending row as reserved would both bypass limits and
		// let abandoned one-time checkout rows block the account forever.
		fulfillmentPlan := *plan
		if order.PurchaseLimitReserved {
			fulfillmentPlan.MaxPurchasePerUser = 0
		} else if !legacyOrder {
			fulfillmentPlan.MaxPurchasePerUser = currentPlan.MaxPurchasePerUser
		}
		if !order.PurchaseLimitReserved && fulfillmentPlan.MaxPurchasePerUser > 0 {
			capacityErr := ensureSubscriptionPurchaseCapacityTx(
				tx,
				order.UserId,
				order.PlanId,
				fulfillmentPlan.MaxPurchasePerUser,
			)
			if capacityErr != nil && !errors.Is(capacityErr, ErrSubscriptionPurchaseLimitReached) {
				return capacityErr
			}
			if errors.Is(capacityErr, ErrSubscriptionPurchaseLimitReached) {
				// A reproducible Epay reservation may have been created after this older,
				// one-time checkout. Once the older order is authoritatively paid, let it
				// take one slot from the newest later reservation. No reservation is
				// disturbed while spare capacity exists. The user row lock serializes this
				// transfer with every checkout/settlement for the account.
				var laterReservation SubscriptionOrder
				if err := lockForUpdate(tx).
					Select("id").
					Where(
						"user_id = ? AND plan_id = ? AND id > ? AND status = ? AND purchase_limit_reserved = ?",
						order.UserId,
						order.PlanId,
						order.Id,
						common.TopUpStatusPending,
						true,
					).
					Order("id desc").
					First(&laterReservation).Error; err != nil {
					return capacityErr
				}
				if err := tx.Model(&SubscriptionOrder{}).
					Where("id = ? AND status = ? AND purchase_limit_reserved = ?", laterReservation.Id, common.TopUpStatusPending, true).
					Update("purchase_limit_reserved", false).Error; err != nil {
					return err
				}
			}
		}
		subscription, err := CreateUserSubscriptionFromPlanTx(tx, order.UserId, &fulfillmentPlan, "order")
		if err != nil {
			return err
		}
		if subscription.PrevUserGroup != "" {
			upgradeGroup = strings.TrimSpace(subscription.UpgradeGroup)
		}
		if err := upsertSubscriptionTopUpTx(tx, &order); err != nil {
			return err
		}
		order.Status = common.TopUpStatusSuccess
		order.CompleteTime = common.GetTimestamp()
		if providerPayload != "" {
			order.ProviderPayload = providerPayload
		}
		if actualPaymentMethod != "" && order.PaymentMethod != actualPaymentMethod {
			order.PaymentMethod = actualPaymentMethod
		}
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		logUserId = order.UserId
		logPlanTitle = plan.Title
		logMoney = order.Money
		logPaymentMethod = order.PaymentMethod
		completed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if upgradeGroup != "" && logUserId > 0 {
		refreshSubscriptionUserGroupCache(logUserId, "subscription payment completion")
	}
	if logUserId > 0 {
		msg := fmt.Sprintf("订阅购买成功，套餐: %s，支付金额: %.2f，支付方式: %s", logPlanTitle, logMoney, logPaymentMethod)
		RecordLog(logUserId, LogTypeTopup, msg)
	}
	return completed, nil
}

func upsertSubscriptionTopUpTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid subscription order")
	}
	now := common.GetTimestamp()
	var topup TopUp
	if err := tx.Where("trade_no = ?", order.TradeNo).First(&topup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			topup = TopUp{
				UserId:        order.UserId,
				Amount:        0,
				Money:         order.Money,
				TradeNo:       order.TradeNo,
				PaymentMethod: order.PaymentMethod,
				CreateTime:    order.CreateTime,
				CompleteTime:  now,
				Status:        common.TopUpStatusSuccess,
			}
			return tx.Create(&topup).Error
		}
		return err
	}
	topup.Money = order.Money
	if topup.PaymentMethod == "" {
		topup.PaymentMethod = order.PaymentMethod
	} else if topup.PaymentMethod != order.PaymentMethod {
		return ErrPaymentMethodMismatch
	}
	if topup.CreateTime == 0 {
		topup.CreateTime = order.CreateTime
	}
	topup.CompleteTime = now
	topup.Status = common.TopUpStatusSuccess
	return tx.Save(&topup).Error
}

func ExpireSubscriptionOrder(tradeNo string, expectedPaymentProvider string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusPending {
			return nil
		}
		order.Status = common.TopUpStatusExpired
		order.CompleteTime = common.GetTimestamp()
		return tx.Save(&order).Error
	})
}

// Admin bind (no payment). Creates a UserSubscription from a plan.
func AdminBindSubscription(userId int, planId int, sourceNote string) (string, error) {
	if userId <= 0 || planId <= 0 {
		return "", errors.New("invalid userId or planId")
	}
	plan, err := GetSubscriptionPlanById(planId)
	if err != nil {
		return "", err
	}
	groupChanged := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		// 与 CompleteSubscriptionOrder 一致：先锁用户行，再做购买次数检查。
		var userRow User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", userId).First(&userRow).Error; err != nil {
			return err
		}
		subscription, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "admin")
		if err == nil {
			groupChanged = subscription.PrevUserGroup != ""
		}
		return err
	})
	if err != nil {
		return "", err
	}
	if groupChanged {
		refreshSubscriptionUserGroupCache(userId, "admin subscription creation")
		return fmt.Sprintf("用户分组将升级到 %s", plan.UpgradeGroup), nil
	}
	return "", nil
}

func calcSubscriptionBalanceQuota(priceAmount float64) (int, error) {
	if priceAmount <= 0 {
		return 0, nil
	}
	if common.QuotaPerUnit <= 0 {
		return 0, errors.New("额度单位配置错误")
	}
	// Ceil first (charge at least the exact price), then reject values outside
	// the int32 quota domain before a balance purchase can mutate state.
	quota := decimal.NewFromFloat(priceAmount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Ceil()
	return common.QuotaFromDecimalStrict(quota)
}

var (
	runSubscriptionBalanceTransaction = func(fn func(*gorm.DB) error) error {
		return DB.Transaction(fn)
	}
	beforeSubscriptionBalanceTransactionCommit = func() error { return nil }
	resolveSubscriptionBalancePurchaseCommitFn = resolveSubscriptionBalancePurchaseCommit
)

func resolveSubscriptionBalancePurchaseCommit(tradeNo string, userId int, planId int) (bool, error) {
	var order SubscriptionOrder
	result := DB.Where(
		"trade_no = ? AND user_id = ? AND plan_id = ? AND payment_method = ? AND payment_provider = ? AND status = ?",
		tradeNo, userId, planId, PaymentMethodBalance, PaymentProviderBalance, common.TopUpStatusSuccess,
	).Limit(1).Find(&order)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func resolveSubscriptionBalancePurchaseCommitByTradeNo(tradeNo string, userId int) (bool, error) {
	var order SubscriptionOrder
	result := DB.Where(
		"trade_no = ? AND user_id = ? AND payment_method = ? AND payment_provider = ? AND status = ?",
		tradeNo, userId, PaymentMethodBalance, PaymentProviderBalance, common.TopUpStatusSuccess,
	).Limit(1).Find(&order)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func subscriptionBalanceQuotaOperationKey(tradeNo string) string {
	return "subscription_balance_quota:" + tradeNo
}

func subscriptionBalanceQuotaOperationTTLSeconds() int {
	ttl := userCacheTTLSeconds() * 10
	if ttl < 300 {
		return 300
	}
	return ttl
}

func subscriptionBalanceQuotaFenceValue(tradeNo string) string {
	return "inflight:" + tradeNo
}

const reserveSubscriptionBalanceQuotaScript = `
local state = redis.call('HGET', KEYS[3], 'state')
local fence = redis.call('GET', KEYS[2])
if state then
  if tonumber(redis.call('HGET', KEYS[3], 'user_id') or '0') ~= tonumber(ARGV[2])
    or tonumber(redis.call('HGET', KEYS[3], 'quota') or '-1') ~= tonumber(ARGV[1]) then
    return -3
  end
  if state ~= 'reserved' then
    return -3
  end
  if fence and fence ~= ARGV[5] then
    return -2
  end
  if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
    or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
    or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
    return -1
  end
  redis.call('SET', KEYS[2], ARGV[5], 'EX', ARGV[4])
  return 2
end
if fence then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
local quota = tonumber(redis.call('HGET', KEYS[1], 'Quota'))
if quota == nil or quota < tonumber(ARGV[1]) then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'Quota', -tonumber(ARGV[1]))
redis.call('HSET', KEYS[3], 'state', 'reserved', 'user_id', ARGV[2], 'quota', ARGV[1])
redis.call('EXPIRE', KEYS[3], ARGV[4])
redis.call('SET', KEYS[2], ARGV[5], 'EX', ARGV[4])
return 1`

const compensateSubscriptionBalanceQuotaScript = `
local state = redis.call('HGET', KEYS[3], 'state')
if not state then
  return -1
end
if tonumber(redis.call('HGET', KEYS[3], 'user_id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[3], 'quota') or '-1') ~= tonumber(ARGV[1]) then
  return -3
end
if state == 'compensated' then
  return 2
end
if state ~= 'reserved' then
  return -3
end
if redis.call('GET', KEYS[2]) ~= ARGV[5] then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  redis.call('HSET', KEYS[3], 'state', 'compensated')
  redis.call('EXPIRE', KEYS[3], ARGV[4])
  redis.call('SET', KEYS[2], ARGV[6], 'EX', ARGV[4])
  redis.call('DEL', KEYS[1])
  return 3
end
redis.call('HINCRBY', KEYS[1], 'Quota', tonumber(ARGV[1]))
redis.call('HSET', KEYS[3], 'state', 'compensated')
redis.call('EXPIRE', KEYS[3], ARGV[4])
redis.call('DEL', KEYS[2])
return 1`

const commitSubscriptionBalanceQuotaScript = `
local state = redis.call('HGET', KEYS[3], 'state')
if not state then
  return -1
end
if tonumber(redis.call('HGET', KEYS[3], 'user_id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[3], 'quota') or '-1') ~= tonumber(ARGV[1]) then
  return -3
end
if state == 'committed' then
  return 2
end
if state ~= 'reserved' then
  return -3
end
local fence = redis.call('GET', KEYS[2])
if fence == ARGV[4] then
  redis.call('HSET', KEYS[3], 'state', 'committed')
  redis.call('EXPIRE', KEYS[3], ARGV[3])
  redis.call('DEL', KEYS[1])
  redis.call('DEL', KEYS[2])
  return 1
end
if not fence then
  redis.call('HSET', KEYS[3], 'state', 'committed')
  redis.call('EXPIRE', KEYS[3], ARGV[3])
  redis.call('DEL', KEYS[1])
  redis.call('SET', KEYS[2], ARGV[5], 'EX', ARGV[3])
  return 3
end
redis.call('DEL', KEYS[1])
return -2`

const markSubscriptionBalanceQuotaUnknownScript = `
local state = redis.call('HGET', KEYS[3], 'state')
local fence = redis.call('GET', KEYS[2])
if not state then
  if fence and fence ~= ARGV[5] and fence ~= ARGV[4] then
    redis.call('DEL', KEYS[1])
    return -2
  end
  redis.call('HSET', KEYS[3], 'state', 'unknown', 'user_id', ARGV[2], 'quota', ARGV[1])
  redis.call('EXPIRE', KEYS[3], ARGV[3])
  redis.call('SET', KEYS[2], ARGV[4])
  redis.call('DEL', KEYS[1])
  return 1
end
if tonumber(redis.call('HGET', KEYS[3], 'user_id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[3], 'quota') or '-1') ~= tonumber(ARGV[1]) then
  return -3
end
if state == 'unknown' then
  if fence == ARGV[4] then
    redis.call('DEL', KEYS[1])
    return 2
  end
  redis.call('DEL', KEYS[1])
  return -2
end
if state ~= 'reserved' then
  return -3
end
if fence and fence ~= ARGV[5] then
  redis.call('DEL', KEYS[1])
  return -2
end
redis.call('HSET', KEYS[3], 'state', 'unknown')
redis.call('EXPIRE', KEYS[3], ARGV[3])
redis.call('SET', KEYS[2], ARGV[4])
redis.call('DEL', KEYS[1])
return 1`

func reserveSubscriptionBalanceCacheQuota(userId int, quota int, tradeNo string) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), reserveSubscriptionBalanceQuotaScript,
		[]string{getUserCacheKey(userId), getUserQuotaUncertaintyKey(userId), subscriptionBalanceQuotaOperationKey(tradeNo)},
		quota, userId, userCacheSchemaVersion, subscriptionBalanceQuotaOperationTTLSeconds(),
		subscriptionBalanceQuotaFenceValue(tradeNo)).Int()
	if err != nil {
		return cacheQuotaMiss, err
	}
	switch result {
	case 1, 2:
		return cacheQuotaOK, nil
	case 0:
		return cacheQuotaInsufficient, nil
	case -1:
		return cacheQuotaMiss, nil
	case -2:
		return cacheQuotaFenced, nil
	default:
		return cacheQuotaMiss, errors.New("subscription balance quota operation conflicts with its Redis journal")
	}
}

func compensateSubscriptionBalanceCacheDebit(userId int, quota int, tradeNo string) error {
	if !common.RedisEnabled || quota <= 0 {
		return nil
	}
	result, err := common.RDB.Eval(context.Background(), compensateSubscriptionBalanceQuotaScript,
		[]string{getUserCacheKey(userId), getUserQuotaUncertaintyKey(userId), subscriptionBalanceQuotaOperationKey(tradeNo)},
		quota, userId, userCacheSchemaVersion, subscriptionBalanceQuotaOperationTTLSeconds(),
		subscriptionBalanceQuotaFenceValue(tradeNo), "subscription_rollback:"+tradeNo).Int()
	if err == nil && (result == 1 || result == 2 || result == 3) {
		return nil
	}

	cacheErr := fmt.Errorf("restore subscription balance cache debit: user=%d quota=%d result=%d: %w",
		userId, quota, result, errors.Join(ErrQuotaCacheUnavailable, err))
	fenceErr := fenceUserQuotaCacheUncertainty(userId, "subscription_balance_rollback_cache_error",
		subscriptionBalanceQuotaFenceValue(tradeNo))
	return errors.Join(cacheErr, fenceErr)
}

func finalizeCommittedSubscriptionBalanceCacheDebit(userId int, quota int, tradeNo string) {
	if !common.RedisEnabled || quota <= 0 {
		return
	}
	ttl := subscriptionBalanceQuotaOperationTTLSeconds()
	result, err := common.RDB.Eval(context.Background(), commitSubscriptionBalanceQuotaScript,
		[]string{getUserCacheKey(userId), getUserQuotaUncertaintyKey(userId), subscriptionBalanceQuotaOperationKey(tradeNo)},
		quota, userId, ttl, subscriptionBalanceQuotaFenceValue(tradeNo), "subscription_commit_reconcile:"+tradeNo).Int()
	if err != nil || (result != 1 && result != 2 && result != 3) {
		common.SysError(fmt.Sprintf("failed to finalize committed subscription balance cache debit: user=%d quota=%d order=%s result=%d error=%v",
			userId, quota, tradeNo, result, err))
		if fenceErr := fenceUserQuotaCacheUncertainty(userId, "subscription_balance_commit_finalize_unknown:"+tradeNo,
			subscriptionBalanceQuotaFenceValue(tradeNo)); fenceErr != nil {
			common.SysError(fmt.Sprintf("failed to fence subscription balance cache after finalize error: user=%d order=%s error=%v",
				userId, tradeNo, fenceErr))
		}
	}
	if hydrateErr := ensureUserQuotaCacheAvailable(userId); hydrateErr != nil {
		// A surviving in-flight fence is stale-low and therefore safe. Keep the
		// successful durable purchase while the next request retries reconciliation.
		common.SysError(fmt.Sprintf("failed to hydrate committed subscription balance: user=%d order=%s error=%v",
			userId, tradeNo, hydrateErr))
	}
}

func markSubscriptionBalanceCacheCommitUnknown(userId int, quota int, tradeNo string) error {
	if !common.RedisEnabled || quota <= 0 {
		return nil
	}
	ttl := subscriptionBalanceQuotaOperationTTLSeconds()
	result, err := common.RDB.Eval(context.Background(), markSubscriptionBalanceQuotaUnknownScript,
		[]string{getUserCacheKey(userId), getUserQuotaUncertaintyKey(userId), subscriptionBalanceQuotaOperationKey(tradeNo)},
		quota, userId, ttl, "subscription_commit_unknown:"+tradeNo, subscriptionBalanceQuotaFenceValue(tradeNo)).Int()
	if err == nil && (result == 1 || result == 2) {
		return nil
	}
	markErr := fmt.Errorf("mark subscription balance cache commit unknown: user=%d quota=%d order=%s result=%d: %w",
		userId, quota, tradeNo, result, errors.Join(ErrQuotaCacheUnavailable, err))
	fenceErr := fenceUserQuotaCacheUntilReconciled(userId, "subscription_commit_unknown:"+tradeNo,
		subscriptionBalanceQuotaFenceValue(tradeNo))
	return errors.Join(markErr, fenceErr)
}

// PurchaseSubscriptionWithBalance creates a subscription by deducting the user's wallet quota.
func PurchaseSubscriptionWithBalance(userId int, planId int) error {
	if userId <= 0 || planId <= 0 {
		return errors.New("invalid userId or planId")
	}

	plan, err := GetSubscriptionPlanById(planId)
	if err != nil {
		return err
	}
	if !plan.Enabled {
		return errors.New("套餐未启用")
	}
	if plan.PriceAmount < 0 {
		return errors.New("套餐价格不能为负数")
	}
	if plan.AllowBalancePay != nil && !*plan.AllowBalancePay {
		return errors.New("该套餐不允许使用余额兑换")
	}
	requiredQuota, err := calcSubscriptionBalanceQuota(plan.PriceAmount)
	if err != nil {
		return err
	}
	expectedPriceAmount := plan.PriceAmount
	expectedCurrency := plan.Currency
	if requiredQuota > 0 && common.RedisEnabled {
		// Hydrate and validate the authoritative hash before opening a database
		// transaction. Once the transaction holds a row lock, a cache miss must
		// fail closed instead of reading through a second database connection.
		if err := ensureUserQuotaCacheAvailable(userId); err != nil {
			return err
		}
	}

	var logPlanTitle string
	var logMoney float64
	chargedQuota := requiredQuota
	var upgradeGroup string
	tradeNo := fmt.Sprintf("SUBBALUSR%dNO%s%d", userId, common.GetRandomString(6), time.Now().UnixNano())
	cacheDebited := false
	transactionCallbackCompleted := false
	err = runSubscriptionBalanceTransaction(func(tx *gorm.DB) error {
		// Re-read and lock the authoritative row. The first lookup may have come
		// from the plan cache; charging from that stale snapshot could accept a
		// disabled plan or an old price.
		var currentPlan SubscriptionPlan
		if err := lockForUpdate(tx).Where("id = ?", planId).First(&currentPlan).Error; err != nil {
			return err
		}
		currentPlan.NormalizeDefaults()
		plan := &currentPlan
		if !plan.Enabled {
			return errors.New("套餐未启用")
		}
		if plan.PriceAmount < 0 {
			return errors.New("套餐价格不能为负数")
		}
		if plan.AllowBalancePay != nil && !*plan.AllowBalancePay {
			return errors.New("该套餐不允许使用余额兑换")
		}

		currentRequiredQuota, err := calcSubscriptionBalanceQuota(plan.PriceAmount)
		if err != nil {
			return err
		}
		if !decimal.NewFromFloat(plan.PriceAmount).Equal(decimal.NewFromFloat(expectedPriceAmount)) ||
			plan.Currency != expectedCurrency || currentRequiredQuota != requiredQuota {
			return errors.New("套餐价格已变更，请重试")
		}

		// Serialize the durable debit and MaxPurchasePerUser checks for this
		// account. Redis is reserved only after this lock is held, and the matching
		// database debit commits atomically with the order and subscription.
		var userRow User
		if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", userId).First(&userRow).Error; err != nil {
			return err
		}
		if requiredQuota > 0 {
			if common.RedisEnabled {
				result, reserveErr := reserveSubscriptionBalanceCacheQuota(userId, requiredQuota, tradeNo)
				if reserveErr != nil {
					fenceErr := fenceUserQuotaCacheUncertainty(userId, "subscription_balance_reserve_unknown:"+tradeNo,
						subscriptionBalanceQuotaFenceValue(tradeNo))
					return errors.Join(fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, userId), reserveErr, fenceErr)
				}
				if result == cacheQuotaMiss || result == cacheQuotaFenced {
					// A deterministic miss/fence means this reservation did not mutate
					// Redis. In particular, never overwrite another operation's in-flight
					// fence: its owner must retain the ability to finalize atomically.
					return fmt.Errorf("%w: user %d", ErrQuotaCacheUnavailable, userId)
				}
				if result == cacheQuotaInsufficient {
					return errors.New("余额不足")
				}
				cacheDebited = true
			}

			debit := tx.Model(&User{}).
				Where("id = ? AND quota >= ?", userId, requiredQuota).
				Update("quota", gorm.Expr("quota - ?", requiredQuota))
			if debit.Error != nil {
				return debit.Error
			}
			if debit.RowsAffected != 1 {
				return errors.New("余额不足")
			}
		}

		subscription, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, PaymentMethodBalance)
		if err != nil {
			return err
		}

		now := common.GetTimestamp()
		order := &SubscriptionOrder{
			UserId:          userId,
			PlanId:          plan.Id,
			Money:           plan.PriceAmount,
			TradeNo:         tradeNo,
			PaymentMethod:   PaymentMethodBalance,
			PaymentProvider: PaymentProviderBalance,
			Status:          common.TopUpStatusSuccess,
			CreateTime:      now,
			CompleteTime:    now,
			ProviderPayload: fmt.Sprintf("charged_quota=%d", requiredQuota),
		}
		if err := order.SetPlanSnapshot(plan); err != nil {
			return err
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		logPlanTitle = plan.Title
		logMoney = plan.PriceAmount
		if subscription.PrevUserGroup != "" {
			upgradeGroup = strings.TrimSpace(subscription.UpgradeGroup)
		}
		if err := beforeSubscriptionBalanceTransactionCommit(); err != nil {
			return err
		}
		transactionCallbackCompleted = true
		return nil
	})
	if err != nil {
		if !transactionCallbackCompleted {
			if cacheDebited {
				err = errors.Join(err, compensateSubscriptionBalanceCacheDebit(userId, chargedQuota, tradeNo))
			}
			return err
		}

		// The callback completed, so the driver error may have arrived after the
		// server committed. The order is the durable journal for the same
		// transaction as the debit and subscription.
		committed, reconcileErr := resolveSubscriptionBalancePurchaseCommitFn(tradeNo, userId, planId)
		if reconcileErr != nil || !committed {
			var fenceErr error
			if cacheDebited {
				fenceErr = markSubscriptionBalanceCacheCommitUnknown(userId, chargedQuota, tradeNo)
			}
			common.SysError(fmt.Sprintf("subscription balance commit status is uncertain for user %d order %s: committed=%t error=%v",
				userId, tradeNo, committed, reconcileErr))
			return errors.Join(err, reconcileErr, fenceErr, ErrSubscriptionBalanceCommitUncertain)
		}
		common.SysLog(fmt.Sprintf("subscription balance order %s committed despite transaction result: %v", tradeNo, err))
	}
	if cacheDebited {
		finalizeCommittedSubscriptionBalanceCacheDebit(userId, chargedQuota, tradeNo)
	}

	if upgradeGroup != "" {
		refreshSubscriptionUserGroupCache(userId, "subscription balance purchase")
	}
	msg := fmt.Sprintf("使用余额购买订阅成功，套餐: %s，支付金额: %.2f，扣除额度: %d", logPlanTitle, logMoney, chargedQuota)
	RecordLog(userId, LogTypeTopup, msg)
	return nil
}

// GetAllActiveUserSubscriptions returns all active subscriptions for a user.
func GetAllActiveUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	err := DB.Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

// HasActiveUserSubscription returns whether the user has any active subscription.
// This is a lightweight existence check to avoid heavy pre-consume transactions.
func HasActiveUserSubscription(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UserActiveSubscriptionsAllowWalletOverflow returns whether wallet balance may be used
// after the user's subscription quota is exhausted. A single active subscription that
// disallows wallet overflow (allow_wallet_overflow = false) blocks the fallback.
func UserActiveSubscriptionsAllowWalletOverflow(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var strictCount int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ? AND allow_wallet_overflow = ?",
			userId, "active", now, false).
		Count(&strictCount).Error; err != nil {
		return false, err
	}
	return strictCount == 0, nil
}

// GetAllUserSubscriptions returns all subscriptions (active and expired) for a user.
func GetAllUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var subs []UserSubscription
	err := DB.Where("user_id = ?", userId).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

func buildSubscriptionSummaries(subs []UserSubscription) []SubscriptionSummary {
	if len(subs) == 0 {
		return []SubscriptionSummary{}
	}
	result := make([]SubscriptionSummary, 0, len(subs))
	for _, sub := range subs {
		subCopy := sub
		result = append(result, SubscriptionSummary{
			Subscription: &subCopy,
		})
	}
	return result
}

// AdminInvalidateUserSubscription marks a user subscription as cancelled and ends it immediately.
func AdminInvalidateUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		if err := tx.Model(&sub).Updates(map[string]interface{}{
			"status":     "cancelled",
			"end_time":   now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" && userId > 0 {
		refreshSubscriptionUserGroupCache(userId, "admin subscription update")
	}
	if downgradeGroup != "" {
		return fmt.Sprintf("用户分组将回退到 %s", downgradeGroup), nil
	}
	return "", nil
}

// AdminDeleteUserSubscription hard-deletes a user subscription.
func AdminDeleteUserSubscription(userSubscriptionId int) (string, error) {
	if userSubscriptionId <= 0 {
		return "", errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		if err := tx.Where("id = ?", userSubscriptionId).Delete(&UserSubscription{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if cacheGroup != "" && userId > 0 {
		refreshSubscriptionUserGroupCache(userId, "admin subscription deletion")
	}
	if downgradeGroup != "" {
		return fmt.Sprintf("用户分组将回退到 %s", downgradeGroup), nil
	}
	return "", nil
}

func resetUserSubscriptionTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64, advanceResetTime bool) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	// Update only the reset columns: a full-row Save would write back the stale
	// snapshot and could clobber a concurrent PostConsumeUserSubscriptionDelta.
	updates := map[string]interface{}{
		"amount_used": 0,
		"updated_at":  now,
	}
	sub.AmountUsed = 0
	if advanceResetTime {
		nextReset := calcNextResetTime(time.Unix(now, 0), plan, sub.EndTime)
		sub.NextResetTime = nextReset
		if nextReset > 0 {
			sub.LastResetTime = now
		} else {
			sub.LastResetTime = 0
		}
		updates["next_reset_time"] = sub.NextResetTime
		updates["last_reset_time"] = sub.LastResetTime
	}
	return tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error
}

func buildSubscriptionResetResult(plan *SubscriptionPlan, subs []UserSubscription, advanceResetTime bool) *SubscriptionResetResult {
	userIds := make([]int, 0, len(subs))
	seenUsers := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if _, ok := seenUsers[sub.UserId]; ok {
			continue
		}
		seenUsers[sub.UserId] = struct{}{}
		userIds = append(userIds, sub.UserId)
	}
	return &SubscriptionResetResult{
		PlanId:           plan.Id,
		MatchedCount:     len(subs),
		ResetCount:       len(subs),
		UserCount:        len(userIds),
		AdvanceResetTime: advanceResetTime,
		PlanTitle:        plan.Title,
		AffectedUserIds:  userIds,
	}
}

func adminResetUserSubscriptionsByPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, now int64, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if tx == nil || plan == nil {
		return nil, errors.New("invalid reset args")
	}
	var subs []UserSubscription
	if err := lockForUpdate(tx).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, plan.Id, "active", now).
		Order("end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, errors.New("该用户没有有效的此套餐订阅")
	}
	for i := range subs {
		if err := resetUserSubscriptionTx(tx, &subs[i], plan, now, advanceResetTime); err != nil {
			return nil, err
		}
	}
	return buildSubscriptionResetResult(plan, subs, advanceResetTime), nil
}

func adminResetPlanSubscriptionsTx(tx *gorm.DB, plan *SubscriptionPlan, now int64, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if tx == nil || plan == nil {
		return nil, errors.New("invalid reset args")
	}
	var subs []UserSubscription
	if err := lockForUpdate(tx).
		Where("plan_id = ? AND status = ? AND end_time > ?", plan.Id, "active", now).
		Order("user_id asc, end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	for i := range subs {
		if err := resetUserSubscriptionTx(tx, &subs[i], plan, now, advanceResetTime); err != nil {
			return nil, err
		}
	}
	return buildSubscriptionResetResult(plan, subs, advanceResetTime), nil
}

func AdminResetUserSubscriptionsByPlan(userId int, planId int, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if userId <= 0 || planId <= 0 {
		return nil, errors.New("invalid userId or planId")
	}
	var result *SubscriptionResetResult
	now := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		plan, err := getSubscriptionPlanByIdTx(tx, planId)
		if err != nil {
			return err
		}
		result, err = adminResetUserSubscriptionsByPlanTx(tx, userId, plan, now, advanceResetTime)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func AdminResetPlanSubscriptions(planId int, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if planId <= 0 {
		return nil, errors.New("invalid planId")
	}
	var result *SubscriptionResetResult
	now := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		plan, err := getSubscriptionPlanByIdTx(tx, planId)
		if err != nil {
			return err
		}
		result, err = adminResetPlanSubscriptionsTx(tx, plan, now, advanceResetTime)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type SubscriptionPreConsumeResult struct {
	UserSubscriptionId int
	PreConsumed        int64
	AmountTotal        int64
	AmountUsedBefore   int64
	AmountUsedAfter    int64
}

// ExpireDueSubscriptions marks expired subscriptions and handles group downgrade.
func ExpireDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("status = ? AND end_time > 0 AND end_time <= ?", "active", now).
		Order("end_time asc, id asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	expiredCount := 0
	userIds := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if sub.UserId > 0 {
			userIds[sub.UserId] = struct{}{}
		}
	}
	for userId := range userIds {
		cacheGroup := ""
		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&UserSubscription{}).
				Where("user_id = ? AND status = ? AND end_time > 0 AND end_time <= ?", userId, "active", now).
				Updates(map[string]interface{}{
					"status":     "expired",
					"updated_at": common.GetTimestamp(),
				})
			if res.Error != nil {
				return res.Error
			}
			expiredCount += int(res.RowsAffected)

			// If there's an active upgraded subscription, keep current group.
			var activeSub UserSubscription
			activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND upgrade_group <> ''",
				userId, "active", now).
				Order("end_time desc, id desc").
				Limit(1).
				Find(&activeSub)
			if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
				return nil
			}

			// No active upgraded subscription remains. Determine how to revert the user's
			// group: an explicit downgrade target takes precedence; otherwise revert to the
			// original baseline recovered from the whole chain (renewals/stacked subscriptions
			// store an empty prev_user_group, so trusting a single row would silently skip the
			// revert and strand the user in the upgraded group). Locate the most recently
			// expired subscription that defines a group transition.
			var lastExpired UserSubscription
			expiredQuery := tx.Where("user_id = ? AND status = ? AND (downgrade_group <> '' OR upgrade_group <> '')",
				userId, "expired").
				Order("end_time desc, id desc").
				Limit(1).
				Find(&lastExpired)
			if expiredQuery.Error != nil || expiredQuery.RowsAffected == 0 {
				return nil
			}
			currentGroup, err := getUserGroupByIdTx(tx, userId)
			if err != nil {
				return err
			}
			currentGroup = strings.TrimSpace(currentGroup)
			if currentGroup == "" {
				return nil
			}
			// An explicit downgrade group takes precedence; otherwise revert to the
			// pre-purchase baseline recovered from the whole chain.
			target := strings.TrimSpace(lastExpired.DowngradeGroup)
			if target == "" {
				// Only revert when the user's current group was actually granted by an
				// expired subscription, so we never touch a manually-assigned group.
				var grantedByExpired UserSubscription
				grantQuery := tx.Where("user_id = ? AND status = ? AND upgrade_group = ?",
					userId, "expired", currentGroup).
					Limit(1).
					Find(&grantedByExpired)
				if grantQuery.Error != nil {
					return grantQuery.Error
				}
				if grantQuery.RowsAffected == 0 {
					return nil
				}
				prevGroup, err := resolveOriginalUserGroupTx(tx, userId, currentGroup)
				if err != nil {
					return err
				}
				if prevGroup == "" || prevGroup == currentGroup {
					// Baseline unrecoverable (every row recorded an empty prev, e.g. the
					// user was placed into this group manually before subscribing). Leave
					// the group as-is for admin remediation rather than guessing.
					return nil
				}
				target = prevGroup
			}
			if target == "" || target == currentGroup {
				return nil
			}
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", target).Error; err != nil {
				return err
			}
			cacheGroup = target
			return nil
		})
		if err != nil {
			return expiredCount, err
		}
		if cacheGroup != "" {
			refreshSubscriptionUserGroupCache(userId, "subscription expiration")
		}
	}
	return expiredCount, nil
}

// SubscriptionPreConsumeRecord stores idempotent pre-consume operations per request.
type SubscriptionPreConsumeRecord struct {
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `json:"user_id" gorm:"index"`
	UserSubscriptionId int    `json:"user_subscription_id" gorm:"index"`
	PreConsumed        int64  `json:"pre_consumed" gorm:"type:bigint;not null;default:0"`
	Status             string `json:"status" gorm:"type:varchar(32);index"` // consumed/refunded
	CreatedAt          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
}

func (r *SubscriptionPreConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	r.CreatedAt = now
	r.UpdatedAt = now
	return nil
}

func (r *SubscriptionPreConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

func maybeResetUserSubscriptionWithPlanTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	if sub.NextResetTime > 0 && sub.NextResetTime > now {
		return nil
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return nil
	}
	baseUnix := sub.LastResetTime
	if baseUnix <= 0 {
		baseUnix = sub.StartTime
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, sub.EndTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, sub.EndTime)
	}
	if !advanced {
		if sub.NextResetTime == 0 && next > 0 {
			sub.NextResetTime = next
			sub.LastResetTime = base.Unix()
			return tx.Save(sub).Error
		}
		return nil
	}
	sub.AmountUsed = 0
	sub.LastResetTime = base.Unix()
	sub.NextResetTime = next
	return tx.Save(sub).Error
}

// PreConsumeUserSubscription pre-consumes from any active subscription total quota.
func PreConsumeUserSubscription(requestId string, userId int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if strings.TrimSpace(requestId) == "" {
		return nil, errors.New("requestId is empty")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be > 0")
	}
	now := GetDBTimestamp()

	returnValue := &SubscriptionPreConsumeResult{}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing SubscriptionPreConsumeRecord
		query := tx.Where("request_id = ?", requestId).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.Status == "refunded" {
				return errors.New("subscription pre-consume already refunded")
			}
			var sub UserSubscription
			if err := tx.Where("id = ?", existing.UserSubscriptionId).First(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = existing.PreConsumed
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = sub.AmountUsed
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}

		var subs []UserSubscription
		if err := lockForUpdate(tx).
			Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
			Order("end_time asc, id asc").
			Find(&subs).Error; err != nil {
			return errors.New("no active subscription")
		}
		if len(subs) == 0 {
			return errors.New("no active subscription")
		}
		for _, candidate := range subs {
			sub := candidate
			plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
			if err != nil {
				return err
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
				return err
			}
			usedBefore := sub.AmountUsed
			if sub.AmountTotal > 0 {
				remain := sub.AmountTotal - usedBefore
				if remain < amount {
					continue
				}
			}
			record := &SubscriptionPreConsumeRecord{
				RequestId:          requestId,
				UserId:             userId,
				UserSubscriptionId: sub.Id,
				PreConsumed:        amount,
				Status:             "consumed",
			}
			if err := tx.Create(record).Error; err != nil {
				var dup SubscriptionPreConsumeRecord
				if err2 := tx.Where("request_id = ?", requestId).First(&dup).Error; err2 == nil {
					if dup.Status == "refunded" {
						return errors.New("subscription pre-consume already refunded")
					}
					returnValue.UserSubscriptionId = sub.Id
					returnValue.PreConsumed = dup.PreConsumed
					returnValue.AmountTotal = sub.AmountTotal
					returnValue.AmountUsedBefore = sub.AmountUsed
					returnValue.AmountUsedAfter = sub.AmountUsed
					return nil
				}
				return err
			}
			sub.AmountUsed += amount
			if err := tx.Save(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = amount
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = usedBefore
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}
		return fmt.Errorf("subscription quota insufficient, need=%d", amount)
	})
	if err != nil {
		return nil, err
	}
	return returnValue, nil
}

// RefundSubscriptionPreConsume is idempotent and refunds pre-consumed subscription quota by requestId.
func RefundSubscriptionPreConsume(requestId string) error {
	if strings.TrimSpace(requestId) == "" {
		return errors.New("requestId is empty")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := lockForUpdate(tx).
			Where("request_id = ?", requestId).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" {
			return nil
		}
		if record.PreConsumed <= 0 {
			record.Status = "refunded"
			return tx.Save(&record).Error
		}
		if err := PostConsumeUserSubscriptionDelta(record.UserSubscriptionId, -record.PreConsumed); err != nil {
			return err
		}
		record.Status = "refunded"
		return tx.Save(&record).Error
	})
}

// ResetDueSubscriptions resets subscriptions whose next_reset_time has passed.
func ResetDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("next_reset_time > 0 AND next_reset_time <= ? AND status = ?", now, "active").
		Order("next_reset_time asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	resetCount := 0
	for _, sub := range subs {
		subCopy := sub
		plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
		if err != nil || plan == nil {
			continue
		}
		err = DB.Transaction(func(tx *gorm.DB) error {
			var locked UserSubscription
			if err := lockForUpdate(tx).
				Where("id = ? AND next_reset_time > 0 AND next_reset_time <= ?", subCopy.Id, now).
				First(&locked).Error; err != nil {
				return nil
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &locked, plan, now); err != nil {
				return err
			}
			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
	}
	return resetCount, nil
}

// CleanupSubscriptionPreConsumeRecords removes old idempotency records to keep table small.
func CleanupSubscriptionPreConsumeRecords(olderThanSeconds int64) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 7 * 24 * 3600
	}
	cutoff := GetDBTimestamp() - olderThanSeconds
	res := DB.Where("updated_at < ?", cutoff).Delete(&SubscriptionPreConsumeRecord{})
	return res.RowsAffected, res.Error
}

type SubscriptionPlanInfo struct {
	PlanId    int
	PlanTitle string
}

func GetSubscriptionPlanInfoByUserSubscriptionId(userSubscriptionId int) (*SubscriptionPlanInfo, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	cacheKey := fmt.Sprintf("sub:%d", userSubscriptionId)
	if cached, found, err := getSubscriptionPlanInfoCache().Get(cacheKey); err == nil && found {
		return &cached, nil
	}
	var sub UserSubscription
	if err := DB.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
	if err != nil {
		return nil, err
	}
	info := &SubscriptionPlanInfo{
		PlanId:    sub.PlanId,
		PlanTitle: plan.Title,
	}
	_ = getSubscriptionPlanInfoCache().SetWithTTL(cacheKey, *info, subscriptionPlanInfoCacheTTL())
	return info, nil
}

// Update subscription used amount by delta (positive consume more, negative refund).
func PostConsumeUserSubscriptionDelta(userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).
			Where("id = ?", userSubscriptionId).
			First(&sub).Error; err != nil {
			return err
		}
		newUsed := sub.AmountUsed + delta
		if newUsed < 0 {
			newUsed = 0
		}
		if sub.AmountTotal > 0 && newUsed > sub.AmountTotal {
			return fmt.Errorf("subscription used exceeds total, used=%d total=%d", newUsed, sub.AmountTotal)
		}
		// Update only amount_used: a full-row Save would write back the whole
		// locked-read snapshot and clobber concurrent writers of other columns.
		return tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).
			Update("amount_used", newUsed).Error
	})
}
