package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/price"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

var stripeAdaptor = &StripeAdaptor{}

func newStripeCheckoutBackend(httpClient *http.Client, apiURL string) stripe.Backend {
	config := &stripe.BackendConfig{
		LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull},
	}
	if httpClient != nil {
		config.HTTPClient = httpClient
	}
	if apiURL != "" {
		config.URL = stripe.String(apiURL)
	}
	return stripe.GetBackendWithConfig(stripe.APIBackend, config)
}

var stripeCheckoutBackend = newStripeCheckoutBackend(nil, "")

// StripePayRequest represents a payment request for Stripe checkout.
type StripePayRequest struct {
	// Amount is the quantity of units to purchase.
	Amount int64 `json:"amount"`
	// PaymentMethod specifies the payment method (e.g., "stripe").
	PaymentMethod string `json:"payment_method"`
	// SuccessURL is the optional custom URL to redirect after successful payment.
	// If empty, defaults to the server's console log page.
	SuccessURL string `json:"success_url,omitempty"`
	// CancelURL is the optional custom URL to redirect when payment is canceled.
	// If empty, defaults to the server's console topup page.
	CancelURL string `json:"cancel_url,omitempty"`
}

type StripeAdaptor struct {
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := prepareStripeTopUp(req.Amount, group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if rejectInvalidCreditedQuota(c, id, quote.quota) {
		return
	}
	if err := quote.verifyPrice(c.Request.Context()); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": quote.payMoney})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	// Gate the request path on the same condition the inbound webhook enforces
	// (isStripeWebhookEnabled == isStripeTopUpEnabled). Without this, a user
	// could be sent to a real Stripe Checkout and pay while payment compliance
	// is unconfirmed; the completion webhook would then be rejected and the
	// order would never be credited.
	if !isStripeTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe 支付未启用"})
		return
	}
	if req.PaymentMethod != model.PaymentMethodStripe {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付成功重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	if req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付取消重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}
	quote, err := prepareStripeTopUp(req.Amount, user.Group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if rejectInvalidCreditedQuota(c, id, quote.quota) {
		return
	}
	if err := quote.verifyPrice(c.Request.Context()); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	payLink, err := genStripeLink(c, referenceId, user.StripeCustomer, user.Email, quote, req.SuccessURL, req.CancelURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 创建 Checkout Session 失败 user_id=%d trade_no=%s amount=%d reason=sdk_error", id, referenceId, req.Amount))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	topUp := &model.TopUp{
		SiteId:          middleware.GetRequestSiteId(c),
		UserId:          id,
		Amount:          quote.units,
		Money:           float64(quote.units),
		TradeNo:         referenceId,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, referenceId, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Stripe 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%s", id, referenceId, quote.units, quote.payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	if !isStripeWebhookEnabled() {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe webhook 读取请求体失败 reason=body_read_failed path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	// Log a minimal receipt only. The raw body carries customer PII (email, name,
	// billing address) and the signature is verbose auth material; neither belongs
	// in INFO logs that fire on every legitimate transaction. Correlation fields
	// (event_type, trade_no) are logged after verification below.
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 收到请求 path=%q client_ip=%s body_bytes=%d", c.Request.URL.Path, c.ClientIP(), len(payload)))
	event, err := webhook.ConstructEventWithOptions(payload, signature, setting.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 验签失败 reason=invalid_signature path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	callerIp := c.ClientIP()
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 验签成功 event_type=%q client_ip=%s path=%q", string(event.Type), callerIp, c.Request.URL.Path))
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		sessionCompleted(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionExpired:
		sessionExpired(ctx, event)
	case stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		sessionAsyncPaymentSucceeded(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		sessionAsyncPaymentFailed(ctx, event, callerIp)
	case stripe.EventTypeChargeRefunded:
		chargeRefunded(ctx, event, callerIp)
	case stripe.EventTypeChargeDisputeCreated:
		chargeDisputeCreated(ctx, event, callerIp)
	default:
		logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 忽略事件 event_type=%q client_ip=%s", string(event.Type), callerIp))
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(ctx context.Context, event stripe.Event, callerIp string) {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "complete" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.completed 状态异常，忽略处理 trade_no=%q status=%q client_ip=%s", referenceId, status, callerIp))
		return
	}

	paymentStatus := event.GetObjectValue("payment_status")
	if paymentStatus != "paid" {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe Checkout 支付未完成，等待异步结果 trade_no=%q payment_status=%q client_ip=%s", referenceId, paymentStatus, callerIp))
		return
	}

	fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentSucceeded handles delayed payment methods (bank transfer, SEPA, etc.)
// that confirm payment after the checkout session completes.
func sessionAsyncPaymentSucceeded(ctx context.Context, event stripe.Event, callerIp string) {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付成功 trade_no=%q client_ip=%s", referenceId, callerIp))

	fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentFailed marks orders as failed when delayed payment methods
// ultimately fail (e.g. bank transfer not received, SEPA rejected).
func sessionAsyncPaymentFailed(ctx context.Context, event stripe.Event, callerIp string) {
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败 trade_no=%q client_ip=%s", referenceId, callerIp))

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败事件缺少订单号 client_ip=%s", callerIp))
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败但本地订单不存在 trade_no=%q client_ip=%s", referenceId, callerIp))
		return
	}

	if topUp.PaymentProvider != model.PaymentProviderStripe {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败但订单支付网关不匹配 trade_no=%q payment_provider=%q client_ip=%s", referenceId, topUp.PaymentProvider, callerIp))
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付失败但订单状态非 pending，忽略处理 trade_no=%q status=%q client_ip=%s", referenceId, topUp.Status, callerIp))
		return
	}

	topUp.Status = common.TopUpStatusFailed
	if err := topUp.Update(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 标记充值订单失败状态失败 trade_no=%q client_ip=%s error=%q", referenceId, callerIp, err.Error()))
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已标记为失败 trade_no=%q client_ip=%s", referenceId, callerIp))
}

// fulfillOrder is the shared logic for crediting quota after payment is confirmed.
func fulfillOrder(ctx context.Context, event stripe.Event, referenceId string, customerId string, callerIp string) {
	if len(referenceId) == 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 完成订单时缺少订单号 client_ip=%s", callerIp))
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	payload := map[string]any{
		"customer":     customerId,
		"amount_total": event.GetObjectValue("amount_total"),
		"currency":     strings.ToUpper(event.GetObjectValue("currency")),
		"event_type":   string(event.Type),
	}
	if err := model.CompleteSubscriptionOrder(referenceId, common.GetJsonString(payload), model.PaymentProviderStripe, ""); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单处理成功 trade_no=%q event_type=%q client_ip=%s", referenceId, string(event.Type), callerIp))
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		logger.LogError(ctx, fmt.Sprintf("Stripe 订阅订单处理失败 trade_no=%q event_type=%q client_ip=%s error=%q", referenceId, string(event.Type), callerIp, err.Error()))
		return
	}

	// Capture the payment_intent so a later refund/dispute (keyed by payment_intent,
	// not trade_no) can be linked back to this order for quota clawback.
	paymentIntent := event.GetObjectValue("payment_intent")
	err := model.Recharge(referenceId, customerId, paymentIntent, callerIp)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 充值处理失败 trade_no=%q event_type=%q client_ip=%s error=%q", referenceId, string(event.Type), callerIp, err.Error()))
		return
	}

	total, _ := strconv.ParseFloat(event.GetObjectValue("amount_total"), 64)
	currency := strings.ToUpper(event.GetObjectValue("currency"))
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值成功 trade_no=%q amount_total=%.2f currency=%q event_type=%q client_ip=%s", referenceId, total/100, currency, string(event.Type), callerIp))
}

func sessionExpired(ctx context.Context, event stripe.Event) {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.expired 状态异常，忽略处理 trade_no=%q status=%q", referenceId, status))
		return
	}

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, "Stripe checkout.expired 缺少订单号")
		return
	}

	// Subscription order expiration
	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	if err := model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单已过期 trade_no=%q", referenceId))
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		logger.LogError(ctx, fmt.Sprintf("Stripe 订阅订单过期处理失败 trade_no=%q error=%q", referenceId, err.Error()))
		return
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusExpired)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单不存在，无法标记过期 trade_no=%q", referenceId))
		return
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 充值订单过期处理失败 trade_no=%q error=%q", referenceId, err.Error()))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已过期 trade_no=%q", referenceId))
}

