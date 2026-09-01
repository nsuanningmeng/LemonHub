package controller

import (
	"context"
	"errors"
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
	site := middleware.GetRequestSite(c)

	// 获取支付方式
	payMethods := epayPayMethodsForSite(site)
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
				"color":     "#635BFF",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// Waffo Pancake is displayed above the standard Waffo gateway.
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
				"color":     "#F97316",
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
				"color":     "#3B82F6",
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
	if site != nil {
		_, payOk := parseSitePayConfig(site.PayConfig)
		bal, _ := model.GetSiteWalletBalance(site.Id)
		available := payOk && len(epayPayMethodsForSite(site)) > 0 && bal > 0
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
	cfg, ok := epayMerchantConfigForSite(nil)
	if !ok {
		return nil
	}
	if err := service.ValidateStrictSSRFProtectedFetchURL(cfg.Address); err != nil {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: cfg.PartnerID,
		Key:       cfg.Key,
	}, cfg.Address)
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

type epayMerchantConfig struct {
	PartnerID string
	Key       string
	Address   string
}

// parseSitePayConfig parses Site.PayConfig JSON; ok is true only when the epay
// credential triple is complete and its gateway is a secure HTTPS origin.
func parseSitePayConfig(s string) (sitePayConfig, bool) {
	var cfg sitePayConfig
	if strings.TrimSpace(s) == "" {
		return cfg, false
	}
	if err := common.UnmarshalJsonStr(s, &cfg); err != nil {
		return cfg, false
	}
	cfg.EpayId = strings.TrimSpace(cfg.EpayId)
	cfg.EpayKey = strings.TrimSpace(cfg.EpayKey)
	cfg.PayAddress = strings.TrimSpace(cfg.PayAddress)
	normalizedMethods := make([]string, 0, len(cfg.PayMethods))
	seenMethods := make(map[string]struct{}, len(cfg.PayMethods))
	for _, method := range cfg.PayMethods {
		method = strings.TrimSpace(method)
		if method == "" {
			continue
		}
		if _, exists := seenMethods[method]; exists {
			continue
		}
		seenMethods[method] = struct{}{}
		normalizedMethods = append(normalizedMethods, method)
	}
	cfg.PayMethods = normalizedMethods
	if cfg.EpayId == "" || cfg.EpayKey == "" || cfg.PayAddress == "" {
		return cfg, false
	}
	if _, err := validateEpayGatewayAddress(cfg.PayAddress); err != nil {
		return cfg, false
	}
	return cfg, true
}

// epayPayMethodsForSite returns fresh maps so appending provider methods cannot
// mutate the global payment setting's backing slice. A sub-site's allowlist belongs
// to its own merchant and never inherits Epay methods from the main-site merchant.
func epayPayMethodsForSite(site *model.Site) []map[string]string {
	configured := operation_setting.PayMethods
	if site == nil {
		methods := make([]map[string]string, 0, len(configured))
		for _, method := range configured {
			clone := make(map[string]string, len(method))
			for key, value := range method {
				clone[key] = value
			}
			methods = append(methods, clone)
		}
		return methods
	}

	siteConfig, ok := parseSitePayConfig(site.PayConfig)
	if !ok {
		return []map[string]string{}
	}
	methods := make([]map[string]string, 0, len(siteConfig.PayMethods))
	for _, allowedType := range siteConfig.PayMethods {
		method := map[string]string{
			"name": allowedType,
			"icon": "LuCreditCard",
			"type": allowedType,
		}
		for _, globalMethod := range configured {
			if globalMethod["type"] != allowedType {
				continue
			}
			method = make(map[string]string, len(globalMethod))
			for key, value := range globalMethod {
				method[key] = value
			}
			break
		}
		methods = append(methods, method)
	}
	return methods
}

// epayMerchantConfigForSite resolves payment credentials from the order owner, never
// from callback Host headers. A nil site means the global/main-site merchant.
func epayMerchantConfigForSite(site *model.Site) (epayMerchantConfig, bool) {
	if site == nil {
		cfg := epayMerchantConfig{
			PartnerID: strings.TrimSpace(operation_setting.EpayId),
			Key:       strings.TrimSpace(operation_setting.EpayKey),
			Address:   strings.TrimSpace(operation_setting.PayAddress),
		}
		if cfg.PartnerID == "" || cfg.Key == "" || cfg.Address == "" {
			return epayMerchantConfig{}, false
		}
		if _, err := validateEpayGatewayAddress(cfg.Address); err != nil {
			return epayMerchantConfig{}, false
		}
		return cfg, true
	}
	siteConfig, ok := parseSitePayConfig(site.PayConfig)
	if !ok {
		return epayMerchantConfig{}, false
	}
	return epayMerchantConfig{
		PartnerID: siteConfig.EpayId,
		Key:       siteConfig.EpayKey,
		Address:   siteConfig.PayAddress,
	}, true
}

// getEpayClientForSite returns the epay client for a request's site: the sub-site's own
// pay_config when present, otherwise the global (main-site) config. nil if incomplete.
func getEpayClientForSite(site *model.Site) *epay.Client {
	cfg, ok := epayMerchantConfigForSite(site)
	if !ok {
		return nil
	}
	if err := service.ValidateStrictSSRFProtectedFetchURL(cfg.Address); err != nil {
		return nil
	}
	client, err := epay.NewClient(&epay.Config{PartnerID: cfg.PartnerID, Key: cfg.Key}, cfg.Address)
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
		minTopup = common.QuotaFromDecimal(dMinTopup.Mul(dQuotaPerUnit))
	}
	return int64(minTopup)
}

