package service

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

// progressPersistInterval is how many sends happen between intermediate progress
// persists, so a long campaign reflects progress without hammering the DB.
const progressPersistInterval = 25

// recipientBatchSize bounds how many user rows are loaded per keyset page while
// streaming a bulk send, capping peak memory regardless of audience size.
const recipientBatchSize = 500

// isSafeRecipient defends the bulk-send path against header injection and
// unintended multi-recipient delivery. common.SendEmail writes the address into a
// raw "To:" header and splits on ';', so a single malformed/legacy DB value could
// otherwise inject headers or fan out to multiple addresses. We require a single,
// RFC-5322-parseable address with no CR/LF or ';'.
func isSafeRecipient(email string) bool {
	if email == "" {
		return false
	}
	if strings.ContainsAny(email, ";\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	// Reject "Name <addr>" forms — only the bare address is acceptable here.
	return addr.Address == email
}

// StartEmailCampaign launches the bulk send for an already-persisted campaign in a
// background goroutine and returns immediately. The caller (controller) does NOT
// block on delivery. siteScope is the EffectiveSiteScope of the operator and bounds
// the recipient query so a sub-site admin can only mail their own users.
func StartEmailCampaign(campaign *model.EmailCampaign, siteScope int) {
	if campaign == nil || campaign.Id <= 0 {
		common.SysError("StartEmailCampaign called with an unsaved campaign")
		return
	}
	gopool.Go(func() {
		runEmailCampaign(campaign, siteScope)
	})
}

// runEmailCampaign performs the actual send loop. It is intentionally defensive: any
// failure transitions the campaign to a terminal status and persists it.
func runEmailCampaign(campaign *model.EmailCampaign, siteScope int) {
	// 1. Mark as sending.
	campaign.Status = model.EmailCampaignStatusSending
	if err := campaign.UpdateProgress(); err != nil {
		common.SysError(fmt.Sprintf("email campaign %d: failed to mark sending: %s", campaign.Id, err.Error()))
	}

	// 2. Record a TotalCount upper bound via a cheap COUNT (candidates before the
	//    in-Go opt-out filter), so progress can be shown without loading all users.
	if total, err := model.CountMarketingCandidates(campaign.TargetGroup, campaign.TargetStatus, siteScope); err != nil {
		common.SysError(fmt.Sprintf("email campaign %d: failed to count recipients: %s", campaign.Id, err.Error()))
		campaign.Status = model.EmailCampaignStatusFailed
		campaign.FinishedAt = common.GetTimestamp()
		if perr := campaign.UpdateProgress(); perr != nil {
			common.SysError(fmt.Sprintf("email campaign %d: failed to persist failure: %s", campaign.Id, perr.Error()))
		}
		return
	} else {
		campaign.TotalCount = int(total)
		if err := campaign.UpdateProgress(); err != nil {
			common.SysError(fmt.Sprintf("email campaign %d: failed to persist total: %s", campaign.Id, err.Error()))
		}
	}

	// 3. Render the HTML body once (markdown is escaped + wrapped safely).
	bodyHTML := common.WrapEmailHTML(campaign.Subject, common.MarkdownToEmailHTML(campaign.Content))

	// 4. Throttle: spread sends across the configured 封/分钟 rate.
	ratePerMin := operation_setting.GetEmailPromotionSetting().GetRatePerMinute()
	delay := time.Minute / time.Duration(ratePerMin)

	// 5. Stream recipients in keyset batches so a large audience is never fully
	//    materialized; send one address per call (no recipient disclosure), skipping
	//    unsafe addresses and per-user marketing opt-outs.
	afterId := 0
	for {
		batch, err := model.GetMarketingRecipientsBatch(campaign.TargetGroup, campaign.TargetStatus, siteScope, afterId, recipientBatchSize)
		if err != nil {
			common.SysError(fmt.Sprintf("email campaign %d: failed to load recipient batch: %s", campaign.Id, err.Error()))
			break
		}
		if len(batch) == 0 {
			break
		}
		for i := range batch {
			user := batch[i]
			afterId = user.Id
			if !isSafeRecipient(user.Email) || user.GetSetting().MarketingEmailDisabled {
				continue
			}
			if err := common.SendEmail(campaign.Subject, user.Email, bodyHTML); err != nil {
				campaign.FailCount++
				common.SysError(fmt.Sprintf("email campaign %d: failed to send to user %d: %s", campaign.Id, user.Id, err.Error()))
			} else {
				campaign.SentCount++
			}

			processed := campaign.SentCount + campaign.FailCount
			if processed%progressPersistInterval == 0 {
				if err := campaign.UpdateProgress(); err != nil {
					common.SysError(fmt.Sprintf("email campaign %d: failed to persist progress: %s", campaign.Id, err.Error()))
				}
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		if len(batch) < recipientBatchSize {
			break
		}
	}

	// 7. Finalize.
	if campaign.FailCount > 0 && campaign.SentCount == 0 {
		campaign.Status = model.EmailCampaignStatusFailed
	} else {
		campaign.Status = model.EmailCampaignStatusCompleted
	}
	campaign.FinishedAt = common.GetTimestamp()
	if err := campaign.UpdateProgress(); err != nil {
		common.SysError(fmt.Sprintf("email campaign %d: failed to persist final status: %s", campaign.Id, err.Error()))
	}
	common.SysLog(fmt.Sprintf("email campaign %d finished: status=%s total=%d sent=%d fail=%d",
		campaign.Id, campaign.Status, campaign.TotalCount, campaign.SentCount, campaign.FailCount))
}

// announcementItem is the subset of a console announcement object we care about for
// diffing newly-added announcements. The JSON id is a number; decode into float64
// (default for JSON numbers into any) is handled separately via the raw map.
type announcementItem struct {
	Id          float64 `json:"id"`
	Content     string  `json:"content"`
	PublishDate string  `json:"publishDate"`
	Type        string  `json:"type"`
	Extra       string  `json:"extra"`
}

// MaybeSendAnnouncementEmails is invoked by the announcement-save hook (the integrator
// wraps this in gopool.Go after a successful console_setting.announcements update). It
// diffs old vs new announcements, and for any newly-added announcement creates an
// "announcement" campaign that mails enabled users (TargetStatus=1, all groups).
//
// It is defensive: malformed/empty old JSON is treated as an empty list, parse errors
// abort quietly, and it never panics.
func MaybeSendAnnouncementEmails(oldJSON, newJSON string, createdBy, siteId int) {
	if !operation_setting.GetEmailPromotionSetting().AnnouncementEmailEnabled {
		return
	}

	newItems, err := parseAnnouncements(newJSON)
	if err != nil {
		common.SysLog(fmt.Sprintf("MaybeSendAnnouncementEmails: failed to parse new announcements: %s", err.Error()))
		return
	}
	if len(newItems) == 0 {
		return
	}
	// Tolerate empty/invalid old JSON: treat as an empty list (everything is "added").
	oldItems, err := parseAnnouncements(oldJSON)
	if err != nil {
		common.SysLog(fmt.Sprintf("MaybeSendAnnouncementEmails: ignoring unparseable old announcements: %s", err.Error()))
		oldItems = nil
	}

	added := diffAddedAnnouncements(oldItems, newItems)
	if len(added) == 0 {
		return
	}

	// One campaign covering all newly-added announcements.
	subject := announcementSubject(added)
	content := announcementMarkdown(added)

	campaign := &model.EmailCampaign{
		SiteId:       siteId,
		Subject:      subject,
		Content:      content,
		TargetGroup:  "",
		TargetStatus: 1,
		Source:       model.EmailCampaignSourceAnnouncement,
		Status:       model.EmailCampaignStatusPending,
		CreatedBy:    createdBy,
	}
	if err := campaign.Insert(); err != nil {
		common.SysError(fmt.Sprintf("MaybeSendAnnouncementEmails: failed to insert campaign: %s", err.Error()))
		return
	}
	common.SysLog(fmt.Sprintf("MaybeSendAnnouncementEmails: created announcement campaign %d for %d new announcement(s)", campaign.Id, len(added)))
	StartEmailCampaign(campaign, siteId)
}

// parseAnnouncements decodes a console announcements JSON array. Empty/blank input
// yields an empty slice with no error.
func parseAnnouncements(raw string) ([]announcementItem, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var items []announcementItem
	if err := common.UnmarshalJsonStr(trimmed, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// diffAddedAnnouncements returns announcements present in newItems but not in oldItems.
// It matches by id when both sides carry a non-zero id, falling back to (content +
// publishDate) when ids are unreliable (e.g. 0/missing).
func diffAddedAnnouncements(oldItems, newItems []announcementItem) []announcementItem {
	oldIds := make(map[float64]bool, len(oldItems))
	oldKeys := make(map[string]bool, len(oldItems))
	for _, it := range oldItems {
		if it.Id != 0 {
			oldIds[it.Id] = true
		}
		oldKeys[contentKey(it)] = true
	}

	var added []announcementItem
	for _, it := range newItems {
		if it.Id != 0 {
			if oldIds[it.Id] {
				continue
			}
		} else if oldKeys[contentKey(it)] {
			continue
		}
		added = append(added, it)
	}
	return added
}

func contentKey(it announcementItem) string {
	return it.Content + "\x00" + it.PublishDate
}

// announcementSubject derives a concise subject from the first added announcement's
// first non-empty line, capped to a reasonable length.
func announcementSubject(added []announcementItem) string {
	const fallback = "New Announcement"
	if len(added) == 0 {
		return fallback
	}
	for _, line := range strings.Split(added[0].Content, "\n") {
		line = strings.TrimSpace(line)
		// Strip a leading markdown heading marker for a cleaner subject.
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line == "" {
			continue
		}
		if len([]rune(line)) > 120 {
			line = string([]rune(line)[:120])
		}
		return line
	}
	return fallback
}

// announcementMarkdown concatenates the markdown content of all added announcements.
func announcementMarkdown(added []announcementItem) string {
	parts := make([]string, 0, len(added))
	for _, it := range added {
		body := strings.TrimSpace(it.Content)
		if body == "" {
			continue
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n---\n\n")
}
