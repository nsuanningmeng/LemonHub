package controller

import (
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// Sub-site (white-label) self-service endpoints. Every handler below is gated by
// middleware.SiteAdminAuth (role >= RoleSubSiteAdmin), but the role gate alone provides NO
// tenant isolation: the authoritative scope is middleware.EffectiveSiteScope(c), which returns
// the operator's OWN account site_id (taken from session/token, never the request Host or body).
//
// Therefore the FIRST line of every handler is `siteId := middleware.EffectiveSiteScope(c)`
// followed by a `siteId <= 0` reject. This fails closed for:
//   - main-site admins / root  -> SiteScopeAll (-1)  -> rejected (they use /api/site instead)
//   - unidentified operators    -> SiteScopeDenied (-2) -> rejected
//   - legacy/unknown            -> 0                  -> rejected
//
// Only a genuine sub-site admin (siteId > 0) proceeds, and EVERY query/mutation is then forced
// onto that siteId — a sub-site admin can never read or write another tenant's data.

// requireSiteScope resolves the operator's own sub-site id and enforces that the caller is a
// scoped sub-site admin (siteId > 0). It writes a 403-semantic error and returns ok=false when
// the operator is not a legitimate sub-site admin, so callers just `if !ok { return }`.
func requireSiteScope(c *gin.Context) (int, bool) {
	siteId := middleware.EffectiveSiteScope(c)
	if siteId <= 0 {
		common.ApiErrorMsg(c, "无权访问")
		return 0, false
	}
	return siteId, true
}

// SiteAdminDashboard returns the operator's own sub-site overview (brand fields, wallet,
// discount, domains). Scope: EffectiveSiteScope -> GetSiteById(siteId).
func SiteAdminDashboard(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	site, err := model.GetSiteById(siteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":                    site.Id,
		"name":                  site.Name,
		"logo":                  site.Logo,
		"notice":                site.Notice,
		"footer":                site.Footer,
		"home_badge":            site.HomeBadge,
		"home_title_line1":      site.HomeTitleLine1,
		"home_title_line2":      site.HomeTitleLine2,
		"status":                site.Status,
		"discount_rate":         site.DiscountRate,
		"wallet_balance":        site.WalletBalance,
		"wallet_warn_threshold": site.WalletWarnThreshold,
		"domains":               site.Domains,
	})
}

// SiteAdminGetWalletLogs returns a page of the operator's own sub-site wallet flow records.
// Scope: GetSiteWalletLogs is forced to siteId; the optional ?type only narrows within the site.
func SiteAdminGetWalletLogs(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	logType, _ := strconv.Atoi(c.Query("type"))
	pageInfo := common.GetPageQuery(c)
	logs, total, err := model.GetSiteWalletLogs(siteId, logType, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

type siteAdminWarnThresholdRequest struct {
	Threshold int64 `json:"threshold"`
}

// SiteAdminSetWarnThreshold updates the low-balance alert threshold of the operator's own
// sub-site. Scope: SetSiteWalletWarnThreshold is forced to siteId (the request carries no id).
func SiteAdminSetWarnThreshold(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	var req siteAdminWarnThresholdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetSiteWalletWarnThreshold(siteId, req.Threshold); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site_admin.warn_threshold", map[string]interface{}{
		"id":        siteId,
		"threshold": req.Threshold,
	})
	common.ApiSuccess(c, nil)
}

// SiteAdminGetRedemptions lists the operator's own sub-site redemption codes. Scope: the third
// arg to GetAllRedemptions is the concrete siteId (NOT EffectiveSiteScope's SiteScopeAll), so the
// model adds `WHERE site_id = siteId`.
func SiteAdminGetRedemptions(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), siteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
}

// SiteAdminSearchRedemptions searches the operator's own sub-site redemption codes. Scope:
// SearchRedemptions is forced to siteId.
func SiteAdminSearchRedemptions(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), siteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
}

type siteAdminRedemptionRequest struct {
	Name        string `json:"name"`
	Quota       int    `json:"quota"`
	Count       int    `json:"count"`
	ExpiredTime int64  `json:"expired_time"`
}

// SiteAdminAddRedemption generates redemption codes for the operator's own sub-site, atomically
// debiting that site's procurement wallet (面值 × discount_rate). Scope: the discount rate comes
// from GetSiteById(siteId) and GenerateRedemptions is called with siteId, so codes and the wallet
// debit are bound to the operator's own tenant only.
func SiteAdminAddRedemption(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	var req siteAdminRedemptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if n := utf8.RuneCountInString(req.Name); n == 0 || n > 20 {
		common.ApiErrorMsg(c, "兑换码名称长度必须为 1-20 个字符")
		return
	}
	if req.Count < 1 || req.Count > 100 {
		common.ApiErrorMsg(c, "数量必须在 1-100 之间")
		return
	}
	if req.Quota <= 0 {
		common.ApiErrorMsg(c, "单码额度必须大于 0")
		return
	}
	if valid, msg := validateExpiredTime(c, req.ExpiredTime); !valid {
		common.ApiErrorMsg(c, msg)
		return
	}
	site, err := model.GetSiteById(siteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cost := calcRedemptionCostMilli(req.Quota, site.DiscountRate)
	keys, err := model.GenerateRedemptions(siteId, c.GetInt("id"), req.Name, req.Quota, req.Count, req.ExpiredTime, cost)
	if err != nil {
		if errors.Is(err, model.ErrInsufficientWalletBalance) {
			common.ApiErrorMsg(c, "子站钱包余额不足，无法生成兑换码")
			return
		}
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site_admin.redemption_create", map[string]interface{}{
		"name":  req.Name,
		"count": req.Count,
		"quota": logger.LogQuota(req.Quota),
	})
	common.ApiSuccess(c, keys)
}