// stripeMinorAmount reads a Stripe minor-unit amount field (e.g. amount,
// amount_refunded) from a webhook event. event.GetObjectValue renders the decoded
// JSON number with fmt %v on a float64, so values >= 1e6 come back in scientific
// notation (e.g. 1000000 -> "1e+06"); strconv.ParseInt fails on that and would
// silently yield 0. Parse as float (Stripe minor amounts are integral, so the
// int64 cast is exact) — mirrors how fulfillOrder reads amount_total.
func stripeMinorAmount(event stripe.Event, key string) int64 {
	f, _ := strconv.ParseFloat(event.GetObjectValue(key), 64)
	return int64(f)
}

// chargeRefunded reverses (part of) a credited Stripe top-up when Stripe reports a
// refund. The charge object carries the cumulative amount_refunded, so clawback is
// proportional to the refunded fraction and idempotent across partial/duplicate
// refunds (model.ReverseStripeTopUp tracks how much was already reversed). Refunds
// initiated by the operator in the Stripe dashboard fire this event too, and are
// intentionally clawed back as well.
func chargeRefunded(ctx context.Context, event stripe.Event, callerIp string) {
	paymentIntent := event.GetObjectValue("payment_intent")
	if paymentIntent == "" {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 退款事件缺少 payment_intent client_ip=%s", callerIp))
		return
	}
	amountRefunded := stripeMinorAmount(event, "amount_refunded")
	amount := stripeMinorAmount(event, "amount")
	if amount <= 0 {
		// Without a positive charge total the refunded fraction is undefined; skip
		// rather than risk over-clawing. (Refund events always carry a charge amount.)
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 退款事件金额缺失或非法，忽略 payment_intent=%q amount=%d amount_refunded=%d client_ip=%s", paymentIntent, amount, amountRefunded, callerIp))
		return
	}

	err := model.ReverseStripeTopUp(paymentIntent, amountRefunded, amount, false, callerIp)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 退款事件无匹配充值订单，忽略 payment_intent=%q client_ip=%s", paymentIntent, callerIp))
		return
	}
	if errors.Is(err, model.ErrTopUpAmountInvalid) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 退款金额非法，未回扣 payment_intent=%q amount_refunded=%d amount=%d client_ip=%s", paymentIntent, amountRefunded, amount, callerIp))
		return
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 退款回扣失败 payment_intent=%q amount_refunded=%d amount=%d client_ip=%s error=%q", paymentIntent, amountRefunded, amount, callerIp, err.Error()))
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 退款回扣完成 payment_intent=%q amount_refunded=%d amount=%d client_ip=%s", paymentIntent, amountRefunded, amount, callerIp))
}