func getTopUpQuota(amount int64) (int, error) {
	quota := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quota = decimal.NewFromInt(quota.Div(quotaPerUnit).IntPart()).Mul(quotaPerUnit)
	} else {
		quota = quota.Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	return common.QuotaFromDecimalStrict(quota)
}

func getMaxTopUpAmount() int64 {
	if common.QuotaPerUnit <= 0 {
		return 0
	}
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	maxStoredAmount := decimal.NewFromInt(common.MaxQuota - 1).
		Div(quotaPerUnit).
		Floor()
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return maxStoredAmount.Add(decimal.NewFromInt(1)).
			Mul(quotaPerUnit).
			Ceil().
			Sub(decimal.NewFromInt(1)).
			IntPart()
	}
	return maxStoredAmount.IntPart()
}

func validateCreditedQuota(quota decimal.Decimal) (int, error) {
	value, err := common.QuotaFromDecimalStrict(quota)
	if err != nil {
		return 0, errors.New("充值额度超出系统可表示范围")
	}
	if value <= 0 {
		return 0, errors.New("充值额度必须大于 0")
	}
	return value, nil
}

func validateTopUpQuota(amount int64) (int, error) {
	quota, err := getTopUpQuota(amount)
	if err == nil && quota > 0 {
		return quota, nil
	}
	maxAmount := getMaxTopUpAmount()
	if maxAmount > 0 && amount > maxAmount {
		return 0, fmt.Errorf("单笔充值数量不能大于 %d", maxAmount)
	}
	return 0, errors.New("充值数量无效")
}

func rejectInvalidCreditedQuota(c *gin.Context, userId int, quota decimal.Decimal) bool {
	creditedQuota, err := validateCreditedQuota(quota)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userId, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return true
	}
	return false
}

