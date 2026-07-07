package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const epayTestMerchantKey = "epay-test-merchant-key"

// setupEpayCallbackTest wires an isolated in-memory DB plus a global epay merchant
// config, so the notify / return / reconcile paths run end to end against real
// signature verification and real settlement.
func setupEpayCallbackTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Site{}, &model.SiteDomain{}, &model.SiteWalletLog{}, &model.Log{}))
	t.Cleanup(func() { _ = sqlDB.Close() })

	origAddr, origId, origKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = epayTestMerchantKey
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = origAddr, origId, origKey
	})

	// The reconcile happy-path test queries an httptest gateway on 127.0.0.1:<random>,
	// which the default SSRF policy (private IP + non-standard port) would block. Disable
	// SSRF for these fixtures; TestQueryEpayOrderPaidRejectsSSRF exercises it with SSRF on.
	fs := system_setting.GetFetchSetting()
	origSSRF, origPriv := fs.EnableSSRFProtection, fs.AllowPrivateIp
	fs.EnableSSRFProtection, fs.AllowPrivateIp = false, true
	t.Cleanup(func() { fs.EnableSSRFProtection, fs.AllowPrivateIp = origSSRF, origPriv })
}

// createEpayTestOrder creates a user (no inviter) plus a pending main-site epay order
// of Amount=10 / Money=100 created ageSeconds ago.
func createEpayTestOrder(t *testing.T, tradeNo string, ageSeconds int64) *model.User {
	t.Helper()
	pw, err := common.Password2Hash("x")
	require.NoError(t, err)
	name := strings.ToLower(tradeNo)
	u := &model.User{Username: "u" + name, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "aff" + name}
	require.NoError(t, model.DB.Create(u).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: u.Id, Amount: 10, Money: 100, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		CreateTime: common.GetTimestamp() - ageSeconds, Status: common.TopUpStatusPending,
	}).Error)
	return u
}

// signedEpayValues builds a gateway-style callback parameter set signed with the test
// merchant key. overrides are applied BEFORE signing (yielding a validly signed payload
// with those values); tamper AFTER on the returned url.Values to break the signature.
func signedEpayValues(tradeNo string, overrides map[string]string) url.Values {
	return signedEpayValuesWithKey(tradeNo, overrides, epayTestMerchantKey)
}

// signedEpayValuesWithKey is signedEpayValues with an explicit merchant key, so a
// sub-site callback can be signed with the sub-site's OWN key.
func signedEpayValuesWithKey(tradeNo string, overrides map[string]string, key string) url.Values {
	params := map[string]string{
		"pid":          "1001",
		"trade_no":     "GW" + tradeNo,
		"out_trade_no": tradeNo,
		"type":         "alipay",
		"name":         "TUC10",
		"money":        "100.00",
		"trade_status": epay.StatusTradeSuccess,
	}
	for k, v := range overrides {
		params[k] = v
	}
	signed := epay.GenerateParams(params, key)
	values := url.Values{}
	for k, v := range signed {
		values.Set(k, v)
	}
	return values
}

const subSiteEpayKey = "sub-site-own-merchant-key"

// createEpaySubSiteOrder creates a sub-site with its OWN epay pay_config (key
// subSiteEpayKey), the given wallet balance, and a pending epay order of Money=100
// owned by that sub-site. payAddress is the sub-site's own gateway address (a stub URL
// for notify tests, or a live httptest gateway URL for reconcile tests).
func createEpaySubSiteOrder(t *testing.T, tradeNo string, siteId int, walletMilli int64, payAddress string) *model.User {
	t.Helper()
	cfg := sitePayConfig{EpayId: "1001", EpayKey: subSiteEpayKey, PayAddress: payAddress, PayMethods: []string{"alipay"}}
	payConfig, err := common.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Site{
		Id: siteId, Name: "sub", Status: model.SiteStatusNormal,
		WalletBalance: walletMilli, DiscountRate: model.DiscountRateBase, PayConfig: string(payConfig),
	}).Error)

	pw, err := common.Password2Hash("x")
	require.NoError(t, err)
	name := strings.ToLower(tradeNo)
	u := &model.User{Username: "u" + name, SiteId: siteId, Password: pw, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "aff" + name}
	require.NoError(t, model.DB.Create(u).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		SiteId: siteId, UserId: u.Id, Amount: 10, Money: 100, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		CreateTime: common.GetTimestamp() - 300, Status: common.TopUpStatusPending,
	}).Error)
	return u
}