// chargeDisputeCreated reverses the full remaining credited quota of a Stripe
// top-up when a chargeback/dispute is opened, and flags the order (status=disputed)
// for admin review. Per the configured policy the account is NOT auto-disabled.
func chargeDisputeCreated(ctx context.Context, event stripe.Event, callerIp string) {
	paymentIntent := event.GetObjectValue("payment_intent")
	if paymentIntent == "" {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 拒付事件缺少 payment_intent client_ip=%s", callerIp))
		return
	}
	amount := stripeMinorAmount(event, "amount")
	reason := event.GetObjectValue("reason")

	err := model.ReverseStripeTopUp(paymentIntent, amount, amount, true, callerIp)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 拒付事件无匹配充值订单 payment_intent=%q reason=%q client_ip=%s", paymentIntent, reason, callerIp))
		return
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 拒付回扣失败 payment_intent=%q reason=%q client_ip=%s error=%q", paymentIntent, reason, callerIp, err.Error()))
		return
	}
	logger.LogWarn(ctx, fmt.Sprintf("Stripe 拒付已回扣额度并标记订单待管理员复核 payment_intent=%q reason=%q client_ip=%s", paymentIntent, reason, callerIp))
}

// genStripeLink generates a Stripe Checkout session URL for payment.
// It creates a new checkout session with the specified parameters and returns the payment URL.
//
// Parameters:
//   - referenceId: unique reference identifier for the transaction
//   - customerId: existing Stripe customer ID (empty string if new customer)
//   - email: customer email address for new customer creation
//   - quote: validated quantity, currency, and payment configuration snapshot
//   - successURL: custom URL to redirect after successful payment (empty for default)
//   - cancelURL: custom URL to redirect when payment is canceled (empty for default)
//
// Returns the checkout session URL or an error if the session creation fails.
func genStripeLink(c *gin.Context, referenceId string, customerId string, email string, quote *stripeTopUpQuote, successURL string, cancelURL string) (string, error) {
	// Use custom URLs if provided, otherwise use defaults
	if successURL == "" {
		successURL = paymentReturnPath(c, "/usage-logs")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath(c, "/wallet")
	}

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(successURL),
		CancelURL:         stripe.String(cancelURL),
		Currency:          stripe.String(quote.currency),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(quote.priceID),
				Quantity: stripe.Int64(quote.quantity),
			},
		},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(setting.StripePromotionCodesEnabled),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	params.Context = c.Request.Context()
	result, err := (session.Client{B: stripeCheckoutBackend, Key: quote.apiKey}).New(params)
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