func rejectInvalidTopUpQuota(c *gin.Context, userId int, amount int64) bool {
	creditedQuota, err := validateTopUpQuota(amount)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userId, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return true
	}
	return false
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
	id := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, id, req.Amount) {
		return
	}

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

	site := middleware.GetRequestSite(c)
	paymentMethodAllowed := false
	for _, method := range epayPayMethodsForSite(site) {
		if method["type"] == req.PaymentMethod {
			paymentMethodAllowed = true
			break
		}
	}
	if !paymentMethodAllowed {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	// Sub-site pre-order check: the agent must have enough procurement-wallet balance to
	// cover this recharge's wholesale cost (面值 × discount_rate). If not, reject up front
	// so the user never pays into an order the platform can't settle (auto-degradation).
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
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%d money=%.2f site_id=%d notify_path=%q request_host=%q trusted_host=%v", id, tradeNo, req.PaymentMethod, req.Amount, payMoney, middleware.GetRequestSiteId(c), notifyUrl.Path, c.Request.Host, service.IsRequestHostTrusted(c)))
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
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 回调 POST 表单解析失败 reason=form_parse_failed path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
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
	topUp    *model.TopUp
	site     *model.Site
	info     *epay.VerifyRes
	merchant epayMerchantConfig
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
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调缺少订单号 source=%s path=%q client_ip=%s", source, c.Request.URL.Path, c.ClientIP()))
		return nil
	}
	topUp, err := model.FindTopUpByTradeNo(tradeNo)
	if err != nil {
		// Transient DB error — NOT "order missing". Rejecting lets the gateway retry.
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 回调订单查询失败 reason=order_lookup_failed source=%s client_ip=%s", source, c.ClientIP()))
		return nil
	}
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调订单不存在 source=%s trade_no=%q client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	if topUp.PaymentProvider != model.PaymentProviderEpay {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 source=%s trade_no=%q order_provider=%q client_ip=%s", source, tradeNo, topUp.PaymentProvider, c.ClientIP()))
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
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 子站订单无法加载子站 source=%s trade_no=%q site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
			return nil
		}
	}
	merchant, ok := epayMerchantConfigForSite(site)
	if !ok {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 source=%s trade_no=%q site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
		return nil
	}
	client, err := epay.NewClient(&epay.Config{PartnerID: merchant.PartnerID, Key: merchant.Key}, merchant.Address)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 初始化失败 source=%s trade_no=%q site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
		return nil
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调验签失败 reason=invalid_signature source=%s trade_no=%q client_ip=%s", source, topUp.TradeNo, c.ClientIP()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调验签失败 source=%s trade_no=%q client_ip=%s verify_status=false", source, tradeNo, c.ClientIP()))
		}
		return nil
	}
	if params["pid"] == "" || params["pid"] != merchant.PartnerID {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调商户不匹配 source=%s trade_no=%q site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
		return nil
	}
	if strings.TrimSpace(verifyInfo.TradeNo) == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调缺少网关订单号 source=%s trade_no=%q site_id=%d client_ip=%s", source, tradeNo, topUp.SiteId, c.ClientIP()))
		return nil
	}
	if strings.TrimSpace(verifyInfo.Type) == "" || verifyInfo.Type != topUp.PaymentMethod {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调支付方式不匹配 source=%s trade_no=%q callback_type=%q order_type=%q site_id=%d client_ip=%s", source, tradeNo, verifyInfo.Type, topUp.PaymentMethod, topUp.SiteId, c.ClientIP()))
		return nil
	}
	if !epayMoneyMatchesOrderStrict(verifyInfo.Money, topUp.Money) {
		// Defense in depth against a compromised/malicious gateway reporting success
		// for a different amount than the order was created for.
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 回调金额与订单不符 source=%s trade_no=%q callback_money=%q order_money=%.2f client_ip=%s", source, tradeNo, verifyInfo.Money, topUp.Money, c.ClientIP()))
		return nil
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 回调验签成功 source=%s trade_no=%q callback_type=%q trade_status=%q client_ip=%s", source, verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP()))
	return &epayCallbackVerification{topUp: topUp, site: site, info: verifyInfo, merchant: merchant}
}

// epayMoneyMatchesOrderStrict requires the gateway to return a concrete amount that
// exactly equals the two-decimal value submitted at order creation. It deliberately
// rejects missing values and values that merely round to the expected amount.
func epayMoneyMatchesOrderStrict(gatewayMoney string, orderMoney float64) bool {
	gatewayMoney = strings.TrimSpace(gatewayMoney)
	if gatewayMoney == "" {
		return false
	}
	got, err := decimal.NewFromString(gatewayMoney)
	if err != nil {
		return false
	}
	want, err := decimal.NewFromString(strconv.FormatFloat(orderMoney, 'f', 2, 64))
	if err != nil {
		return false
	}
	return got.Equal(want)
}

// confirmEpayTopUpPayment treats the signed callback only as a trigger. A pending
// order is eligible for settlement only after the owning gateway independently
// reports that the same merchant order is paid for the exact stored amount.
func confirmEpayTopUpPayment(c *gin.Context, verified *epayCallbackVerification, source string) bool {
	if verified.topUp.Status == common.TopUpStatusSuccess || verified.topUp.Status == model.TopUpStatusManualReview {
		return true
	}
	if verified.topUp.Status != common.TopUpStatusPending {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 查单跳过非待支付订单 source=%s trade_no=%q status=%q site_id=%d client_ip=%s", source, verified.topUp.TradeNo, verified.topUp.Status, verified.topUp.SiteId, c.ClientIP()))
		return false
	}
	now := common.GetTimestamp()
	claimed, err := model.ClaimEpayTopUpQueryAttempt(
		verified.topUp.Id,
		now,
		now-epayCallbackQueryCooldownSeconds,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 查单限流状态更新失败 source=%s trade_no=%q site_id=%d client_ip=%s error=%q", source, verified.topUp.TradeNo, verified.topUp.SiteId, c.ClientIP(), err.Error()))
		return false
	}
	if !claimed {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 重复查单已抑制 source=%s trade_no=%q site_id=%d client_ip=%s", source, verified.topUp.TradeNo, verified.topUp.SiteId, c.ClientIP()))
		return false
	}
	result, err := queryEpayOrderContextForSite(
		c.Request.Context(),
		verified.merchant.Address,
		verified.merchant.PartnerID,
		verified.merchant.Key,
		verified.topUp.TradeNo,
		verified.topUp.SiteId,
	)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 主动查单失败 source=%s trade_no=%q site_id=%d client_ip=%s error=%q", source, verified.topUp.TradeNo, verified.topUp.SiteId, c.ClientIP(), err.Error()))
		return false
	}
	err = validateEpayPaidOrder(result, epayOrderExpectation{
		PartnerID:      verified.merchant.PartnerID,
		TradeNo:        verified.info.TradeNo,
		ServiceTradeNo: verified.topUp.TradeNo,
		Type:           verified.info.Type,
		Money:          verified.topUp.Money,
	})
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 主动查单未确认付款 source=%s trade_no=%q site_id=%d client_ip=%s error=%q", source, verified.topUp.TradeNo, verified.topUp.SiteId, c.ClientIP(), err.Error()))
		return false
	}
	return true
}