// SiteAdminVoidRedemption voids one unused code and refunds its cost to the operator's own
// sub-site wallet. Scope: VoidRedemption(id, siteId, op) re-reads the row and rejects when
// r.SiteId != siteId, so a sub-site admin can never void another tenant's code by guessing its id.
func SiteAdminVoidRedemption(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.VoidRedemption(id, siteId, c.GetInt("id")); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site_admin.redemption_void", map[string]interface{}{"id": id})
	common.ApiSuccess(c, nil)
}

// SiteAdminExportRedemptions streams the operator's own sub-site redemption codes as CSV. Scope:
// GetAllRedemptions is called with siteId, so the export can only ever contain this tenant's rows.
func SiteAdminExportRedemptions(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	redemptions, _, err := model.GetAllRedemptions(0, 100000, siteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="redemptions.csv"`)
	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	if err := w.Write([]string{
		"id", "key", "name", "quota", "status", "cost_amount",
		"created_time", "redeemed_time", "used_user_id",
	}); err != nil {
		return
	}
	for _, r := range redemptions {
		if err := w.Write([]string{
			strconv.Itoa(r.Id),
			r.Key,
			r.Name,
			strconv.Itoa(r.Quota),
			strconv.Itoa(r.Status),
			strconv.FormatInt(r.CostAmount, 10),
			strconv.FormatInt(r.CreatedTime, 10),
			strconv.FormatInt(r.RedeemedTime, 10),
			strconv.Itoa(r.UsedUserId),
		}); err != nil {
			return
		}
	}
}

type siteAdminBrandingRequest struct {
	Name           string `json:"name"`
	Logo           string `json:"logo"`
	Notice         string `json:"notice"`
	Footer         string `json:"footer"`
	HomeBadge      string `json:"home_badge"`
	HomeTitleLine1 string `json:"home_title_line1"`
	HomeTitleLine2 string `json:"home_title_line2"`
}

// SiteAdminUpdateBranding updates ONLY the four brand fields (name/logo/notice/footer) of the
// operator's own sub-site. Scope: UpdateSiteBranding is forced to siteId and, by construction,
// can never touch domains/discount_rate/owner/status/pay_config — a sub-site admin cannot widen
// their own privileges or rebrand another tenant.
func SiteAdminUpdateBranding(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	var req siteAdminBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		common.ApiErrorMsg(c, "子站名称不能为空")
		return
	}
	if err := model.UpdateSiteBranding(siteId, name, req.Logo, req.Notice, req.Footer, strings.TrimSpace(req.HomeBadge), strings.TrimSpace(req.HomeTitleLine1), strings.TrimSpace(req.HomeTitleLine2)); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site_admin.branding_update", map[string]interface{}{
		"id":   siteId,
		"name": name,
	})
	common.ApiSuccess(c, nil)
}

type siteAdminPayConfigRequest struct {
	EpayId     string   `json:"epay_id"`
	EpayKey    string   `json:"epay_key"`
	PayAddress string   `json:"pay_address"`
	PayMethods []string `json:"pay_methods"`
}

// SiteAdminGetPayConfig returns the operator's own sub-site 收款 (epay) configuration so
// the agent can review/edit their own merchant credentials. Scope: GetSiteById(siteId).
func SiteAdminGetPayConfig(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	site, err := model.GetSiteById(siteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg, _ := parseSitePayConfig(site.PayConfig)
	common.ApiSuccess(c, gin.H{
		"epay_id":     cfg.EpayId,
		"epay_key":    cfg.EpayKey,
		"pay_address": cfg.PayAddress,
		"pay_methods": cfg.PayMethods,
	})
}

// SiteAdminUpdatePayConfig stores the operator's own sub-site 收款 configuration. Scope:
// UpdateSitePayConfig is forced to siteId — a sub-site admin can only configure their own.
func SiteAdminUpdatePayConfig(c *gin.Context) {
	siteId, ok := requireSiteScope(c)
	if !ok {
		return
	}
	var req siteAdminPayConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := sitePayConfig{
		EpayId:     strings.TrimSpace(req.EpayId),
		EpayKey:    strings.TrimSpace(req.EpayKey),
		PayAddress: strings.TrimSpace(req.PayAddress),
		PayMethods: req.PayMethods,
	}
	data, err := common.Marshal(cfg)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateSitePayConfig(siteId, string(data)); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site_admin.pay_config", map[string]interface{}{"id": siteId})
	common.ApiSuccess(c, nil)
}
