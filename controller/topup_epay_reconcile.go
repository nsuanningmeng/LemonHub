package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// Epay top-up orders settle only via the gateway's async notify and the browser
// return — and both can be lost (popular 易支付 implementations deliver the notify
// once or give up after a few retries; the user may close the tab before the
// return fires). The reconciliation sweep is the last safety net: it actively asks
// the gateway (api.php?act=order) whether recent pending orders are in fact paid,
// and settles the ones that are.
const (
	// epayReconcileWindowSeconds bounds how far back the sweep looks. Anything older
	// is admin-补单 territory; an unbounded scan would also re-query dead orders forever.
	epayReconcileWindowSeconds int64 = 24 * 60 * 60
	// epayReconcileGraceSeconds keeps the sweep off orders the user may still be
	// paying, and gives the normal notify/return path the first shot.
	epayReconcileGraceSeconds int64 = 120
	// epayReconcileBatchSize caps gateway queries per sweep. The work list is
	// newest-first (see GetPendingEpayTopUps) so a recently-paid order whose callbacks
	// were lost is always queried, even behind a backlog of older abandoned orders.
	epayReconcileBatchSize = 100

	epayOrderQueryTimeout = 10 * time.Second
)

// epayOrderQueryResponse is the classic 易支付 merchant order query response
// (api.php?act=order). code/status/money are RawMessage because gateway forks
// disagree on whether they are JSON numbers or strings.
type epayOrderQueryResponse struct {
	Code   json.RawMessage `json:"code"`
	Msg    string          `json:"msg"`
	Status json.RawMessage `json:"status"`
	Money  json.RawMessage `json:"money"`
}

// jsonScalarString renders a JSON scalar that may be encoded as either a string or a
// number ("1" vs 1) as its plain text form. A JSON null (or absent field) becomes the
// empty string, so a gateway that reports `"money": null` is treated as "amount
// omitted" (tolerated by the amount check) rather than the literal text "null".
func jsonScalarString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return strings.Trim(s, `"`)
}

// redactEpaySecret removes the merchant key from an error/log string. Transport errors
// from net/http embed the full request URL, which for the order query carries the key
// as a query parameter — this keeps that credential out of the logs. Both the raw key
// and its URL-encoded form are stripped, because the key appears percent-encoded inside
// the request URL (url.Values.Encode) but callers may also log the raw value.
func redactEpaySecret(msg, key string) string {
	if key == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, key, "***")
	if enc := url.QueryEscape(key); enc != key {
		msg = strings.ReplaceAll(msg, enc, "***")
	}
	return msg
}