// New Stripe orders store the actual recharge units in Amount and Money. Price
// adjustments affect the checkout quantity only; existing orders keep their
// recorded Money and therefore retain their original settlement/refund behavior.
type stripeTopUpQuote struct {
	units     int64
	quantity  int64
	quota     decimal.Decimal
	unitPrice decimal.Decimal
	payMoney  string
	currency  string
	priceID   string
	apiKey    string
}

func prepareStripeTopUp(amount int64, group string) (*stripeTopUpQuote, error) {
	if amount <= 0 || amount > common.MaxWalletQuota {
		return nil, errors.New("充值数量无效")
	}
	ratio := common.GetTopupGroupRatio(group)
	if ratio == 0 {
		ratio = 1
	}
	discount := 1.0
	if configured, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if math.IsNaN(configured) || math.IsInf(configured, 0) {
			return nil, errors.New("Stripe 充值价格配置无效")
		}
		if configured > 0 {
			discount = configured
		}
	}
	for _, value := range []float64{common.QuotaPerUnit, setting.StripeUnitPrice, ratio, discount} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("Stripe 充值价格配置无效")
		}
	}
	minimum := getStripeMinTopup()
	if amount < minimum {
		return nil, fmt.Errorf("充值数量不能小于 %d", minimum)
	}
	units := decimal.NewFromInt(amount)
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		if !units.Mod(quotaPerUnit).IsZero() {
			return nil, errors.New("Stripe 充值数量必须为完整计费单位")
		}
		units = units.Div(quotaPerUnit)
	}
	unitCount, err := common.WalletQuotaFromDecimalStrict(units)
	if err != nil || unitCount <= 0 {
		return nil, errors.New("充值数量无效")
	}
	quota := units.Mul(quotaPerUnit)
	if _, err := validateCreditedQuota(quota); err != nil {
		return nil, err
	}
	quantity := units.Mul(decimal.NewFromFloat(ratio)).Mul(decimal.NewFromFloat(discount))
	if !quantity.Equal(quantity.Truncate(0)) {
		return nil, errors.New("Stripe 当前价格无法精确应用充值倍率或折扣")
	}
	checkoutQuantity, err := common.WalletQuotaFromDecimalStrict(quantity)
	if err != nil || checkoutQuantity <= 0 {
		return nil, errors.New("Stripe 支付数量超出范围")
	}
	return &stripeTopUpQuote{
		units: int64(unitCount), quantity: int64(checkoutQuantity), quota: quota,
		unitPrice: decimal.NewFromFloat(setting.StripeUnitPrice),
		priceID:   strings.TrimSpace(setting.StripePriceId),
		apiKey:    strings.TrimSpace(setting.StripeApiSecret),
	}, nil
}

