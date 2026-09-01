package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type SubscriptionEpayPayRequest struct {
	PlanId        int    `json:"plan_id"`
	PaymentMethod string `json:"payment_method"`
}

func SubscriptionRequestEpay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionEpayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}

	userId := c.GetInt("id")

	// Browser return stays per-domain (the user is already on that domain); the server-to-server
	// notify must hit the STABLE, gateway-registered callback address — subscription epay always
	// uses the global merchant (GetEpayClient below), so a per-request-host notify could target an
	// unreachable/unregistered domain and strand a paid order.
	returnUrl, err := url.Parse(strings.TrimRight(service.GetRequestBaseURL(c), "/") + "/api/subscription/epay/return")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}
	notifyUrl, err := url.Parse(strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/subscription/epay/notify")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("SUBUSR%dNO%s", userId, tradeNo)

	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}

	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	createdOrder := true
	if err := order.InsertWithPlanSnapshot(plan); err != nil {
		if errors.Is(err, model.ErrSubscriptionPurchaseLimitReached) {
			existing, lookupErr := model.GetReusablePendingSubscriptionOrder(
				userId,
				plan.Id,
				model.PaymentProviderEpay,
				req.PaymentMethod,
			)
			if lookupErr != nil {
				common.ApiError(c, err)
				return
			}
			order = existing
			createdOrder = false
		} else {
			common.ApiErrorMsg(c, "创建订单失败")
			return
		}
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           order.PaymentMethod,
		ServiceTradeNo: order.TradeNo,
		Name:           fmt.Sprintf("SUB:%s", plan.Title),
		Money:          strconv.FormatFloat(order.Money, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		if createdOrder {
			_ = model.ExpireSubscriptionOrder(order.TradeNo, model.PaymentProviderEpay)
		}
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

type subscriptionEpayCallbackVerification struct {
	order    *model.SubscriptionOrder
	info     *epay.VerifyRes
	merchant epayMerchantConfig
}

// verifySubscriptionEpayCallback binds a signed callback to the global merchant and
// the immutable local subscription order snapshot. The callback Host is deliberately
// irrelevant: subscriptions currently always use the global epay merchant, even when
// the browser entered through another trusted domain.
func verifySubscriptionEpayCallback(c *gin.Context, params map[string]string, source string) *subscriptionEpayCallbackVerification {
	tradeNo := params["out_trade_no"]
	if tradeNo == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调缺少订单号 source=%s path=%q client_ip=%s", source, c.Request.URL.Path, c.ClientIP()))
		return nil
	}
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	if order == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调订单不存在 source=%s trade_no=%q client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	if order.PaymentProvider != model.PaymentProviderEpay {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付订单网关不匹配 source=%s trade_no=%q order_provider=%q client_ip=%s", source, tradeNo, order.PaymentProvider, c.ClientIP()))
		return nil
	}
	merchant, ok := epayMerchantConfigForSite(nil)
	if !ok {
		logger.LogError(c.Request.Context(), fmt.Sprintf("订阅易支付全局商户配置不完整 source=%s trade_no=%q client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	client, err := epay.NewClient(&epay.Config{PartnerID: merchant.PartnerID, Key: merchant.Key}, merchant.Address)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("订阅易支付 client 初始化失败 source=%s trade_no=%q client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调验签失败 source=%s trade_no=%q client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	if params["pid"] == "" || params["pid"] != merchant.PartnerID {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调商户不匹配 source=%s trade_no=%q client_ip=%s", source, tradeNo, c.ClientIP()))
		return nil
	}
	if verifyInfo.ServiceTradeNo != tradeNo || strings.TrimSpace(verifyInfo.TradeNo) == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调订单标识无效 source=%s trade_no=%q gateway_trade_no=%q client_ip=%s", source, tradeNo, verifyInfo.TradeNo, c.ClientIP()))
		return nil
	}
	if strings.TrimSpace(verifyInfo.Type) == "" || verifyInfo.Type != order.PaymentMethod {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调支付方式不匹配 source=%s trade_no=%q callback_type=%q order_type=%q client_ip=%s", source, tradeNo, verifyInfo.Type, order.PaymentMethod, c.ClientIP()))
		return nil
	}
	if !epayMoneyMatchesOrderStrict(verifyInfo.Money, order.Money) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付回调金额不匹配 source=%s trade_no=%q callback_money=%q order_money=%.2f client_ip=%s", source, tradeNo, verifyInfo.Money, order.Money, c.ClientIP()))
		return nil
	}
	return &subscriptionEpayCallbackVerification{order: order, info: verifyInfo, merchant: merchant}
}

// confirmAndCompleteSubscriptionEpay treats the callback as a trigger, not payment
// proof. Network I/O happens before the process/DB order locks; CompleteSubscriptionOrder
// rechecks provider and pending state under a database row lock for cross-instance safety.
func confirmAndCompleteSubscriptionEpay(c *gin.Context, verified *subscriptionEpayCallbackVerification, source string) bool {
	if verified.order.Status == common.TopUpStatusSuccess {
		return true
	}
	if verified.order.Status != common.TopUpStatusPending {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付查单跳过非待支付订单 source=%s trade_no=%q status=%q client_ip=%s", source, verified.order.TradeNo, verified.order.Status, c.ClientIP()))
		return false
	}
	now := common.GetTimestamp()
	claimed, err := model.ClaimEpaySubscriptionOrderQueryAttempt(
		verified.order.Id,
		now,
		now-epayCallbackQueryCooldownSeconds,
	)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("订阅易支付查单限流状态更新失败 source=%s trade_no=%q client_ip=%s error=%q", source, verified.order.TradeNo, c.ClientIP(), err.Error()))
		return false
	}
	if !claimed {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付重复查单已抑制 source=%s trade_no=%q client_ip=%s", source, verified.order.TradeNo, c.ClientIP()))
		return false
	}
	result, err := queryEpayOrderContext(c.Request.Context(), verified.merchant.Address, verified.merchant.PartnerID, verified.merchant.Key, verified.order.TradeNo)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付主动查单失败 source=%s trade_no=%q client_ip=%s error=%q", source, verified.order.TradeNo, c.ClientIP(), err.Error()))
		return false
	}
	err = validateEpayPaidOrder(result, epayOrderExpectation{
		PartnerID:      verified.merchant.PartnerID,
		TradeNo:        verified.info.TradeNo,
		ServiceTradeNo: verified.order.TradeNo,
		Type:           verified.info.Type,
		Money:          verified.order.Money,
	})
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("订阅易支付主动查单未确认付款 source=%s trade_no=%q client_ip=%s error=%q", source, verified.order.TradeNo, c.ClientIP(), err.Error()))
		return false
	}

	LockOrder(verified.order.TradeNo)
	defer UnlockOrder(verified.order.TradeNo)
	if err := model.CompleteSubscriptionOrder(verified.order.TradeNo, common.GetJsonString(verified.info), model.PaymentProviderEpay, verified.info.Type); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("订阅易支付结算失败 source=%s trade_no=%q client_ip=%s error=%q", source, verified.order.TradeNo, c.ClientIP(), err.Error()))
		return false
	}
	return true
}

