package controller

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// maxEmailCampaignContentBytes bounds campaign markdown under MySQL's TEXT limit.
const maxEmailCampaignContentBytes = 60000

// Bulk-send abuse guards.
const (
	// campaignDedupWindowSeconds is the window in which an identical (admin, site,
	// subject, content) campaign is treated as a duplicate submit.
	campaignDedupWindowSeconds = 60
	// maxActiveCampaignsPerSite caps concurrent pending/sending campaigns per site.
	maxActiveCampaignsPerSite = 2
)

// createEmailCampaignRequest is the admin-supplied body for launching a bulk email.
type createEmailCampaignRequest struct {
	Subject      string `json:"subject"`
	Content      string `json:"content"`       // markdown source
	TargetGroup  string `json:"target_group"`  // "" = all groups
	TargetStatus int    `json:"target_status"` // 0 = all, 1 = enabled only
}

// ListEmailCampaigns GET /api/email-campaign/ — paginated list, newest first,
// scoped to the operator's effective site.
func ListEmailCampaigns(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	campaigns, total, err := model.GetEmailCampaigns(middleware.EffectiveSiteScope(c), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(campaigns)
	common.ApiSuccess(c, pageInfo)
}

// CreateEmailCampaign POST /api/email-campaign/ — validate input, persist a pending
// campaign bound to the request site, then launch the send asynchronously. The created
// campaign (with id) is returned without blocking on delivery.
func CreateEmailCampaign(c *gin.Context) {
	var req createEmailCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	subject := strings.TrimSpace(req.Subject)
	content := strings.TrimSpace(req.Content)
	if subject == "" {
		common.ApiErrorMsg(c, "subject is required")
		return
	}
	if utf8.RuneCountInString(subject) > 255 {
		common.ApiErrorMsg(c, "subject too long (max 255 characters)")
		return
	}
	if content == "" {
		common.ApiErrorMsg(c, "content is required")
		return
	}
	// Bound content under MySQL's TEXT byte limit (65535) so it can never be
	// silently truncated on a MySQL backend regardless of SQL strict mode.
	if len(content) > maxEmailCampaignContentBytes {
		common.ApiErrorMsg(c, "content too long")
		return
	}
	if req.TargetStatus != 0 && req.TargetStatus != 1 {
		common.ApiErrorMsg(c, "invalid target_status")
		return
	}

	siteId := middleware.GetRequestSiteId(c)
	createdBy := c.GetInt("id")

	// Idempotency: an accidental double-submit or client retry (same admin + site +
	// subject + content within a short window) returns the existing campaign instead
	// of launching a duplicate full-audience blast.
	if dup, found, err := model.FindRecentDuplicateCampaign(createdBy, siteId, subject, content, common.GetTimestamp()-campaignDedupWindowSeconds); err == nil && found {
		common.ApiSuccess(c, dup)
		return
	}
	// Concurrency cap: bound simultaneous bulk sends per site so a burst of campaigns
	// cannot multiply the effective send rate and exhaust SMTP / harm sender reputation.
	if active, err := model.CountActiveCampaigns(siteId); err == nil && active >= maxActiveCampaignsPerSite {
		common.ApiErrorMsg(c, "another email campaign is already in progress, please wait for it to finish")
		return
	}

	campaign := &model.EmailCampaign{
		SiteId:       siteId,
		Subject:      subject,
		Content:      content,
		TargetGroup:  strings.TrimSpace(req.TargetGroup),
		TargetStatus: req.TargetStatus,
		Source:       model.EmailCampaignSourceManual,
		Status:       model.EmailCampaignStatusPending,
		CreatedBy:    createdBy,
	}
	if err := campaign.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}

	// Audit the bulk-send action. Only metadata is recorded — never the subject/body —
	// to avoid persisting potentially sensitive content in the audit log.
	recordManageAudit(c, "email_campaign.create", map[string]interface{}{
		"campaign_id":   campaign.Id,
		"target_group":  campaign.TargetGroup,
		"target_status": campaign.TargetStatus,
		"source":        campaign.Source,
	})

	// Launch send asynchronously; recipients are bounded by the operator's site scope.
	service.StartEmailCampaign(campaign, middleware.EffectiveSiteScope(c))

	common.ApiSuccess(c, campaign)
}

// GetEmailCampaignDetail GET /api/email-campaign/:id — return one campaign. Sub-site
// admins (scope != SiteScopeAll) may only read campaigns owned by their site.
func GetEmailCampaignDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	campaign, err := model.GetEmailCampaignById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	scope := middleware.EffectiveSiteScope(c)
	if scope != model.SiteScopeAll && campaign.SiteId != scope {
		common.ApiError(c, errors.New("campaign not found"))
		return
	}

	common.ApiSuccess(c, campaign)
}