// settleEpayTopUp finishes a gateway-confirmed PAID top-up order exactly once and runs
// the post-settlement bookkeeping shared by the async notify, the browser return and
// the reconciliation sweep. Main-site orders use RechargeEpay's row lock and atomic
// quota-capacity check; sub-site orders use CompleteEpayTopUp's wallet debit + CAS.
func settleEpayTopUp(ctx context.Context, topUp *model.TopUp, site *model.Site, callerIP string, source string) (string, error) {
	return settleEpayTopUpWithPaymentMethod(ctx, topUp, site, "", callerIP, source)
}

func settleEpayTopUpWithPaymentMethod(ctx context.Context, topUp *model.TopUp, site *model.Site, actualPaymentMethod string, callerIP string, source string) (string, error) {
	LockOrder(topUp.TradeNo)
	defer UnlockOrder(topUp.TradeNo)

	if site == nil {
		alreadyDone, settleErr := model.RechargeEpay(topUp.TradeNo, actualPaymentMethod, callerIP)
		if settleErr != nil {
			logger.LogError(ctx, fmt.Sprintf("易支付 结算失败 source=%s trade_no=%s user_id=%d caller_ip=%s error=%q", source, topUp.TradeNo, topUp.UserId, callerIP, settleErr.Error()))
			return "", settleErr
		}
		if alreadyDone {
			logger.LogInfo(ctx, fmt.Sprintf("易支付 重复结算幂等忽略 source=%s trade_no=%s user_id=%d caller_ip=%s", source, topUp.TradeNo, topUp.UserId, callerIP))
		}
		return common.TopUpStatusSuccess, nil
	}

	// Settle: for a sub-site, debit the agent wallet (面值 × discount_rate) atomically with
	// crediting the user; idempotent across duplicate callbacks; insufficient wallet parks
	// the order for manual review (user is NOT credited until an admin resolves it).
	cost := siteTopupCostMilli(topUp.Money, site.DiscountRate)
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
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s param_count=%d", c.Request.URL.Path, c.ClientIP(), c.Request.Method, len(params)))
	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.URL.Path, c.ClientIP()))
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
	if !confirmEpayTopUpPayment(c, v, "notify") {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if _, err := settleEpayTopUpWithPaymentMethod(c.Request.Context(), v.topUp, v.site, v.info.Type, c.ClientIP(), "notify"); err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	_, _ = c.Writer.Write([]byte("success"))
}

// EpayReturn handles the browser return after an epay top-up payment. The signed
// browser payload is only a trigger: the owning gateway must independently confirm
// the exact paid order before settlement. This safely recovers a lost async notify;
// settlement remains idempotent when both paths race. The user ends on the wallet page.
func EpayReturn(c *gin.Context) {
	params := parseEpayCallbackParams(c)
	if len(params) == 0 {
		// Bare visit without a signed payload: just land the user on the wallet page.
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet"))
		return
	}
	v := verifyEpayTopUpCallback(c, params, "return")
	if v == nil {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=fail"))
		return
	}
	if v.info.TradeStatus != epay.StatusTradeSuccess {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=pending"))
		return
	}
	if !confirmEpayTopUpPayment(c, v, "return") {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=pending"))
		return
	}
	finalStatus, err := settleEpayTopUpWithPaymentMethod(c.Request.Context(), v.topUp, v.site, v.info.Type, c.ClientIP(), "return")
	if err != nil {
		// Paid, but settlement hit a transient error — the notify retry or the
		// reconciliation sweep will finish it; show the user "processing".
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=pending"))
		return
	}
	if finalStatus == common.TopUpStatusSuccess {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=success"))
		return
	}
	// manual_review (paid, awaiting admin) or any other parked state.
	c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=pending"))
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
	id := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, id, req.Amount) {
		return
	}
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
