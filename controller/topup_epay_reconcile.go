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
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	// epayReconcileBatchSize caps gateway queries per sweep. Persistent attempt
	// timestamps rotate the bounded work list fairly through abandoned orders.
	epayReconcileBatchSize = 100
	// A callback that already triggered a query suppresses duplicates across all
	// application instances for longer than the outbound query timeout.
	epayCallbackQueryCooldownSeconds int64 = 15

	epayOrderQueryTimeout = 10 * time.Second
	// Distinct signed callbacks can target different valid pending orders. Bound
	// aggregate outbound work as a second line of defense against a leaked key.
	epayOrderQueryMaxConcurrency = 16
	// Untrusted sub-site merchants share only half of the process-wide capacity,
	// preserving room for the global merchant and subscription recovery.
	epaySubsiteOrderQueryMaxConcurrency = 8
	// A single sub-site may not monopolize the shared sub-site pool.
	epayPerSiteOrderQueryMaxConcurrency = 2
)

// epayOrderQueryResponse is the classic 易支付 merchant order query response
// (api.php?act=order). Scalar fields use RawMessage because gateway forks disagree
// on whether values such as code, status, pid and money are JSON numbers or strings.
type epayOrderQueryResponse struct {
	Code           json.RawMessage `json:"code"`
	Msg            string          `json:"msg"`
	Status         json.RawMessage `json:"status"`
	PartnerID      json.RawMessage `json:"pid"`
	TradeNo        json.RawMessage `json:"trade_no"`
	ServiceTradeNo json.RawMessage `json:"out_trade_no"`
	Type           json.RawMessage `json:"type"`
	Money          json.RawMessage `json:"money"`
}

type epayOrderQueryResult struct {
	Paid           bool
	PartnerID      string
	TradeNo        string
	ServiceTradeNo string
	Type           string
	Money          string
}

type epayOrderExpectation struct {
	PartnerID      string
	TradeNo        string
	ServiceTradeNo string
	Type           string
	Money          float64
}

// epayOrderQueryBaseClient is a narrow test seam for trusting httptest TLS
// certificates. Production always supplies the project's SSRF-protected client;
// queryEpayOrder only reuses its dial-time-protected transport and enforces its own
// shorter timeout and no-redirect policy below.
var epayOrderQueryBaseClient = service.NewStrictSSRFProtectedHTTPClient()
var epayOrderQuerySlots = make(chan struct{}, epayOrderQueryMaxConcurrency)
var epaySubsiteOrderQuerySlots = make(chan struct{}, epaySubsiteOrderQueryMaxConcurrency)
var epaySiteOrderQueryActivity = struct {
	sync.Mutex
	active map[int]int
}{active: make(map[int]int)}

// validateEpayGatewayAddress validates an epay gateway base URL before any payment
// or query request is built from it. Credentials, query parameters and fragments do
// not belong in the configured base address: accepting them makes URL composition
// ambiguous and risks exposing secrets. The returned URL is safe for callers to
// extend with a gateway path and query.
func validateEpayGatewayAddress(address string) (*url.URL, error) {
	address = strings.TrimSpace(address)
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("invalid epay gateway address: %w", err)
	}
	if !parsed.IsAbs() || !strings.EqualFold(parsed.Scheme, "https") {
		return nil, errors.New("epay gateway address must be an absolute HTTPS URL")
	}
	if parsed.Host == "" || parsed.Hostname() == "" || parsed.Opaque != "" {
		return nil, errors.New("epay gateway address must include a valid host")
	}
	if parsed.User != nil {
		return nil, errors.New("epay gateway address must not include user information")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, errors.New("epay gateway address must not include a query string")
	}
	if strings.Contains(address, "#") || parsed.Fragment != "" || parsed.RawFragment != "" {
		return nil, errors.New("epay gateway address must not include a fragment")
	}
	return parsed, nil
}

// jsonScalarString renders a JSON scalar that may be encoded as either a string or a
// number ("1" vs 1) as its plain text form. A JSON null (or absent field) becomes the
// empty string; strict settlement validation then rejects a missing identity or amount.
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

// queryEpayOrder asks an epay gateway for the authoritative state of a merchant order.
// A gateway-level failure (non-200, non-JSON, code != 1) is an error — callers must
// treat the order as UNCONFIRMED, never as paid. Returned errors never contain the key.
func queryEpayOrder(payAddress, pid, key, tradeNo string) (*epayOrderQueryResult, error) {
	return queryEpayOrderContext(context.Background(), payAddress, pid, key, tradeNo)
}