func getStripeMinTopup() int64 {
	return getMinimumTopUpAmount(setting.StripeMinTopUp)
}

func (quote *stripeTopUpQuote) verifyPrice(ctx context.Context) error {
	if !strings.HasPrefix(quote.apiKey, "sk_") && !strings.HasPrefix(quote.apiKey, "rk_") {
		return errors.New("无效的Stripe API密钥")
	}
	params := &stripe.PriceParams{}
	params.Context = ctx
	configuredPrice, err := (price.Client{B: stripeCheckoutBackend, Key: quote.apiKey}).Get(quote.priceID, params)
	if err != nil {
		return errors.New("无法验证 Stripe 充值价格")
	}
	if configuredPrice.ID != quote.priceID || !configuredPrice.Active ||
		configuredPrice.Type != stripe.PriceTypeOneTime || configuredPrice.BillingScheme != stripe.PriceBillingSchemePerUnit ||
		configuredPrice.TransformQuantity != nil || configuredPrice.CustomUnitAmount != nil || configuredPrice.Recurring != nil {
		return errors.New("Stripe 充值价格必须为有效的固定单次价格")
	}
	currency := string(configuredPrice.Currency)
	decimals, wholeAmount, err := stripeChargeCurrencyPrecision(currency)
	if err != nil {
		return err
	}
	// stripe-go decodes unit_amount_decimal through float64. Read the original
	// decimal string so a high precision price cannot pass verification by rounding.
	var rawPrice struct {
		UnitAmount        *int64 `json:"unit_amount"`
		UnitAmountDecimal string `json:"unit_amount_decimal"`
	}
	if configuredPrice.LastResponse == nil || common.Unmarshal(configuredPrice.LastResponse.RawJSON, &rawPrice) != nil {
		return errors.New("Stripe 充值价格响应无效")
	}
	var minorPrice decimal.Decimal
	if rawPrice.UnitAmountDecimal != "" {
		minorPrice, err = decimal.NewFromString(rawPrice.UnitAmountDecimal)
	} else if rawPrice.UnitAmount != nil {
		minorPrice = decimal.NewFromInt(*rawPrice.UnitAmount)
	} else {
		return errors.New("Stripe 充值价格响应无效")
	}
	scale := decimal.New(1, decimals)
	if err != nil || !minorPrice.IsPositive() || !quote.unitPrice.Mul(scale).Equal(minorPrice) {
		return errors.New("Stripe 配置单价与网关价格不一致")
	}
	totalMinor := minorPrice.Mul(decimal.NewFromInt(quote.quantity))
	if !totalMinor.Equal(totalMinor.Truncate(0)) || totalMinor.GreaterThan(decimal.NewFromInt(common.MaxWalletQuota)) ||
		(wholeAmount && !totalMinor.Mod(scale).IsZero()) {
		return errors.New("Stripe 支付金额无法精确表示")
	}
	quote.currency = currency
	quote.payMoney = totalMinor.Div(scale).StringFixed(decimals)
	return nil
}

// Stripe charge precision differs from ISO payout precision, especially for
// ISK/UGX and HUF/TWD. See https://docs.stripe.com/currencies#special-cases.
func stripeChargeCurrencyPrecision(currency string) (int32, bool, error) {
	if len(currency) != 3 || strings.IndexFunc(currency, func(r rune) bool { return r < 'a' || r > 'z' }) >= 0 {
		return 0, false, errors.New("Stripe 价格币种无效")
	}
	switch currency {
	case "bif", "clp", "djf", "gnf", "jpy", "kmf", "krw", "mga", "pyg", "rwf", "vnd", "vuv", "xaf", "xof", "xpf":
		return 0, false, nil
	case "isk", "ugx":
		return 2, true, nil
	case "bhd", "iqd", "jod", "kwd", "lyd", "omr", "tnd":
		// These ISO three-decimal currencies are not supported charge currencies
		// in the current Stripe integration; do not guess a two-decimal amount.
		return 0, false, errors.New("Stripe 价格币种暂不支持")
	default:
		return 2, false, nil
	}
}
