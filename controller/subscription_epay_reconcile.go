package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// subscriptionEpayReconcileSummary is persisted inside the existing epay
// reconciliation system-task result. Subscription epay orders always belong to
// the global merchant, regardless of the domain through which checkout started.
type subscriptionEpayReconcileSummary struct {
	Scanned        int `json:"scanned"`
	Settled        int `json:"settled"`
	AlreadySettled int `json:"already_settled"`
	Unpaid         int `json:"unpaid"`
	Failed         int `json:"failed"`
}

// reconcileEpayPendingSubscriptionsOnce recovers paid subscription orders whose
// notify/return path could not finish. The gateway query is payment evidence only
// when every order identity and the exact amount bind back to the local snapshot.
func reconcileEpayPendingSubscriptionsOnce(ctx context.Context) (subscriptionEpayReconcileSummary, error) {
	now := common.GetTimestamp()
	orders, err := model.GetPendingEpaySubscriptionOrders(
		now-epayReconcileWindowSeconds,
		now-epayReconcileGraceSeconds,
		epayReconcileBatchSize,
	)
	if err != nil {
		return subscriptionEpayReconcileSummary{}, err
	}
	summary := subscriptionEpayReconcileSummary{Scanned: len(orders)}
	if len(orders) == 0 {
		return summary, nil
	}
	orderIDs := make([]int, 0, len(orders))
	for _, order := range orders {
		if order != nil {
			orderIDs = append(orderIDs, order.Id)
		}
	}
	if err := model.MarkEpaySubscriptionOrderQueryAttempts(orderIDs, now); err != nil {
		return summary, err
	}

	merchant, ok := epayMerchantConfigForSite(nil)
	if !ok {
		summary.Failed = len(orders)
		return summary, errors.New("epay global merchant configuration is incomplete")
	}

	for _, order := range orders {
		if order == nil || strings.TrimSpace(order.TradeNo) == "" || strings.TrimSpace(order.PaymentMethod) == "" {
			summary.Failed++
			continue
		}

		queryResult, queryErr := queryEpayOrderContext(ctx, merchant.Address, merchant.PartnerID, merchant.Key, order.TradeNo)
		if queryErr != nil {
			summary.Failed++
			logger.LogWarn(ctx, fmt.Sprintf("订阅易支付对账查单失败 trade_no=%s error=%q", order.TradeNo, queryErr.Error()))
			continue
		}
		if !queryResult.Paid {
			summary.Unpaid++
			continue
		}
		if strings.TrimSpace(queryResult.TradeNo) == "" {
			summary.Failed++
			logger.LogWarn(ctx, fmt.Sprintf("订阅易支付对账缺少网关订单号 trade_no=%s", order.TradeNo))
			continue
		}
		if err := validateEpayPaidOrder(queryResult, epayOrderExpectation{
			PartnerID:      merchant.PartnerID,
			ServiceTradeNo: order.TradeNo,
			Type:           order.PaymentMethod,
			Money:          order.Money,
		}); err != nil {
			summary.Failed++
			logger.LogWarn(ctx, fmt.Sprintf("订阅易支付对账结果与订单不符 trade_no=%s order_money=%.2f error=%q", order.TradeNo, order.Money, err.Error()))
			continue
		}

		providerPayload := common.GetJsonString(map[string]string{
			"source":       "reconcile",
			"status":       "1",
			"pid":          queryResult.PartnerID,
			"trade_no":     queryResult.TradeNo,
			"out_trade_no": queryResult.ServiceTradeNo,
			"type":         queryResult.Type,
			"money":        queryResult.Money,
		})
		LockOrder(order.TradeNo)
		completed, completeErr := model.CompleteSubscriptionOrderWithResult(
			order.TradeNo,
			providerPayload,
			model.PaymentProviderEpay,
			queryResult.Type,
		)
		UnlockOrder(order.TradeNo)
		if completeErr != nil {
			summary.Failed++
			logger.LogError(ctx, fmt.Sprintf("订阅易支付对账结算失败 trade_no=%s error=%q", order.TradeNo, completeErr.Error()))
			continue
		}
		if completed {
			summary.Settled++
		} else {
			summary.AlreadySettled++
		}
	}

	if summary.Settled+summary.Failed > 0 {
		logger.LogInfo(ctx, fmt.Sprintf(
			"订阅易支付对账完成 scanned=%d settled=%d already_settled=%d unpaid=%d failed=%d",
			summary.Scanned,
			summary.Settled,
			summary.AlreadySettled,
			summary.Unpaid,
			summary.Failed,
		))
	}
	return summary, nil
}
