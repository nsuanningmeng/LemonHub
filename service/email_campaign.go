package service

import (
	"fmt"
	"html"
	"net/mail"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

// recipientBatchSize bounds how many user rows are loaded per keyset page while
// streaming a bulk send, capping peak memory regardless of audience size.
const recipientBatchSize = 500

// Recovery of campaigns orphaned by a crash/container restart.
const (
	// emailCampaignSweepInterval is how often a master node scans for interrupted
	// campaigns (status pending/sending with a stale progress timestamp).
	emailCampaignSweepInterval = 2 * time.Minute
	// emailCampaignStaleAfter is how long a pending/sending campaign must go without
	// a progress write before it is considered orphaned. It must comfortably exceed
	// the worst-case gap between progress writes on a healthy sender: one send delay
	// (<=60s at the minimum rate of 1/min) plus one full SMTP session (bounded by
	// common.SendEmail's connection deadline).
	emailCampaignStaleAfter = 15 * time.Minute
	// emailCampaignResumeMaxAge caps how long a campaign may have been interrupted
	// (measured from its last progress write, i.e. the last moment it was provably
	// alive) and still be resumed. Anything staler (a zombie from before resume
	// support existed — updated_at NULL/0 — or a node that stayed down for days) is
	// closed as failed instead: re-blasting stale marketing content long after the
	// fact is worse than dropping the tail.
	emailCampaignResumeMaxAge = 24 * time.Hour
	// maxConsecutivePersistFailures parks the send loop when the DB refuses this
	// many progress writes in a row. Sending blind would let the durable cursor lag
	// arbitrarily far behind reality (mass duplicates after a crash) and would let
	// updated_at go stale enough for the recovery sweep to start a second sender.
	maxConsecutivePersistFailures = 5
	// maxConsecutiveSendFailures parks the send loop when SMTP fails this many
	// sends in a row — a black-holed/broken server would otherwise grind through
	// the whole audience at one timeout per recipient, mailing nobody.
	maxConsecutiveSendFailures = 25
)

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
// failure transitions the campaign to a terminal status and persists it. It supports
// both fresh campaigns and resumed ones: counters and the keyset cursor come from the
// (possibly recovered) campaign row, so a resume continues where the crash happened.
func runEmailCampaign(campaign *model.EmailCampaign, siteScope int) {
	// 1. Mark as sending. This also refreshes updated_at, hiding the campaign from
	//    the orphan-recovery sweep while this goroutine owns it.
	campaign.Status = model.EmailCampaignStatusSending
	if err := campaign.UpdateProgress(); err != nil {
		common.SysError(fmt.Sprintf("email campaign %d: failed to mark sending: %s", campaign.Id, err.Error()))
	}

	// 2. Record a TotalCount upper bound via a cheap COUNT (candidates before the
	//    in-Go opt-out filter), so progress can be shown without loading all users.
	//    A resumed campaign keeps its original total so progress stays monotonic.
	if campaign.TotalCount == 0 {
		total, err := model.CountMarketingCandidates(campaign.TargetGroup, campaign.TargetStatus, siteScope)
		if err != nil {
			common.SysError(fmt.Sprintf("email campaign %d: failed to count recipients: %s", campaign.Id, err.Error()))
			campaign.Status = model.EmailCampaignStatusFailed
			campaign.FinishedAt = common.GetTimestamp()
			if perr := campaign.UpdateProgress(); perr != nil {
				common.SysError(fmt.Sprintf("email campaign %d: failed to persist failure: %s", campaign.Id, perr.Error()))
			}
			return
		}
		campaign.TotalCount = int(total)
		if err := campaign.UpdateProgress(); err != nil {
			common.SysError(fmt.Sprintf("email campaign %d: failed to persist total: %s", campaign.Id, err.Error()))
		}
	}

	// 3. Render the HTML body once (markdown is escaped + wrapped safely), with a
	//    fixed opt-out footer: bulk email must always tell recipients why they got
	//    it and where to turn it off (per-user toggle on the profile page).
	profileURL := html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/") + "/profile")
	safeSystemName := html.EscapeString(common.SystemName)
	footerHTML := fmt.Sprintf(
		`<div style="margin-top:24px;padding-top:16px;border-top:1px solid #e5e7eb;font-size:12px;color:#8a919f;">`+
			`<p>您收到此邮件是因为您在 %s 注册了账户。如不想再收到此类邮件,可前往<a href="%s" target="_blank" rel="noopener noreferrer">个人设置</a>关闭营销邮件。</p>`+
			`<p>You are receiving this email because you have an account on %s. You can turn off marketing emails in your <a href="%s" target="_blank" rel="noopener noreferrer">profile settings</a>.</p>`+
			`</div>`,
		safeSystemName, profileURL, safeSystemName, profileURL)
	bodyHTML := common.WrapEmailHTML(campaign.Subject, common.MarkdownToEmailHTML(campaign.Content)+footerHTML)

	// 4. Throttle: spread sends across the configured 封/分钟 rate.
	ratePerMin := operation_setting.GetEmailPromotionSetting().GetRatePerMinute()
	delay := time.Minute / time.Duration(ratePerMin)

	// persistProgress writes progress and tracks consecutive persist failures.
	// Returning false means the DB has been refusing writes for a while: the caller
	// must PARK the campaign (return, leaving status=sending) rather than keep
	// sending blind — an unpersisted cursor means mass duplicates after a crash, and
	// a stale updated_at would let the recovery sweep start a second sender.
	consecutivePersistFailures := 0
	persistProgress := func() bool {
		if err := campaign.UpdateProgress(); err != nil {
			consecutivePersistFailures++
			common.SysError(fmt.Sprintf("email campaign %d: failed to persist progress (%d consecutive): %s",
				campaign.Id, consecutivePersistFailures, err.Error()))
			return consecutivePersistFailures < maxConsecutivePersistFailures
		}
		consecutivePersistFailures = 0
		return true
	}

	// 5. Stream recipients in keyset batches so a large audience is never fully
	//    materialized; send one address per call (no recipient disclosure), skipping
	//    unsafe addresses and per-user marketing opt-outs. Progress (including the
	//    resume cursor) is persisted after every send: the write is trivially cheap
	//    next to the SMTP round trip and bounded by the send-rate cap, and it limits
	//    duplicate re-sends after a crash to at most one email.
	//
	//    Transient trouble (batch SELECT error, persistent progress-write failure,
	//    a run of consecutive SMTP failures) PARKS the campaign: the loop returns
	//    with status still "sending", and the recovery sweep resumes it from the
	//    durable cursor once it has been quiet for emailCampaignStaleAfter. It must
	//    never be finalized as completed on an error path — that would silently
	//    drop the remaining audience while reporting success.
	afterId := campaign.LastUserId
	consecutiveSendFailures := 0
	for {
		batch, err := model.GetMarketingRecipientsBatch(campaign.TargetGroup, campaign.TargetStatus, siteScope, afterId, recipientBatchSize)
		if err != nil {
			common.SysError(fmt.Sprintf("email campaign %d: failed to load recipient batch, parking for recovery sweep: %s", campaign.Id, err.Error()))
			return
		}
		if len(batch) == 0 {
			break
		}
		for i := range batch {
			user := batch[i]
			afterId = user.Id
			campaign.LastUserId = user.Id
			if !isSafeRecipient(user.Email) || user.GetSetting().MarketingEmailDisabled {
				continue
			}
			if err := common.SendEmail(campaign.Subject, user.Email, bodyHTML); err != nil {
				campaign.FailCount++
				consecutiveSendFailures++
				common.SysError(fmt.Sprintf("email campaign %d: failed to send to user %d: %s", campaign.Id, user.Id, err.Error()))
			} else {
				campaign.SentCount++
				consecutiveSendFailures = 0
			}
			if !persistProgress() {
				common.SysError(fmt.Sprintf("email campaign %d: parking after %d consecutive progress-persist failures", campaign.Id, consecutivePersistFailures))
				return
			}
			if consecutiveSendFailures >= maxConsecutiveSendFailures {
				common.SysError(fmt.Sprintf("email campaign %d: parking after %d consecutive send failures (SMTP down?)", campaign.Id, consecutiveSendFailures))
				return
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		// Persist at batch end as well, so the cursor advances over a trailing run of
		// skipped (opted-out/unsafe) users and updated_at stays fresh during long scans.
		if !persistProgress() {
			common.SysError(fmt.Sprintf("email campaign %d: parking after %d consecutive progress-persist failures", campaign.Id, consecutivePersistFailures))
			return
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

var (
	emailCampaignRecoveryOnce     sync.Once
	emailCampaignRecoverySweeping atomic.Bool
)

// StartEmailCampaignRecoveryTask starts the periodic sweep that resumes (or closes
// out) bulk-email campaigns orphaned by a crash or container restart. The send loop
// is an in-process goroutine with no durable queue, so without this sweep an
// interrupted campaign would stay "sending" forever, never reach its remaining
// recipients, and permanently consume one of the per-site active-campaign slots.
// Master-only, like the other singleton background tasks; the atomic DB claim keeps
// multi-master deployments from resuming the same campaign twice.
func StartEmailCampaignRecoveryTask() {
	emailCampaignRecoveryOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ticker := time.NewTicker(emailCampaignSweepInterval)
			defer ticker.Stop()
			recoverInterruptedEmailCampaigns()
			for range ticker.C {
				recoverInterruptedEmailCampaigns()
			}
		})
	})
}

// recoverInterruptedEmailCampaigns claims each stale pending/sending campaign and
// either resumes it from its persisted cursor or, when it is too old to resume
// safely, closes it as failed. Resumes run inline (sequentially) so several
// recovered campaigns cannot multiply the configured send rate; ticks that fire
// while a resume is still sending are skipped via the CAS guard.
func recoverInterruptedEmailCampaigns() {
	if !emailCampaignRecoverySweeping.CompareAndSwap(false, true) {
		return
	}
	defer emailCampaignRecoverySweeping.Store(false)

	staleBefore := common.GetTimestamp() - int64(emailCampaignStaleAfter/time.Second)
	stale, err := model.FindStaleActiveCampaigns(staleBefore)
	if err != nil {
		common.SysError(fmt.Sprintf("email campaign recovery: failed to list stale campaigns: %s", err.Error()))
		return
	}
	for _, campaign := range stale {
		claimed, err := model.ClaimStaleEmailCampaign(campaign.Id, staleBefore)
		if err != nil {
			common.SysError(fmt.Sprintf("email campaign recovery: failed to claim campaign %d: %s", campaign.Id, err.Error()))
			continue
		}
		if !claimed {
			continue // another node got there first, or the campaign came back to life
		}
		// campaign.UpdatedAt still holds the pre-claim value: the last moment the
		// campaign was provably alive. Age the interruption from there, NOT from
		// CreatedAt — a healthy multi-day campaign briefly interrupted must resume,
		// while one that has been dead for over a day (or a pre-upgrade zombie with
		// updated_at NULL/0) must not suddenly blast stale content.
		if common.GetTimestamp()-campaign.UpdatedAt > int64(emailCampaignResumeMaxAge/time.Second) {
			campaign.Status = model.EmailCampaignStatusFailed
			campaign.FinishedAt = common.GetTimestamp()
			if err := campaign.UpdateProgress(); err != nil {
				common.SysError(fmt.Sprintf("email campaign recovery: failed to close out campaign %d: %s", campaign.Id, err.Error()))
			}
			common.SysLog(fmt.Sprintf("email campaign recovery: campaign %d interrupted too long ago, closed as failed (sent=%d fail=%d total=%d)",
				campaign.Id, campaign.SentCount, campaign.FailCount, campaign.TotalCount))
			continue
		}
		common.SysLog(fmt.Sprintf("email campaign recovery: resuming campaign %d from user id %d (sent=%d fail=%d total=%d)",
			campaign.Id, campaign.LastUserId, campaign.SentCount, campaign.FailCount, campaign.TotalCount))
		runEmailCampaign(campaign, resumeSiteScope(campaign))
	}
}

// resumeSiteScope reconstructs the recipient scope a campaign was originally launched
// with. It must mirror the creation call sites exactly; today both use SiteScopeAll:
//   - manual campaigns are created behind AdminAuth (role >= RoleAdminUser), whose
//     EffectiveSiteScope is always SiteScopeAll;
//   - announcement campaigns mail all users because console announcements are a
//     single global option (RootAuth-managed), visible on every site's console.
//
// If a creation call site ever launches with a narrower scope, this mapping must
// learn to reproduce it (e.g. from a persisted scope column).
func resumeSiteScope(campaign *model.EmailCampaign) int {
	return model.SiteScopeAll
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
	// Console announcements are a single global option shown on every site's
	// console, so the email audience is all users — never just the users of
	// whichever domain the root admin happened to be browsing when saving.
	StartEmailCampaign(campaign, model.SiteScopeAll)
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
