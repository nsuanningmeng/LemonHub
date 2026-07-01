package service

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/setting/ticket_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	// ticketCleanupInterval is how often the background attachment sweep runs.
	ticketCleanupInterval = 6 * time.Hour
	// orphanUploadMaxAgeSeconds is how long an uploaded-but-never-attached file is
	// kept before the sweep reclaims it (a user may upload then abandon the form).
	orphanUploadMaxAgeSeconds = 24 * 3600
)

var ticketCleanupOnce sync.Once

// StartTicketAttachmentCleanupTask launches a master-node-only periodic sweep that
// reclaims (a) orphaned uploads never bound to a message and (b) — when
// ticket_setting.AttachmentRetentionDays > 0 — attachments of long-closed tickets,
// to bound storage growth. This complements the admin's manual cleanup endpoint.
func StartTicketAttachmentCleanupTask() {
	ticketCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			common.SysLog(fmt.Sprintf("ticket cleanup task started: tick=%s", ticketCleanupInterval))
			ticker := time.NewTicker(ticketCleanupInterval)
			defer ticker.Stop()
			runTicketCleanupOnce()
			for range ticker.C {
				runTicketCleanupOnce()
			}
		})
	})
}

// runTicketCleanupOnce performs one cleanup pass across all sites: it reclaims
// orphaned/closed-ticket attachments and, when configured, purges long-closed
// tickets and their messages. It is defensive: errors are logged and never panic
// the background goroutine.
func runTicketCleanupOnce() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(fmt.Sprintf("ticket cleanup panicked: %v", r))
		}
	}()

	now := common.GetTimestamp()
	setting := ticket_setting.GetTicketSetting()

	// 1. Attachment reclamation: orphaned uploads + (optionally) closed-ticket attachments.
	orphanBefore := now - orphanUploadMaxAgeSeconds
	var attClosedBefore int64
	if days := setting.AttachmentRetentionDays; days > 0 {
		attClosedBefore = now - int64(days)*86400
	}
	if attachments, err := model.CollectAttachmentsForCleanup(model.SiteScopeAll, orphanBefore, attClosedBefore); err != nil {
		common.SysError(fmt.Sprintf("ticket cleanup: collect attachments failed: %s", err.Error()))
	} else if len(attachments) > 0 {
		removed := deleteAttachmentFilesAndRows(attachments)
		common.SysLog(fmt.Sprintf("ticket cleanup: removed %d attachment(s)", removed))
	}

	// 2. Closed-ticket purge: remove old closed tickets + their messages + attachments.
	if days := setting.ClosedTicketRetentionDays; days > 0 {
		purgeClosedTickets(now - int64(days)*86400)
	}
}

// deleteAttachmentFilesAndRows removes the on-disk files then the DB rows for the
// given attachments, returning how many were fully removed.
func deleteAttachmentFilesAndRows(attachments []*model.TicketAttachment) int {
	deletedIds := make([]int, 0, len(attachments))
	for _, a := range attachments {
		if err := common.DeleteUpload(a.FilePath); err != nil {
			common.SysError(fmt.Sprintf("ticket cleanup: delete file %d failed: %s", a.Id, err.Error()))
			continue
		}
		deletedIds = append(deletedIds, a.Id)
	}
	if len(deletedIds) > 0 {
		if _, err := model.DeleteAttachmentRows(deletedIds); err != nil {
			common.SysError(fmt.Sprintf("ticket cleanup: delete attachment rows failed: %s", err.Error()))
		}
	}
	return len(deletedIds)
}

// purgeClosedTickets deletes tickets closed before the cutoff along with their
// messages and any remaining attachments.
func purgeClosedTickets(closedBefore int64) {
	ticketIds, err := model.GetClosedTicketIdsBefore(model.SiteScopeAll, closedBefore, 0)
	if err != nil {
		common.SysError(fmt.Sprintf("ticket cleanup: collect closed tickets failed: %s", err.Error()))
		return
	}
	if len(ticketIds) == 0 {
		return
	}
	if atts, err := model.GetAttachmentsByTicketIds(ticketIds); err == nil && len(atts) > 0 {
		deleteAttachmentFilesAndRows(atts)
	}
	msgs, _ := model.DeleteMessagesByTicketIds(ticketIds)
	tks, _ := model.DeleteTicketsByIds(ticketIds)
	common.SysLog(fmt.Sprintf("ticket cleanup: purged %d closed ticket(s), %d message(s)", tks, msgs))
}

// maxTicketNotifyExcerptRunes bounds how much of a message body is embedded in a
// notification email so the mail stays small; the full content is always readable
// in-app. Measured in runes so multi-byte (CJK) content is truncated cleanly.
const maxTicketNotifyExcerptRunes = 1200

// ticketMessageExcerptHTML renders a (user- or admin-authored) message body as a
// safe HTML fragment for an email notification: the content is HTML-escaped to
// neutralize stored HTML/script injection, newlines become <br> so the message
// stays readable, and overly long bodies are truncated with a notice.
func ticketMessageExcerptHTML(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "<p><em>(no text content)</em></p>"
	}
	truncated := false
	if runes := []rune(content); len(runes) > maxTicketNotifyExcerptRunes {
		content = string(runes[:maxTicketNotifyExcerptRunes])
		truncated = true
	}
	escaped := html.EscapeString(content)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	body := fmt.Sprintf("<blockquote style=\"margin:8px 0;padding:8px 12px;border-left:3px solid #ccc;color:#333;\">%s</blockquote>", escaped)
	if truncated {
		body += "<p><em>(content truncated — log in to view the full message)</em></p>"
	}
	return body
}

