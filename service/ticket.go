package service

import (
	"fmt"
	"html"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

// NotifyAdminsOfTicket notifies platform admins about ticket activity (a newly
// created ticket or a new user reply) when admin notifications are enabled in
// the ticket settings.
//
// It is intended to be invoked from gopool.Go by the request handlers so it must
// never block the request path and must tolerate lookup errors silently. The
// simplest robust delivery is a single notification to the root user, which the
// platform's existing notify pipeline fans out according to the root user's
// configured channel (email/webhook/etc.).
func NotifyAdminsOfTicket(ticket *model.Ticket, action string) {
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

	// User-controlled fields are HTML-escaped because the notification is delivered
	// as an HTML email body (common.SendEmail), preventing stored HTML/phishing
	// injection from a crafted ticket title.
	content := fmt.Sprintf("Ticket #%d [%s] \"%s\" from %s",
		ticket.Id, html.EscapeString(ticket.Type), html.EscapeString(ticket.Title), html.EscapeString(submitter))
	NotifyRootUser("ticket_"+action, subject, content)
}