func runEpayReturn(t *testing.T, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/user/epay/return?"+query.Encode(), nil)
	req.Host = "example.com"
	c.Request = req
	EpayReturn(c)
	return w
}

func runEpayNotify(t *testing.T, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/user/epay/notify?"+query.Encode(), nil)
	req.Host = "example.com"
	c.Request = req
	EpayNotify(c)
	return w
}

func epayOrderStatus(t *testing.T, tradeNo string) string {
	t.Helper()
	var o model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&o).Error)
	return o.Status
}

func epayUserQuota(t *testing.T, userId int) int {
	t.Helper()
	var u model.User
	require.NoError(t, model.DB.Select("quota").First(&u, userId).Error)
	return u.Quota
}

// The browser return carries the same signed payload as the async notify and must
// settle the order (the fallback for lost notifies) — exactly once, replay-safe.
func TestEpayReturnSettlesPaidOrderAndIsIdempotent(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo = "RETOK1"
	u := createEpayTestOrder(t, tradeNo, 300)
	wantQuota := int(10 * int64(common.QuotaPerUnit))

	w := runEpayReturn(t, signedEpayValues(tradeNo, nil))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "pay=success")
	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, tradeNo))
	assert.Equal(t, wantQuota, epayUserQuota(t, u.Id))

	// Replay (user refreshes the return page): still success, never double-credits.
	w = runEpayReturn(t, signedEpayValues(tradeNo, nil))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "pay=success")
	assert.Equal(t, wantQuota, epayUserQuota(t, u.Id), "replay must not credit twice")
}

// A payload whose signature does not verify must never settle — the return endpoint
// grants nothing a forged notify couldn't already attempt.
func TestEpayReturnRejectsForgedSignature(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo = "RETBAD"
	u := createEpayTestOrder(t, tradeNo, 300)

	q := signedEpayValues(tradeNo, nil)
	q.Set("money", "999999.00") // tamper AFTER signing → signature mismatch

	w := runEpayReturn(t, q)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "pay=fail")
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, tradeNo))
	assert.Equal(t, 0, epayUserQuota(t, u.Id))
}

// A validly signed callback whose amount differs from the order (compromised or
// malicious gateway) must be rejected by both the return and the notify paths.
func TestEpayCallbackRejectsAmountMismatch(t *testing.T) {
	setupEpayCallbackTest(t)

	const retTradeNo = "RETAMT"
	uRet := createEpayTestOrder(t, retTradeNo, 300)
	w := runEpayReturn(t, signedEpayValues(retTradeNo, map[string]string{"money": "1.00"}))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "pay=fail")
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, retTradeNo))
	assert.Equal(t, 0, epayUserQuota(t, uRet.Id))

	const ntfTradeNo = "NTFAMT"
	uNtf := createEpayTestOrder(t, ntfTradeNo, 300)
	nw := runEpayNotify(t, signedEpayValues(ntfTradeNo, map[string]string{"money": "1.00"}))
	assert.Equal(t, "fail", nw.Body.String())
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, ntfTradeNo))
	assert.Equal(t, 0, epayUserQuota(t, uNtf.Id))
}

// Handler-level regression for the refactored notify path: a signed TRADE_SUCCESS
// callback settles and acks the literal "success" body the gateway string-compares.
func TestEpayNotifySettlesPaidOrder(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo = "NTFOK1"
	u := createEpayTestOrder(t, tradeNo, 300)

	w := runEpayNotify(t, signedEpayValues(tradeNo, nil))
	assert.Equal(t, "success", w.Body.String())
	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, tradeNo))
	assert.Equal(t, int(10*int64(common.QuotaPerUnit)), epayUserQuota(t, u.Id))
}

// A signed non-success trade_status must not settle; the return shows "processing"
// (the notify path keeps its ack semantics separately).
func TestEpayReturnNonSuccessStatusStaysPending(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo = "RETWIP"
	u := createEpayTestOrder(t, tradeNo, 300)

	w := runEpayReturn(t, signedEpayValues(tradeNo, map[string]string{"trade_status": "TRADE_CLOSED"}))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "pay=pending")
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, tradeNo))
	assert.Equal(t, 0, epayUserQuota(t, u.Id))
}