func queryEpayOrderContext(ctx context.Context, payAddress, pid, key, tradeNo string) (*epayOrderQueryResult, error) {
	return queryEpayOrderContextForSite(ctx, payAddress, pid, key, tradeNo, 0)
}

func queryEpayOrderContextForSite(ctx context.Context, payAddress, pid, key, tradeNo string, siteID int) (*epayOrderQueryResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	gatewayURL, err := validateEpayGatewayAddress(payAddress)
	if err != nil {
		return nil, err
	}
	releaseCapacity, err := acquireEpayOrderQueryCapacity(ctx, siteID)
	if err != nil {
		return nil, err
	}
	defer releaseCapacity()

	q := url.Values{}
	q.Set("act", "order")
	q.Set("pid", pid)
	q.Set("key", key)
	q.Set("out_trade_no", tradeNo)
	gatewayURL.Path = strings.TrimRight(gatewayURL.Path, "/") + "/api.php"
	gatewayURL.RawPath = ""
	gatewayURL.RawQuery = q.Encode()
	requestURL := gatewayURL.String()

	// SSRF defense: payAddress is operator/sub-site-supplied and otherwise unvalidated,
	// so it must clear the shared fetch policy before dispatch. The reused protected
	// transport validates again at RoundTrip and re-resolves/validates the address at
	// dial time, closing the DNS-rebinding gap between URL validation and connection.
	if err := service.ValidateStrictSSRFProtectedFetchURL(requestURL); err != nil {
		return nil, errors.New(redactEpaySecret("gateway address rejected: "+err.Error(), key))
	}
	baseClient := epayOrderQueryBaseClient
	if baseClient == nil || baseClient.Transport == nil {
		return nil, errors.New("secure gateway HTTP client is not initialized")
	}
	client := &http.Client{
		Transport: baseClient.Transport,
		Timeout:   epayOrderQueryTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// The merchant key lives in the query string. Following even a same-host
			// redirect would allow net/http to expose that URL through Referer and lets
			// an untrusted gateway bounce the request to another origin.
			return errors.New("epay gateway redirects are not allowed")
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, errors.New(redactEpaySecret(err.Error(), key))
	}
	resp, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("gateway order query canceled: %w", context.Canceled)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("gateway order query timed out: %w", context.DeadlineExceeded)
		}
		return nil, errors.New(redactEpaySecret(err.Error(), key))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	var parsed epayOrderQueryResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("gateway response not JSON: %w", err)
	}
	if jsonScalarString(parsed.Code) != "1" {
		return nil, errors.New("gateway rejected order query")
	}
	return &epayOrderQueryResult{
		Paid:           jsonScalarString(parsed.Status) == "1",
		PartnerID:      jsonScalarString(parsed.PartnerID),
		TradeNo:        jsonScalarString(parsed.TradeNo),
		ServiceTradeNo: jsonScalarString(parsed.ServiceTradeNo),
		Type:           jsonScalarString(parsed.Type),
		Money:          jsonScalarString(parsed.Money),
	}, nil
}

// acquireEpayOrderQueryCapacity applies three nested, non-blocking limits:
// process-wide, all sub-sites combined, and one specific sub-site. Main-site and
// subscription queries use siteID 0 and therefore always retain the capacity that
// sub-sites are not permitted to consume.
func acquireEpayOrderQueryCapacity(ctx context.Context, siteID int) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	totalSlots := epayOrderQuerySlots
	subsiteSlots := epaySubsiteOrderQuerySlots
	subsiteAcquired := false
	siteAcquired := false

	releaseSubsite := func() {
		if siteAcquired {
			epaySiteOrderQueryActivity.Lock()
			epaySiteOrderQueryActivity.active[siteID]--
			if epaySiteOrderQueryActivity.active[siteID] <= 0 {
				delete(epaySiteOrderQueryActivity.active, siteID)
			}
			epaySiteOrderQueryActivity.Unlock()
		}
		if subsiteAcquired {
			<-subsiteSlots
		}
	}

	if siteID > 0 {
		select {
		case subsiteSlots <- struct{}{}:
			subsiteAcquired = true
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, errors.New("sub-site gateway order query capacity exhausted")
		}

		epaySiteOrderQueryActivity.Lock()
		if epaySiteOrderQueryActivity.active[siteID] >= epayPerSiteOrderQueryMaxConcurrency {
			epaySiteOrderQueryActivity.Unlock()
			releaseSubsite()
			return nil, errors.New("site gateway order query capacity exhausted")
		}
		epaySiteOrderQueryActivity.active[siteID]++
		siteAcquired = true
		epaySiteOrderQueryActivity.Unlock()
	}

	select {
	case totalSlots <- struct{}{}:
		return func() {
			<-totalSlots
			releaseSubsite()
		}, nil
	case <-ctx.Done():
		releaseSubsite()
		return nil, ctx.Err()
	default:
		releaseSubsite()
		return nil, errors.New("gateway order query capacity exhausted")
	}
}

