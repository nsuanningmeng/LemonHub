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
	// LastUserId is the durable keyset cursor: the highest user id already processed
	// by the send loop. A campaign interrupted by a crash/restart resumes from here
	// instead of re-mailing the whole audience.
	LastUserId int   `json:"last_user_id"`
	CreatedBy  int   `json:"created_by" gorm:"index"`
	CreatedAt  int64 `json:"created_at" gorm:"bigint;index"`
	// UpdatedAt is refreshed on every progress persist; the recovery sweep uses its
	// staleness to distinguish an actively-sending campaign from an orphaned one.
	UpdatedAt  int64 `json:"updated_at" gorm:"bigint"`
	FinishedAt int64 `json:"finished_at" gorm:"bigint"`
}

func (c *EmailCampaign) Insert() error {
	if c.CreatedAt == 0 {
		c.CreatedAt = common.GetTimestamp()
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = c.CreatedAt
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

// UpdateProgress persists incremental send progress and status. It also advances
// the resume cursor and refreshes updated_at so the recovery sweep never mistakes
// a live campaign for an orphaned one.
func (c *EmailCampaign) UpdateProgress() error {
	c.UpdatedAt = common.GetTimestamp()
	updates := map[string]interface{}{
		"status":       c.Status,
		"total_count":  c.TotalCount,
		"sent_count":   c.SentCount,
		"fail_count":   c.FailCount,
		"last_user_id": c.LastUserId,
		"updated_at":   c.UpdatedAt,
		"finished_at":  c.FinishedAt,
	}
	return DB.Model(&EmailCampaign{}).Where("id = ?", c.Id).Updates(updates).Error
}

// FindStaleActiveCampaigns returns campaigns stuck in pending/sending whose last
// progress write predates staleBefore — i.e. campaigns orphaned by a crash or
// container restart (a healthy sender persists progress after every send).
// Rows created before the updated_at column existed have NULL there; they are
// zombies by definition, so NULL matches too.
func FindStaleActiveCampaigns(staleBefore int64) ([]*EmailCampaign, error) {
	var list []*EmailCampaign
	err := DB.Where("status IN ? AND (updated_at IS NULL OR updated_at < ?)",
		[]string{EmailCampaignStatusPending, EmailCampaignStatusSending}, staleBefore).
		Order("id asc").Find(&list).Error
	return list, err
}

// ClaimStaleEmailCampaign atomically takes ownership of an orphaned campaign. The
// status+updated_at guard in the WHERE clause makes the claim safe against
// concurrent recovery sweeps on other nodes: at most one conditional UPDATE can
// match, and the refreshed updated_at immediately hides the row from later sweeps.
func ClaimStaleEmailCampaign(id int, staleBefore int64) (bool, error) {
	res := DB.Model(&EmailCampaign{}).
		Where("id = ? AND status IN ? AND (updated_at IS NULL OR updated_at < ?)",
			id, []string{EmailCampaignStatusPending, EmailCampaignStatusSending}, staleBefore).
		Updates(map[string]interface{}{
			"status":     EmailCampaignStatusSending,
			"updated_at": common.GetTimestamp(),
		})
	return res.RowsAffected == 1, res.Error
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

// CountActiveCampaigns counts all pending/sending campaigns, used to bound the
// number of concurrent bulk sends. The cap is global, not per-site: every campaign
// drains the same SMTP account at the full configured rate, so N concurrent
// campaigns multiply the effective send rate N-fold no matter which site owns them.
func CountActiveCampaigns() (int64, error) {
	var count int64
	err := DB.Model(&EmailCampaign{}).
		Where("status IN ?", []string{EmailCampaignStatusPending, EmailCampaignStatusSending}).
		Count(&count).Error
	return count, err
}

// FindRecentDuplicateCampaign returns a campaign with identical subject, content
// and audience created very recently by the same admin — an idempotency guard
// against accidental double-submits / client retries. The audience (group+status)
// is part of the identity: the same content deliberately sent to a second group
// moments later is a new campaign, not a duplicate. Site is NOT part of the
// identity — a manual campaign's audience does not depend on the request host.
func FindRecentDuplicateCampaign(createdBy int, subject, content, targetGroup string, targetStatus int, since int64) (*EmailCampaign, bool, error) {
	var list []*EmailCampaign
	err := DB.Where("created_by = ? AND subject = ? AND created_at >= ?", createdBy, subject, since).
		Order("id desc").Limit(5).Find(&list).Error
	if err != nil {
		return nil, false, err
	}
	for _, c := range list {
		if c.Content == content && c.TargetGroup == targetGroup && c.TargetStatus == targetStatus {
			return c, true, nil
		}
	}
	return nil, false, nil
}