// queryEpayOrderPaid must refuse an SSRF target (private IP / metadata endpoint) when
// SSRF protection is on, and must never leak the merchant key into the returned error.
func TestQueryEpayOrderPaidRejectsSSRF(t *testing.T) {
	fs := system_setting.GetFetchSetting()
	origSSRF, origPriv := fs.EnableSSRFProtection, fs.AllowPrivateIp
	fs.EnableSSRFProtection, fs.AllowPrivateIp = true, false
	t.Cleanup(func() { fs.EnableSSRFProtection, fs.AllowPrivateIp = origSSRF, origPriv })

	const secret = "super-secret-merchant-key"
	paid, money, err := queryEpayOrderPaid("http://169.254.169.254", "1001", secret, "T1")
	require.Error(t, err, "private/metadata target must be rejected before any request")
	assert.False(t, paid)
	assert.Empty(t, money)
	assert.NotContains(t, err.Error(), secret, "merchant key must never appear in errors/logs")
}

// The merchant key must be redacted from the net/http TRANSPORT error (the actual leak
// site: client.Get embeds the full request URL incl. key=...). Uses a key with special
// characters so the URL-encoded form is exercised — plaintext-only redaction would miss
// it. SSRF is allowed here so the request is actually attempted against a dead port.
func TestQueryEpayOrderPaidRedactsKeyInTransportError(t *testing.T) {
	fs := system_setting.GetFetchSetting()
	origSSRF, origPriv := fs.EnableSSRFProtection, fs.AllowPrivateIp
	fs.EnableSSRFProtection, fs.AllowPrivateIp = false, true // reach the HTTP request against loopback
	t.Cleanup(func() { fs.EnableSSRFProtection, fs.AllowPrivateIp = origSSRF, origPriv })

	const secret = "sk+top/secret=key value" // +,/,=,space → percent-encoded in the URL
	_, _, err := queryEpayOrderPaid("http://127.0.0.1:1", "1001", secret, "T1")
	require.Error(t, err, "connection to a dead port must fail at transport level")
	assert.NotContains(t, err.Error(), secret, "raw merchant key must be redacted")
	assert.NotContains(t, err.Error(), url.QueryEscape(secret), "URL-encoded merchant key must be redacted")
}

// epayCallbackMoneyMatchesOrder must accept a gateway amount echoed exactly as the
// order was submitted (strconv.FormatFloat 'f',2), including x.xx5 amounts where a
// decimal.Round comparand would wrongly reject — the regression that would re-strand
// paid orders. Empty/null amounts are tolerated; a genuinely different amount is not.
func TestEpayCallbackMoneyMatchesOrder(t *testing.T) {
	cases := []struct {
		name          string
		callbackMoney string
		orderMoney    float64
		want          bool
	}{
		{"exact", "100.00", 100, true},
		{"trailing-zeros", "100.000", 100, true},
		{"half-even-x005", "1.00", 1.005, true},   // FormatFloat(1.005,'f',2)="1.00"
		{"half-even-x675", "2.67", 2.675, true},   // FormatFloat(2.675,'f',2)="2.67"
		{"empty-tolerated", "", 100, true},
		{"mismatch-low", "1.00", 100, false},
		{"mismatch-high", "999999.00", 100, false},
		{"garbage", "not-a-number", 100, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, epayCallbackMoneyMatchesOrder(tc.callbackMoney, tc.orderMoney))
		})
	}
}

// A sub-site order must be verified with the SUB-SITE's own key, settle the user AND
// atomically debit the agent wallet. A callback signed with the GLOBAL key (wrong key
// for a sub-site order) must be rejected — proof the per-site key is used, not the global.
func TestEpayNotifySubSiteSettlesWithOwnKeyAndDebitsWallet(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo, siteId = "SUBOK1", 7001
	u := createEpaySubSiteOrder(t, tradeNo, siteId, 200000, "https://sub.example.com") // wallet 200000 厘 > cost 100000
	wantQuota := int(10 * int64(common.QuotaPerUnit))
	wantCost := siteTopupCostMilli(100, model.DiscountRateBase) // 100000 厘

	// Wrong key (global) → verification fails → "fail", nothing settled.
	badKey := runEpayNotify(t, signedEpayValues(tradeNo, nil)) // signedEpayValues uses the GLOBAL key
	assert.Equal(t, "fail", badKey.Body.String(), "sub-site order must not verify against the global key")
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, tradeNo))

	// Correct sub-site key → settle + wallet debit.
	w := runEpayNotify(t, signedEpayValuesWithKey(tradeNo, nil, subSiteEpayKey))
	assert.Equal(t, "success", w.Body.String())
	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, tradeNo))
	assert.Equal(t, wantQuota, epayUserQuota(t, u.Id))
	bal, err := model.GetSiteWalletBalance(siteId)
	require.NoError(t, err)
	assert.Equal(t, int64(200000)-wantCost, bal, "agent wallet must be debited by the wholesale cost")
}

