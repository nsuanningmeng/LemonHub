package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Email campaign source.
const (
	EmailCampaignSourceManual       = "manual"
	EmailCampaignSourceAnnouncement = "announcement"
)

// Email campaign status.
const (
	EmailCampaignStatusPending   = "pending"
	EmailCampaignStatusSending   = "sending"
	EmailCampaignStatusCompleted = "completed"
	EmailCampaignStatusFailed    = "failed"
)

type EmailCampaign struct {
	Id           int    `json:"id"`
	SiteId       int    `json:"site_id" gorm:"type:int;default:0;index"`
	Subject      string `json:"subject" gorm:"type:varchar(255)"`
	Content      string `json:"content" gorm:"type:text"` // markdown source
	TargetGroup  string `json:"target_group" gorm:"type:varchar(64)"`
	TargetStatus int    `json:"target_status"` // 0=all, 1=enabled only
	Source       string `json:"source" gorm:"type:varchar(32);index"`
	Status       string `json:"status" gorm:"type:varchar(32);index"`
	TotalCount   int    `json:"total_count"`
	SentCount    int    `json:"sent_count"`
	FailCount    int    `json:"fail_count"`
	CreatedBy    int    `json:"created_by" gorm:"index"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	FinishedAt   int64  `json:"finished_at" gorm:"bigint"`
}

func (c *EmailCampaign) Insert() error {
	if c.CreatedAt == 0 {
		c.CreatedAt = common.GetTimestamp()
	}
	if c.Status == "" {
		c.Status = EmailCampaignStatusPending
	}
	if c.Source == "" {
		c.Source = EmailCampaignSourceManual
	}
	return DB.Create(c).Error
}

func GetEmailCampaignById(id int) (*EmailCampaign, error) {
	if id <= 0 {
		return nil, errors.New("invalid campaign id")
	}
	c := &EmailCampaign{}
	if err := DB.Where("id = ?", id).First(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

// GetEmailCampaigns lists campaigns for the admin console, newest first.
func GetEmailCampaigns(siteScope int, pageInfo *common.PageInfo) ([]*EmailCampaign, int64, error) {
	var list []*EmailCampaign
	var total int64
	query := DB.Model(&EmailCampaign{})
	if siteScope != SiteScopeAll {
		query = query.Where("site_id = ?", siteScope)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// UpdateProgress persists incremental send progress and status.
func (c *EmailCampaign) UpdateProgress() error {
	updates := map[string]interface{}{
		"status":      c.Status,
		"total_count": c.TotalCount,
		"sent_count":  c.SentCount,
		"fail_count":  c.FailCount,
		"finished_at": c.FinishedAt,
	}
	return DB.Model(&EmailCampaign{}).Where("id = ?", c.Id).Updates(updates).Error
}

// marketingRecipientQuery builds the shared filter for bulk-email candidate users:
// non-empty email, optional enabled-only status, optional group, and site scope.
// The MarketingEmailDisabled opt-out lives in the user Setting JSON and is filtered
// in Go by the caller (it cannot be expressed in SQL portably).
func marketingRecipientQuery(targetGroup string, targetStatus int, siteScope int) *gorm.DB {
	query := DB.Model(&User{}).Where("email != ''")
	if targetStatus == 1 {
		query = query.Where("status = ?", common.UserStatusEnabled)
	}
	if targetGroup != "" {
		query = query.Where(commonGroupCol+" = ?", targetGroup)
	}
	if siteScope != SiteScopeAll {
		query = query.Where("site_id = ?", siteScope)
	}
	return query
}

// CountMarketingCandidates returns the number of candidate recipients (before the
// in-Go opt-out filter), used to populate a campaign's TotalCount upper bound
// without materializing every user row.
func CountMarketingCandidates(targetGroup string, targetStatus int, siteScope int) (int64, error) {
	var total int64
	err := marketingRecipientQuery(targetGroup, targetStatus, siteScope).Count(&total).Error
	return total, err
}

// GetMarketingRecipientsBatch streams candidate users in keyset pages (id > afterId,
// ordered by id) so a large audience is never loaded into memory all at once.
func GetMarketingRecipientsBatch(targetGroup string, targetStatus int, siteScope int, afterId int, limit int) ([]User, error) {
	if limit <= 0 {
		limit = 500
	}
	var users []User
	err := marketingRecipientQuery(targetGroup, targetStatus, siteScope).
		Select("id", "username", "email", "role", "status", commonGroupCol, "setting", "site_id").
		Where("id > ?", afterId).
		Order("id asc").Limit(limit).Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

// CountActiveCampaigns counts pending/sending campaigns for a site, used to bound
// the number of concurrent bulk sends.
func CountActiveCampaigns(siteId int) (int64, error) {
	var count int64
	err := DB.Model(&EmailCampaign{}).
		Where("site_id = ? AND status IN ?", siteId, []string{EmailCampaignStatusPending, EmailCampaignStatusSending}).
		Count(&count).Error
	return count, err
}

// FindRecentDuplicateCampaign returns a campaign with identical subject+content
// created very recently by the same admin on the same site — an idempotency guard
// against accidental double-submits / client retries.
func FindRecentDuplicateCampaign(createdBy, siteId int, subject, content string, since int64) (*EmailCampaign, bool, error) {
	var list []*EmailCampaign
	err := DB.Where("created_by = ? AND site_id = ? AND subject = ? AND created_at >= ?", createdBy, siteId, subject, since).
		Order("id desc").Limit(5).Find(&list).Error
	if err != nil {
		return nil, false, err
	}
	for _, c := range list {
		if c.Content == content {
			return c, true, nil
		}
	}
	return nil, false, nil
}