// ticketLinkHTML returns a "view in console" link line when the server address is
// configured, otherwise an empty string. adminView selects the admin vs. user
// ticket console path. ServerAddress is admin-configured (trusted), so it is not
// escaped, matching the rest of the email pipeline (see controller/misc.go).
func ticketLinkHTML(adminView bool) string {
	if path := ticketConsolePath(adminView); path != "" {
		// Escape the URL for the HTML attribute context (defense-in-depth: the base
		// is trusted admin config, but a stray quote would otherwise break out of
		// the href attribute).
		return fmt.Sprintf("<p><a href=\"%s\">View in console</a></p>", html.EscapeString(path))
	}
	return ""
}

// ticketMessageExcerptPlain renders a message body for plain-text notification
// channels (webhook/bark/gotify): trimmed and truncated, with real newlines and
// no markup. Kept separate from the HTML variant so those channels never receive
// raw <br>/<blockquote> tags.
func ticketMessageExcerptPlain(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "(no text content)"
	}
	if runes := []rune(content); len(runes) > maxTicketNotifyExcerptRunes {
		content = string(runes[:maxTicketNotifyExcerptRunes]) +
			"\n(content truncated — log in to view the full message)"
	}
	return content
}

// ticketLinkPlain returns a plain-text "View: <url>" line, or empty when the
// server address is not configured.
func ticketLinkPlain(adminView bool) string {
	if path := ticketConsolePath(adminView); path != "" {
		return "\nView: " + path
	}
	return ""
}

// ticketConsolePath builds the absolute console URL for the ticket list, or ""
// when the server address is unusable. adminView selects the admin path. Only an
// absolute http(s) base is accepted, so an empty, relative, or non-http-scheme
// (e.g. javascript:) ServerAddress never yields a malformed or dangerous link.
func ticketConsolePath(adminView bool) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if !isHTTPURL(base) {
		return ""
	}
	if adminView {
		return base + "/console/tickets-admin"
	}
	return base + "/console/tickets"
}

// isHTTPURL reports whether s begins with an http:// or https:// scheme.
func isHTTPURL(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// NotifyAdminsOfTicket notifies platform admins about ticket activity (a newly
// created ticket or a new user reply) when admin notifications are enabled in
// the ticket settings. messageContent is the body of the triggering message
// (the opening message on "created", the user's reply on "reply") and is embedded
// in the email so admins can triage without opening the console first.
//
// It is intended to be invoked from gopool.Go by the request handlers so it must
// never block the request path and must tolerate lookup errors silently. The
// simplest robust delivery is a single notification to the root user, which the
// platform's existing notify pipeline fans out according to the root user's
// configured channel (email/webhook/etc.).
func NotifyAdminsOfTicket(ticket *model.Ticket, action string, messageContent string) {
	if ticket == nil {
		return
	}
	if !ticket_setting.GetTicketSetting().AdminNotifyEnabled {
		return
	}

	submitter := fmt.Sprintf("user #%d", ticket.UserId)
	if u, err := model.GetUserById(ticket.UserId, false); err == nil && u != nil && u.Username != "" {
		submitter = u.Username
	}

	var subject string
	switch action {
	case "created":
		subject = "New support ticket submitted"
	case "reply":
		subject = "New reply on a support ticket"
	default:
		subject = "Support ticket update"
	}

	// Plain-text body for non-email channels (webhook/bark/gotify), which do not
	// render HTML. Kept tag-free so recipients see clean text.
	plain := fmt.Sprintf("Ticket #%d [%s] \"%s\" from %s\n\n%s%s",
		ticket.Id, ticket.Type, ticket.Title, submitter,
		ticketMessageExcerptPlain(messageContent), ticketLinkPlain(true))

	// HTML body used only on the email channel. User-controlled fields are
	// HTML-escaped because the mail is delivered as an HTML body (common.SendEmail),
	// preventing stored HTML/phishing injection from a crafted title or message.
	htmlBody := fmt.Sprintf("<p>Ticket #%d [%s] \"%s\" from %s</p>",
		ticket.Id, html.EscapeString(ticket.Type), html.EscapeString(ticket.Title), html.EscapeString(submitter))
	htmlBody += ticketMessageExcerptHTML(messageContent)
	htmlBody += ticketLinkHTML(true)

	NotifyRootUserWithEmailContent("ticket_"+action, subject, plain, htmlBody)
}

// NotifyTicketOwnerReply emails the ticket owner that an admin replied, embedding
// the reply text (escaped) so the notification is actionable on its own instead
// of being a bare "you have a new reply" ping. Safe to call from gopool.Go: it
// never blocks the request path and tolerates lookup errors silently.
func NotifyTicketOwnerReply(ticket *model.Ticket, replyContent string) {
	if ticket == nil {
		return
	}
	owner, err := model.GetUserById(ticket.UserId, false)
	if err != nil || owner == nil {
		return
	}
	// Plain-text body for non-email channels (webhook/bark/gotify).
	plain := fmt.Sprintf("Your ticket \"%s\" (#%d) has a new reply:\n\n%s%s",
		ticket.Title, ticket.Id, ticketMessageExcerptPlain(replyContent), ticketLinkPlain(false))

	// HTML body used only on the email channel. The user-set title is escaped
	// because the mail is delivered as an HTML body.
	htmlBody := fmt.Sprintf("<p>Your ticket \"%s\" (#%d) has a new reply:</p>",
		html.EscapeString(ticket.Title), ticket.Id)
	htmlBody += ticketMessageExcerptHTML(replyContent)
	htmlBody += ticketLinkHTML(false)

	notify := dto.NewNotify("ticket_reply", "Your ticket has a new reply", plain, nil)
	notify.EmailContent = htmlBody
	if notifyErr := NotifyUser(owner.Id, owner.Email, owner.GetSetting(), notify); notifyErr != nil {
		common.SysLog("failed to notify ticket owner: " + notifyErr.Error())
	}
}
