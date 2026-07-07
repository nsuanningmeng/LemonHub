package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()

	// 获取支付方式
	payMethods := operation_setting.PayMethods
	if !complianceConfirmed {
		payMethods = []map[string]string{}
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// Waffo Pancake displayed above the legacy Waffo gateway.
	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":      "Waffo Pancake",
				"type":      model.PaymentMethodWaffoPancake,
				"color":     "rgba(var(--semi-orange-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoPancakeMinTopUp),
			})
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoMinTopUp),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	data := gin.H{
		"enable_online_topup":              isEpayTopUpEnabled(),
		"enable_stripe_topup":              isStripeTopUpEnabled(),
		"enable_creem_topup":               isCreemTopUpEnabled(),
		"enable_waffo_topup":               enableWaffo,
		"enable_waffo_pancake_topup":       enableWaffoPancake,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products":          setting.CreemProducts,
		"pay_methods":             payMethods,
		"min_topup":               operation_setting.MinTopUp,
		"stripe_min_topup":        setting.StripeMinTopUp,
		"waffo_min_topup":         setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount,
		"topup_link":              common.TopUpLink,
	}

	// Sub-site online recharge auto-degradation: it is only offered when the agent has
	// configured their own 收款 (pay_config) AND has procurement-wallet balance. When the
	// wallet is drained (or unconfigured), the recharge entry disappears for this site —
	// without affecting already-issued quota or other gateways.
	if site := middleware.GetRequestSite(c); site != nil {
		_, payOk := parseSitePayConfig(site.PayConfig)
		bal, _ := model.GetSiteWalletBalance(site.Id)
		available := payOk && bal > 0
		data["enable_online_topup"] = available
		data["site_topup_available"] = available
	}

	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type AmountRequest struct {
	Amount int64 `json:"amount"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

// sitePayConfig is a sub-site's own epay merchant configuration, stored as JSON in
// Site.PayConfig. A sub-site collects payment into its OWN merchant account.
type sitePayConfig struct {
	EpayId     string   `json:"epay_id"`
	EpayKey    string   `json:"epay_key"`
	PayAddress string   `json:"pay_address"`
	PayMethods []string `json:"pay_methods"`
}

// parseSitePayConfig parses Site.PayConfig JSON; ok is true only when the epay triple is complete.
func parseSitePayConfig(s string) (sitePayConfig, bool) {
	var cfg sitePayConfig
	if strings.TrimSpace(s) == "" {
		return cfg, false
	}
	if err := common.UnmarshalJsonStr(s, &cfg); err != nil {
		return cfg, false
	}
	return cfg, cfg.EpayId != "" && cfg.EpayKey != "" && cfg.PayAddress != ""
}

// getEpayClientForSite returns the epay client for a request's site: the sub-site's own
// pay_config when present, otherwise the global (main-site) config. nil if incomplete.
func getEpayClientForSite(site *model.Site) *epay.Client {
	if site == nil {
		return GetEpayClient()
	}
	cfg, ok := parseSitePayConfig(site.PayConfig)
	if !ok {
		return nil
	}
	client, err := epay.NewClient(&epay.Config{PartnerID: cfg.EpayId, Key: cfg.EpayKey}, cfg.PayAddress)
	if err != nil {
		return nil
	}
	return client
}

// siteTopupCostMilli is the agent's procurement cost in 厘 for a recharge of `money` CNY at
// the sub-site discount rate: money × 1000(厘/元) × discountRate / 10000.
func siteTopupCostMilli(money float64, discountRate int) int64 {
	if money <= 0 || discountRate <= 0 {
		return 0
	}
	cost := decimal.NewFromFloat(money).
		Mul(decimal.NewFromInt(1000)).
		Mul(decimal.NewFromInt(int64(discountRate))).
		Div(decimal.NewFromInt(int64(model.DiscountRateBase)))
	return cost.Round(0).IntPart()
}

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

// getMaxTopup is the per-order ceiling on the requested top-up amount, in the
// same display unit the user enters (mirrors getMinTopup). The credited quota
// is amount(USD) * QuotaPerUnit and must fit the int32 quota column, so orders
// above this could never be credited in full and are rejected up front.
func getMaxTopup() int64 {
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return int64(common.MaxQuota)
	}
	if common.QuotaPerUnit <= 1 {
		return int64(common.MaxQuota)
	}
	return int64(float64(common.MaxQuota) / common.QuotaPerUnit)
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	if req.Amount > getMaxTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("单笔充值数量不能大于 %d", getMaxTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	// Sub-site pre-order check: the agent must have enough procurement-wallet balance to
	// cover this recharge's wholesale cost (面值 × discount_rate). If not, reject up front
	// so the user never pays into an order the platform can't settle (auto-degradation).
	site := middleware.GetRequestSite(c)
	if site != nil {
		cost := siteTopupCostMilli(payMoney, site.DiscountRate)
		bal, balErr := model.GetSiteWalletBalance(site.Id)
		if balErr != nil || bal < cost {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "本站充值暂时不可用"})
			return
		}
	}

	// Server-to-server epay notify must hit a STABLE, gateway-registered, reachable endpoint.
	// For the MAIN site, always use the configured callback address: sending the async notify to
	// whatever trusted domain the user happened to visit (a multi-domain change) can target a
	// frontend-only / unregistered / unreachable domain and strand a paid order as unpaid. A
	// SUB-site collects into its OWN epay merchant, registered against its OWN domain, so it must
	// keep the per-request host. The return URL (browser redirect) stays per-domain in both cases;
	// it targets the EpayReturn settlement fallback, which lands the user on the wallet page.
	notifyBase := strings.TrimRight(service.GetCallbackAddress(), "/")
	if site != nil {
		notifyBase = service.GetCallbackAddressForRequest(c)
	}
	// The return URL is a browser redirect, so it must land on the domain the user is
	// actually browsing — GetRequestBaseURL (the trusted request host, else ServerAddress).
	// NOT GetCallbackAddressForRequest, which can resolve to a gateway-only CustomCallbackAddress
	// the browser cannot reach / has no session on. The endpoint exists on every served domain.
	returnUrl, _ := url.Parse(strings.TrimRight(service.GetRequestBaseURL(c), "/") + "/api/user/epay/return")
	notifyUrl, _ := url.Parse(notifyBase + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := getEpayClientForSite(site)
	if client == nil {
		msg := "当前管理员未配置支付信息"
		if site != nil {
			msg = "本站未配置收款信息"
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": msg})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		SiteId:          middleware.GetRequestSiteId(c),
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%d money=%.2f site_id=%d notify_url=%q request_host=%q trusted_host=%v uri=%q params=%q", id, tradeNo, req.PaymentMethod, req.Amount, payMoney, middleware.GetRequestSiteId(c), notifyUrl.String(), c.Request.Host, service.IsRequestHostTrusted(c), uri, common.GetJsonString(params)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

// parseEpayCallbackParams extracts the epay callback parameters from a GET query or an
// application/x-www-form-urlencoded POST body (gateways deliver both forms). A POST
// whose body cannot be parsed yields an empty map — callers treat that as a bad callback.
func parseEpayCallbackParams(c *gin.Context) map[string]string {
	if c.Request.Method == http.MethodPost {
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 回调 POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			return map[string]string{}
		}
		return lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	}
	return lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
		r[t] = c.Request.URL.Query().Get(t)
		return r
	}, map[string]string{})
}

// epayCallbackVerification is a located and signature-verified epay top-up callback:
// the order it names, the owning sub-site (nil for a main-site order) and the
// gateway-verified payload.
type epayCallbackVerification struct {
	topUp *model.TopUp
	site  *model.Site
	info  *epay.VerifyRes
}

// verifyEpayTopUpCallback locates the top-up order named by an epay callback (async
// notify or browser return) and verifies the MD5 signature with the owning site's OWN
// merchant key, then cross-checks the callback amount against the order so a payload
// signed for a different amount can never settle. Returns nil (after logging) when the
// callback must be rejected. source tags log lines: "notify" or "return".
func verifyEpayTopUpCallback(c *gin.Context, params map[string]string, source string) *epayCallbackVerification {
	// Locate the order BEFORE verifying, so a sub-site order is verified with its OWN
	// pay_config key (the signature was produced with the sub-site's key, not the global
	// one). The order — not the request Host — is authoritative for which site owns it.
	tradeNo := params["out_trade_no"]
	if tradeNo == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调缺少订单号 source=%s path=%q client_ip=%s", source, c.Request.RequestURI, c.ClientIP()))
		return nil
	}
	topUp, err := model.FindTopUpByTradeNo(tradeNo)
	if err != nil {
		// Transient DB error — NOT "order missing". Rejecting lets the gateway retry.
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 回调订单查询失败 source=%s trade_no=%s client_ip=%s error=%q", source, tradeNo, c.ClientIP(), err.Error()))
		return nil
	}
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调订单不存在 source=%s trade_no=%s client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	if topUp.PaymentProvider != model.PaymentProviderEpay {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 source=%s trade_no=%s order_provider=%s client_ip=%s", source, tradeNo, topUp.PaymentProvider, c.ClientIP()))
		return nil
	}

	// Load the owning sub-site from the DB (not the cache) so settlement always has the
	// authoritative pay_config + discount; a cache miss must not lead to a free credit.
	var site *model.Site
	if topUp.SiteId > 0 {
		site, _ = model.GetSiteById(topUp.SiteId)
		if site == nil {
			// Sub-site order but its site can't be loaded: fail (retry) rather than fall back
			// to the global client / a zero cost, which would credit the user for free.
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 子站订单无法加载子站 source=%s trade_no=%s site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
			return nil
		}
	}
	client := getEpayClientForSite(site)
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 source=%s trade_no=%s site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
		return nil
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调验签失败 source=%s trade_no=%s client_ip=%s verify_error=%q", source, tradeNo, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调验签失败 source=%s trade_no=%s client_ip=%s verify_status=false", source, tradeNo, c.ClientIP()))
		}
		return nil
	}
	if !epayCallbackMoneyMatchesOrder(verifyInfo.Money, topUp.Money) {
		// Defense in depth against a compromised/malicious gateway reporting success
		// for a different amount than the order was created for.
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 回调金额与订单不符 source=%s trade_no=%s callback_money=%q order_money=%.2f client_ip=%s", source, tradeNo, verifyInfo.Money, topUp.Money, c.ClientIP()))
		return nil
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 回调验签成功 source=%s trade_no=%s callback_type=%s trade_status=%s client_ip=%s", source, verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP()))
	return &epayCallbackVerification{topUp: topUp, site: site, info: verifyInfo}
}

// epayCallbackMoneyMatchesOrder compares a gateway-reported amount with the order's
// amount at 2 decimal places (the precision sent at purchase). An absent amount is
// tolerated — the MD5 signature already covers whatever the gateway did send, and some
// forks omit money on some channels; failing those would strand genuinely paid orders.
//
// The order-side comparand is derived with strconv.FormatFloat(...'f',2,64), the EXACT
// formatting RequestEpay used to send Money to the gateway. This must not be replaced
// with decimal.NewFromFloat(orderMoney).Round(2): FormatFloat rounds the binary float64
// half-to-even while decimal.Round rounds half-away-from-zero, so for x.xx5 amounts (e.g.
// 1.005 from amount×price×ratio) the two disagree and a gateway that honestly echoes the
// submitted amount would be rejected — permanently stranding a paid order.
func epayCallbackMoneyMatchesOrder(callbackMoney string, orderMoney float64) bool {
	callbackMoney = strings.TrimSpace(callbackMoney)
	if callbackMoney == "" {
		return true
	}
	cb, err := decimal.NewFromString(callbackMoney)
	if err != nil {
		return false
	}
	orderSide, err := decimal.NewFromString(strconv.FormatFloat(orderMoney, 'f', 2, 64))
	if err != nil {
		return false
	}
	return cb.Round(2).Equal(orderSide)
}

// settleEpayTopUp finishes a gateway-confirmed PAID top-up order exactly once and runs
// the post-settlement bookkeeping shared by the async notify, the browser return and
// the reconciliation sweep. Racing deliveries are safe: the order-level lock plus the
// DB-level CAS inside CompleteEpayTopUp make duplicate settlement an idempotent no-op.
func settleEpayTopUp(ctx context.Context, topUp *model.TopUp, site *model.Site, callerIP string, source string) (string, error) {
	LockOrder(topUp.TradeNo)
	defer UnlockOrder(topUp.TradeNo)

	// Settle: for a sub-site, debit the agent wallet (面值 × discount_rate) atomically with
	// crediting the user; idempotent across duplicate callbacks; insufficient wallet parks
	// the order for manual review (user is NOT credited until an admin resolves it).
	var cost int64
	if site != nil {
		cost = siteTopupCostMilli(topUp.Money, site.DiscountRate)
	}
	finalStatus, quotaAdded, settleErr := model.CompleteEpayTopUp(topUp.TradeNo, cost, 0)
	if settleErr != nil {
		// Transient settlement error: reject so the gateway retries / the reconciler
		// picks the order up again (settlement is idempotent).
		logger.LogError(ctx, fmt.Sprintf("易支付 结算失败 source=%s trade_no=%s user_id=%d caller_ip=%s error=%q", source, topUp.TradeNo, topUp.UserId, callerIP, settleErr.Error()))
		return "", settleErr
	}
	switch finalStatus {
	case model.TopUpStatusManualReview:
		logger.LogError(ctx, fmt.Sprintf("易支付 子站钱包不足，订单转人工处理 source=%s trade_no=%s site_id=%d user_id=%d cost=%d caller_ip=%s", source, topUp.TradeNo, topUp.SiteId, topUp.UserId, cost, callerIP))
		model.RecordLog(topUp.UserId, model.LogTypeSystem, fmt.Sprintf("在线充值已支付但子站钱包不足，订单 %s 转人工处理", topUp.TradeNo))
	case common.TopUpStatusSuccess:
		if quotaAdded > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("易支付 充值成功 source=%s trade_no=%s user_id=%d caller_ip=%s quota_to_add=%d money=%.2f", source, topUp.TradeNo, topUp.UserId, callerIP, quotaAdded, topUp.Money))
			model.RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaAdded), topUp.Money), callerIP, topUp.PaymentMethod, "epay")
			if serr := model.SettleReferralOnTopUp(topUp.UserId, topUp.TradeNo, int64(quotaAdded), "epay"); serr != nil {
				logger.LogError(ctx, fmt.Sprintf("邀请返佣结算失败 trade_no=%s user_id=%d error=%q", topUp.TradeNo, topUp.UserId, serr.Error()))
			}
		}
	}
	return finalStatus, nil
}

func EpayNotify(c *gin.Context) {
	// NOTE: the global epay enablement is NOT checked here — that would wrongly reject a
	// sub-site's callback when only sub-sites (not the main site) have epay configured. The
	// per-order client resolution (getEpayClientForSite) gates each callback by the owning
	// site's own config instead.
	params := parseEpayCallbackParams(c)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))
	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	v := verifyEpayTopUpCallback(c, params, "notify")
	if v == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if v.info.TradeStatus != epay.StatusTradeSuccess {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s trade_status=%s client_ip=%s", v.topUp.TradeNo, v.info.TradeStatus, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("success"))
		return
	}
	if _, err := settleEpayTopUp(c.Request.Context(), v.topUp, v.site, c.ClientIP(), "notify"); err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	_, _ = c.Writer.Write([]byte("success"))
}

// EpayReturn handles the browser return after an epay top-up payment. The gateway
// appends the SAME MD5-signed parameter set it sends to the async notify, so this
// endpoint doubles as a settlement fallback for lost notifies (several popular epay
// implementations deliver the notify only once, or give up after a few retries).
// Verification and amount checks are identical to the notify path, so this grants
// nothing a forged notify couldn't already attempt; settlement is idempotent, so
// racing the async notify is safe. The user always ends up on the wallet page.
func EpayReturn(c *gin.Context) {
	params := parseEpayCallbackParams(c)
	if len(params) == 0 {
		// Bare visit without a signed payload: just land the user on the wallet page.
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/console/topup"))
		return
	}
	v := verifyEpayTopUpCallback(c, params, "return")
	if v == nil {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/console/topup?pay=fail"))
		return
	}
	if v.info.TradeStatus != epay.StatusTradeSuccess {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/console/topup?pay=pending"))
		return
	}
	finalStatus, err := settleEpayTopUp(c.Request.Context(), v.topUp, v.site, c.ClientIP(), "return")
	if err != nil {
		// Paid, but settlement hit a transient error — the notify retry or the
		// reconciliation sweep will finish it; show the user "processing".
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/console/topup?pay=pending"))
		return
	}
	if finalStatus == common.TopUpStatusSuccess {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/console/topup?pay=success"))
		return
	}
	// manual_review (paid, awaiting admin) or any other parked state.
	c.Redirect(http.StatusFound, paymentReturnPath(c, "/console/topup?pay=pending"))
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	if req.Amount > getMaxTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("单笔充值数量不能大于 %d", getMaxTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	siteScope := middleware.EffectiveSiteScope(c)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo, siteScope)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo, siteScope)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	// NOTE: admin manual 补单 is intentionally NOT a referral-qualifying top-up — only real
	// online payments settle referral rewards (see SettleReferralOnTopUp call sites).
	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

type RetryTopUpRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminRetryManualReviewTopUp re-settles a parked (manual_review) sub-site recharge order
// after the agent has funded their procurement wallet: it re-runs the atomic settlement
// (debit agent wallet + credit user) using the order's owning site discount. Main admin only.
func AdminRetryManualReviewTopUp(c *gin.Context) {
	var req RetryTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.TradeNo) == "" {
		common.ApiErrorMsg(c, "未提供订单号")
		return
	}
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}
	var cost int64
	if topUp.SiteId > 0 {
		site, err := model.GetSiteById(topUp.SiteId)
		if err != nil || site == nil {
			common.ApiErrorMsg(c, "订单所属子站不存在")
			return
		}
		cost = siteTopupCostMilli(topUp.Money, site.DiscountRate)
		if cost <= 0 {
			common.ApiErrorMsg(c, "无法计算子站成本，请检查支付金额与折扣率")
			return
		}
	}
	finalStatus, quotaAdded, err := model.RetryManualReviewTopUp(req.TradeNo, cost, c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "topup.retry_settlement", map[string]interface{}{"trade_no": req.TradeNo, "status": finalStatus})
	// A parked order is a real online (epay) payment that was only delayed by an insufficient
	// agent wallet, so settling it here is consistent with the webhook paths. Idempotent.
	if finalStatus == common.TopUpStatusSuccess && quotaAdded > 0 {
		if serr := model.SettleReferralOnTopUp(topUp.UserId, req.TradeNo, int64(quotaAdded), "epay"); serr != nil {
			common.SysError("referral settlement failed (epay manual retry): " + serr.Error())
		}
	}
	common.ApiSuccess(c, gin.H{"status": finalStatus, "quota_added": quotaAdded})
}