// A sub-site order whose agent wallet is drained by callback time parks as manual_review,
// credits nothing, and still acks "success" (the payment is on record; an admin resolves it).
func TestEpayNotifySubSiteInsufficientWalletParksManualReview(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo, siteId = "SUBPARK", 7002
	u := createEpaySubSiteOrder(t, tradeNo, siteId, 100, "https://sub.example.com") // wallet 100 厘 < cost 100000

	w := runEpayNotify(t, signedEpayValuesWithKey(tradeNo, nil, subSiteEpayKey))
	assert.Equal(t, "success", w.Body.String(), "parked order still acks success so the gateway stops retrying")
	assert.Equal(t, model.TopUpStatusManualReview, epayOrderStatus(t, tradeNo))
	assert.Equal(t, 0, epayUserQuota(t, u.Id), "manual_review must not credit the user")
	bal, err := model.GetSiteWalletBalance(siteId)
	require.NoError(t, err)
	assert.Equal(t, int64(100), bal, "insufficient wallet must be left untouched")
}

// The return path and the async notify racing the SAME paid order must credit the user
// exactly once (idempotent CAS + order lock), whichever arrives first.
func TestEpayReturnAndNotifySamePaidOrderCreditOnce(t *testing.T) {
	setupEpayCallbackTest(t)
	const tradeNo = "RACE1"
	u := createEpayTestOrder(t, tradeNo, 300)
	wantQuota := int(10 * int64(common.QuotaPerUnit))

	// Return settles first.
	rw := runEpayReturn(t, signedEpayValues(tradeNo, nil))
	assert.Contains(t, rw.Header().Get("Location"), "pay=success")
	// Notify arrives after — idempotent no-op, still acks success, no double credit.
	nw := runEpayNotify(t, signedEpayValues(tradeNo, nil))
	assert.Equal(t, "success", nw.Body.String())

	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, tradeNo))
	assert.Equal(t, wantQuota, epayUserQuota(t, u.Id), "user credited exactly once across return + notify")
}

// The reconciliation sweep settles a pending order the gateway reports paid, respects
// the just-created grace window, and never settles unpaid or amount-mismatched orders.
func TestReconcileEpayPendingTopUps(t *testing.T) {
	setupEpayCallbackTest(t)

	uPaid := createEpayTestOrder(t, "RECPAID", 300)     // gateway: paid (numeric code/status)
	uPaidStr := createEpayTestOrder(t, "RECPSTR", 300)  // gateway: paid (string-typed code/status)
	uPaidNull := createEpayTestOrder(t, "RECNULL", 300) // gateway: paid, money null (tolerated)
	uUnpaid := createEpayTestOrder(t, "RECWAIT", 300)   // gateway: unpaid
	uBadAmt := createEpayTestOrder(t, "RECBAMT", 300)   // gateway: paid but wrong amount
	uFresh := createEpayTestOrder(t, "RECNEW1", 10)     // inside grace window: not queried
	wantQuota := int(10 * int64(common.QuotaPerUnit))

	gateway := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		require.Equal(t, "order", q.Get("act"))
		require.Equal(t, "1001", q.Get("pid"))
		require.Equal(t, epayTestMerchantKey, q.Get("key"))
		rw.Header().Set("Content-Type", "application/json")
		switch q.Get("out_trade_no") {
		case "RECPAID":
			fmt.Fprint(rw, `{"code":1,"msg":"ok","out_trade_no":"RECPAID","money":"100.00","status":1}`)
		case "RECPSTR":
			fmt.Fprint(rw, `{"code":"1","msg":"ok","out_trade_no":"RECPSTR","money":100.00,"status":"1"}`)
		case "RECNULL":
			fmt.Fprint(rw, `{"code":1,"msg":"ok","out_trade_no":"RECNULL","money":null,"status":1}`)
		case "RECWAIT":
			fmt.Fprint(rw, `{"code":1,"msg":"ok","out_trade_no":"RECWAIT","money":"100.00","status":0}`)
		case "RECBAMT":
			fmt.Fprint(rw, `{"code":1,"msg":"ok","out_trade_no":"RECBAMT","money":"1.00","status":1}`)
		default:
			fmt.Fprint(rw, `{"code":-1,"msg":"order not found"}`)
		}
	}))
	t.Cleanup(gateway.Close)
	operation_setting.PayAddress = gateway.URL

	summary, err := reconcileEpayPendingTopUpsOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, summary.Scanned, "grace-window order must not be scanned")
	assert.Equal(t, 3, summary.Settled)
	assert.Equal(t, 1, summary.Unpaid)
	assert.Equal(t, 1, summary.Failed, "amount mismatch counts as failed")

	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, "RECPAID"))
	assert.Equal(t, wantQuota, epayUserQuota(t, uPaid.Id))
	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, "RECPSTR"))
	assert.Equal(t, wantQuota, epayUserQuota(t, uPaidStr.Id))
	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, "RECNULL"))
	assert.Equal(t, wantQuota, epayUserQuota(t, uPaidNull.Id))
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, "RECWAIT"))
	assert.Equal(t, 0, epayUserQuota(t, uUnpaid.Id))
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, "RECBAMT"))
	assert.Equal(t, 0, epayUserQuota(t, uBadAmt.Id))
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, "RECNEW1"))
	assert.Equal(t, 0, epayUserQuota(t, uFresh.Id))
}