// queryEpayOrderPaid keeps the compact query contract used by older callers/tests.
func queryEpayOrderPaid(payAddress, pid, key, tradeNo string) (bool, string, error) {
	result, err := queryEpayOrder(payAddress, pid, key, tradeNo)
	if err != nil {
		return false, "", err
	}
	return result.Paid, result.Money, nil
}

// validateEpayPaidOrder binds an authenticated gateway query back to the local order
// and to the signed callback fields. Missing identity fields fail closed: a response
// that cannot prove which merchant/order it describes is not payment evidence.
func validateEpayPaidOrder(result *epayOrderQueryResult, expected epayOrderExpectation) error {
	if result == nil || !result.Paid {
		return errors.New("gateway order is not paid")
	}
	if !epayMoneyMatchesOrderStrict(result.Money, expected.Money) {
		return fmt.Errorf("gateway amount mismatch: got %q", result.Money)
	}
	if expected.PartnerID != "" && result.PartnerID != expected.PartnerID {
		return fmt.Errorf("gateway merchant mismatch: got %q", result.PartnerID)
	}
	if expected.ServiceTradeNo != "" && result.ServiceTradeNo != expected.ServiceTradeNo {
		return fmt.Errorf("gateway merchant order mismatch: got %q", result.ServiceTradeNo)
	}
	if expected.TradeNo != "" && result.TradeNo != expected.TradeNo {
		return fmt.Errorf("gateway order mismatch: got %q", result.TradeNo)
	}
	if expected.Type != "" && result.Type != expected.Type {
		return fmt.Errorf("gateway payment type mismatch: got %q", result.Type)
	}
	return nil
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
	orderIDs := make([]int, 0, len(orders))
	for _, order := range orders {
		if order != nil {
			orderIDs = append(orderIDs, order.Id)
		}
	}
	if err := model.MarkEpayTopUpQueryAttempts(orderIDs, now); err != nil {
		return summary, err
	}
	for _, topUp := range orders {
		var site *model.Site
		if topUp.SiteId > 0 {
			// The owning sub-site's own merchant config is authoritative — same
			// resolution as the notify path; unresolvable config means this sweep
			// cannot decide the order, so it is skipped, never settled for free.
			site, _ = model.GetSiteById(topUp.SiteId)
			if site == nil {
				summary.Skipped++
				continue
			}
		}
		merchant, ok := epayMerchantConfigForSite(site)
		if !ok {
			summary.Skipped++
			continue
		}
		queryResult, qerr := queryEpayOrderContextForSite(
			ctx,
			merchant.Address,
			merchant.PartnerID,
			merchant.Key,
			topUp.TradeNo,
			topUp.SiteId,
		)
		if qerr != nil {
			// Unconfirmed (gateway unreachable / fork without act=order): leave the
			// order pending; it ages out of the window after 24h.
			summary.Failed++
			logger.LogWarn(ctx, fmt.Sprintf("易支付 对账查单失败 trade_no=%s site_id=%d error=%q", topUp.TradeNo, topUp.SiteId, qerr.Error()))
			continue
		}
		if !queryResult.Paid {
			summary.Unpaid++
			continue
		}
		if strings.TrimSpace(queryResult.TradeNo) == "" {
			summary.Failed++
			logger.LogError(ctx, fmt.Sprintf("易支付 对账缺少网关订单号，跳过结算 trade_no=%s", topUp.TradeNo))
			continue
		}
		if err := validateEpayPaidOrder(queryResult, epayOrderExpectation{
			PartnerID:      merchant.PartnerID,
			ServiceTradeNo: topUp.TradeNo,
			Type:           topUp.PaymentMethod,
			Money:          topUp.Money,
		}); err != nil {
			summary.Failed++
			logger.LogError(ctx, fmt.Sprintf("易支付 对账结果与订单不符，跳过结算 trade_no=%s order_money=%.2f error=%q", topUp.TradeNo, topUp.Money, err.Error()))
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