// queryEpayOrderPaid asks an epay gateway whether a merchant order is paid, via the
// classic api.php?act=order merchant query. Returns the paid flag and the
// gateway-side amount (empty when the gateway omits it). A gateway-level failure
// (non-200, non-JSON, code != 1) is an error — the caller must treat the order as
// UNCONFIRMED, never as unpaid-forever. Returned errors never contain the merchant key.
func queryEpayOrderPaid(payAddress, pid, key, tradeNo string) (bool, string, error) {
	q := url.Values{}
	q.Set("act", "order")
	q.Set("pid", pid)
	q.Set("key", key)
	q.Set("out_trade_no", tradeNo)
	requestURL := strings.TrimRight(payAddress, "/") + "/api.php?" + q.Encode()

	// SSRF defense: payAddress is operator/sub-site-supplied and otherwise unvalidated,
	// so it must clear the same fetch policy every other outbound client enforces — on
	// the initial URL AND on each redirect hop (CheckRedirect below), mirroring
	// service/http_client.go, which the stdlib default client would otherwise bypass.
	if err := service.ValidateRelayTargetURL(requestURL); err != nil {
		return false, "", errors.New(redactEpaySecret("gateway address rejected: "+err.Error(), key))
	}
	client := &http.Client{
		Timeout: epayOrderQueryTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := service.ValidateRelayTargetURL(req.URL.String()); err != nil {
				return err
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	resp, err := client.Get(requestURL)
	if err != nil {
		return false, "", errors.New(redactEpaySecret(err.Error(), key))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("gateway http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false, "", err
	}
	var parsed epayOrderQueryResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return false, "", fmt.Errorf("gateway response not JSON: %w", err)
	}
	if jsonScalarString(parsed.Code) != "1" {
		return false, "", fmt.Errorf("gateway code=%s msg=%q", jsonScalarString(parsed.Code), parsed.Msg)
	}
	return jsonScalarString(parsed.Status) == "1", jsonScalarString(parsed.Money), nil
}

// epayReconcileSummary is one sweep's outcome, persisted as the system task result.
type epayReconcileSummary struct {
	Scanned int `json:"scanned"`
	Settled int `json:"settled"`
	Parked  int `json:"parked"`
	Unpaid  int `json:"unpaid"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// reconcileEpayPendingTopUpsOnce sweeps recent pending epay top-up orders and settles
// the ones the gateway reports as PAID. Safe against every concurrent path: settlement
// is the same idempotent CAS the notify and return use.
func reconcileEpayPendingTopUpsOnce(ctx context.Context) (epayReconcileSummary, error) {
	now := common.GetTimestamp()
	orders, err := model.GetPendingEpayTopUps(now-epayReconcileWindowSeconds, now-epayReconcileGraceSeconds, epayReconcileBatchSize)
	if err != nil {
		return epayReconcileSummary{}, err
	}
	summary := epayReconcileSummary{Scanned: len(orders)}
	for _, topUp := range orders {
		var site *model.Site
		var payAddress, pid, key string
		if topUp.SiteId > 0 {
			// The owning sub-site's own merchant config is authoritative — same
			// resolution as the notify path; unresolvable config means this sweep
			// cannot decide the order, so it is skipped, never settled for free.
			site, _ = model.GetSiteById(topUp.SiteId)
			if site == nil {
				summary.Skipped++
				continue
			}
			cfg, ok := parseSitePayConfig(site.PayConfig)
			if !ok {
				summary.Skipped++
				continue
			}
			payAddress, pid, key = cfg.PayAddress, cfg.EpayId, cfg.EpayKey
		} else {
			payAddress, pid, key = operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
			if payAddress == "" || pid == "" || key == "" {
				summary.Skipped++
				continue
			}
		}
		paid, money, qerr := queryEpayOrderPaid(payAddress, pid, key, topUp.TradeNo)
		if qerr != nil {
			// Unconfirmed (gateway unreachable / fork without act=order): leave the
			// order pending; it ages out of the window after 24h.
			summary.Failed++
			logger.LogWarn(ctx, fmt.Sprintf("易支付 对账查单失败 trade_no=%s site_id=%d error=%q", topUp.TradeNo, topUp.SiteId, qerr.Error()))
			continue
		}
		if !paid {
			summary.Unpaid++
			continue
		}
		if !epayCallbackMoneyMatchesOrder(money, topUp.Money) {
			summary.Failed++
			logger.LogError(ctx, fmt.Sprintf("易支付 对账金额与订单不符，跳过结算 trade_no=%s gateway_money=%q order_money=%.2f", topUp.TradeNo, money, topUp.Money))
			continue
		}
		finalStatus, serr := settleEpayTopUp(ctx, topUp, site, "", "reconcile")
		if serr != nil {
			summary.Failed++
			continue
		}
		switch finalStatus {
		case common.TopUpStatusSuccess:
			summary.Settled++
		case model.TopUpStatusManualReview:
			summary.Parked++
		default:
			summary.Skipped++ // a racing notify/return settled it first: idempotent no-op
		}
	}
	if summary.Settled+summary.Parked+summary.Failed > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("易支付 对账完成 scanned=%d settled=%d parked=%d unpaid=%d skipped=%d failed=%d",
			summary.Scanned, summary.Settled, summary.Parked, summary.Unpaid, summary.Skipped, summary.Failed))
	}
	return summary, nil
}
