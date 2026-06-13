package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// SiteRequest is the create/update payload for a sub-site (white-label tenant).
type SiteRequest struct {
	Id                  int      `json:"id"`
	Name                string   `json:"name"`
	Logo                string   `json:"logo"`
	Notice              string   `json:"notice"`
	Footer              string   `json:"footer"`
	OwnerUsername       string   `json:"owner_username"`
	Status              int      `json:"status"`
	DiscountRate        int      `json:"discount_rate"`
	WalletWarnThreshold int64    `json:"wallet_warn_threshold"`
	PayConfig           string   `json:"pay_config"`
	Domains             []string `json:"domains"`
}

func GetAllSites(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	sites, total, err := model.GetAllSites(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(sites)
	common.ApiSuccess(c, pageInfo)
}

func SearchSites(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	sites, total, err := model.SearchSites(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(sites)
	common.ApiSuccess(c, pageInfo)
}

func GetSite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	site, err := model.GetSiteById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, site)
}

// validateSiteRequest enforces field-level constraints shared by create/update.
func validateSiteRequest(req *SiteRequest, requireOwner bool) (string, bool) {
	if strings.TrimSpace(req.Name) == "" {
		return "子站名称不能为空", false
	}
	if len(req.Domains) == 0 {
		return "至少需要绑定一个域名", false
	}
	if requireOwner && strings.TrimSpace(req.OwnerUsername) == "" {
		return "归属代理账号不能为空", false
	}
	// DiscountRate: 0 means "use default (no discount)"; otherwise must be within (0, 10000].
	if req.DiscountRate < 0 || req.DiscountRate > model.DiscountRateBase {
		return "折扣率必须在 1 到 10000 之间（10000 表示原价）", false
	}
	if req.WalletWarnThreshold < 0 {
		return "钱包警戒线不能为负", false
	}
	return "", true
}

func AddSite(c *gin.Context) {
	var req SiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if msg, ok := validateSiteRequest(&req, true); !ok {
		common.ApiErrorMsg(c, msg)
		return
	}

	site := &model.Site{
		Name:                strings.TrimSpace(req.Name),
		Logo:                req.Logo,
		Notice:              req.Notice,
		Footer:              req.Footer,
		OwnerUsername:       strings.TrimSpace(req.OwnerUsername),
		Status:              req.Status,
		DiscountRate:        req.DiscountRate,
		WalletWarnThreshold: req.WalletWarnThreshold,
		PayConfig:           req.PayConfig,
		Domains:             req.Domains,
	}
	if err := model.CreateSite(site); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site.create", map[string]interface{}{
		"name": site.Name,
		"id":   site.Id,
	})

	created, err := model.GetSiteById(site.Id)
	if err != nil {
		common.ApiSuccess(c, site)
		return
	}
	common.ApiSuccess(c, created)
}

func UpdateSite(c *gin.Context) {
	var req SiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Id == 0 {
		common.ApiErrorMsg(c, "无效的子站 ID")
		return
	}
	// Owner is optional on update (empty keeps the existing owner).
	if msg, ok := validateSiteRequest(&req, false); !ok {
		common.ApiErrorMsg(c, msg)
		return
	}

	site := &model.Site{
		Id:                  req.Id,
		Name:                strings.TrimSpace(req.Name),
		Logo:                req.Logo,
		Notice:              req.Notice,
		Footer:              req.Footer,
		OwnerUsername:       strings.TrimSpace(req.OwnerUsername),
		Status:              req.Status,
		DiscountRate:        req.DiscountRate,
		WalletWarnThreshold: req.WalletWarnThreshold,
		PayConfig:           req.PayConfig,
		Domains:             req.Domains,
	}
	if err := model.UpdateSite(site); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site.update", map[string]interface{}{
		"name": site.Name,
		"id":   site.Id,
	})

	updated, err := model.GetSiteById(site.Id)
	if err != nil {
		common.ApiSuccess(c, site)
		return
	}
	common.ApiSuccess(c, updated)
}

func DeleteSite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteSite(id); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "site.delete", map[string]interface{}{
		"id": id,
	})
	common.ApiSuccess(c, nil)
}