// The reconciler must query a SUB-SITE order's gateway with the SUB-SITE's OWN key/pid
// (not the global merchant), settle it, and atomically debit the agent wallet — the
// money-moving path. A drained wallet parks the order (manual_review) crediting nothing;
// an unresolvable pay_config is skipped (never free-credited).
func TestReconcileEpaySubSiteSettlesDebitsAndParks(t *testing.T) {
	setupEpayCallbackTest(t)
	wantQuota := int(10 * int64(common.QuotaPerUnit))
	wantCost := siteTopupCostMilli(100, model.DiscountRateBase) // 100000 厘

	gateway := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		require.Equal(t, "order", q.Get("act"))
		require.Equal(t, "1001", q.Get("pid"))
		// The reconciler MUST authenticate with the sub-site's OWN key, not the global one.
		require.Equal(t, subSiteEpayKey, q.Get("key"), "sub-site order must query with the sub-site key")
		require.NotEqual(t, epayTestMerchantKey, q.Get("key"))
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprint(rw, `{"code":1,"msg":"ok","out_trade_no":"`+q.Get("out_trade_no")+`","money":"100.00","status":1}`)
	}))
	t.Cleanup(gateway.Close)

	// Funded sub-site → settle + wallet debit.
	uPaid := createEpaySubSiteOrder(t, "SUBREC_OK", 8001, 300000, gateway.URL)
	// Drained sub-site → parked (manual_review), nothing credited.
	uPark := createEpaySubSiteOrder(t, "SUBREC_PARK", 8002, 100, gateway.URL)
	// Unresolvable pay_config (blank) → skipped, never settled for free.
	uSkip := createEpaySubSiteOrder(t, "SUBREC_SKIP", 8003, 300000, gateway.URL)
	require.NoError(t, model.DB.Model(&model.Site{}).Where("id = ?", 8003).Update("pay_config", "").Error)

	summary, err := reconcileEpayPendingTopUpsOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Scanned)
	assert.Equal(t, 1, summary.Settled)
	assert.Equal(t, 1, summary.Parked)
	assert.Equal(t, 1, summary.Skipped)

	// Funded: settled, credited, wallet debited by the wholesale cost.
	assert.Equal(t, common.TopUpStatusSuccess, epayOrderStatus(t, "SUBREC_OK"))
	assert.Equal(t, wantQuota, epayUserQuota(t, uPaid.Id))
	balOK, err := model.GetSiteWalletBalance(8001)
	require.NoError(t, err)
	assert.Equal(t, int64(300000)-wantCost, balOK, "reconcile must debit the agent wallet")

	// Drained: parked, not credited, wallet untouched.
	assert.Equal(t, model.TopUpStatusManualReview, epayOrderStatus(t, "SUBREC_PARK"))
	assert.Equal(t, 0, epayUserQuota(t, uPark.Id))
	balPark, err := model.GetSiteWalletBalance(8002)
	require.NoError(t, err)
	assert.Equal(t, int64(100), balPark)

	// Unresolvable config: skipped, order still pending, not credited.
	assert.Equal(t, common.TopUpStatusPending, epayOrderStatus(t, "SUBREC_SKIP"))
	assert.Equal(t, 0, epayUserQuota(t, uSkip.Id))
}