func SubscriptionEpayNotify(c *gin.Context) {
	params := parseEpayCallbackParams(c)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("订阅易支付 webhook 收到请求 path=%q client_ip=%s method=%s param_count=%d", c.Request.URL.Path, c.ClientIP(), c.Request.Method, len(params)))
	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verified := verifySubscriptionEpayCallback(c, params, "notify")
	if verified == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if verified.info.TradeStatus != epay.StatusTradeSuccess {
		_, _ = c.Writer.Write([]byte("success"))
		return
	}
	if !confirmAndCompleteSubscriptionEpay(c, verified, "notify") {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	_, _ = c.Writer.Write([]byte("success"))
}

// SubscriptionEpayReturn handles browser return after payment.
// The signed browser payload is only a trigger; an independent server-side order query
// must confirm payment before settlement. Redirects stay on the trusted request domain.
func SubscriptionEpayReturn(c *gin.Context) {
	params := parseEpayCallbackParams(c)
	if len(params) == 0 {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=fail"))
		return
	}
	verified := verifySubscriptionEpayCallback(c, params, "return")
	if verified == nil {
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=fail"))
		return
	}
	if verified.info.TradeStatus == epay.StatusTradeSuccess {
		if !confirmAndCompleteSubscriptionEpay(c, verified, "return") {
			c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=pending"))
			return
		}
		c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=success"))
		return
	}
	c.Redirect(http.StatusFound, paymentReturnPath(c, "/wallet?pay=pending"))
}
