package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type TicketAttachment struct {
	Id         int    `json:"id"`
	SiteId     int    `json:"site_id" gorm:"type:int;default:0;index"`
	TicketId   int    `json:"ticket_id" gorm:"index"`  // 0 until bound to a message
	MessageId  int    `json:"message_id" gorm:"index"` // 0 until bound to a message
	UserId     int    `json:"user_id" gorm:"index"`    // uploader, authoritative for ownership
	FileName   string `json:"file_name" gorm:"type:varchar(255)"`
	StoredName string `json:"stored_name" gorm:"type:varchar(255)"`
	FilePath   string `json:"file_path" gorm:"type:varchar(512)"`
	FileSize   int64  `json:"file_size" gorm:"bigint"`
	MimeType   string `json:"mime_type" gorm:"type:varchar(128)"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index"`
}

func (a *TicketAttachment) Insert() error {
	if a.CreatedAt == 0 {
		a.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(a).Error
}

func GetAttachmentById(id int) (*TicketAttachment, error) {
	if id <= 0 {
		return nil, errors.New("invalid attachment id")
	}
	a := &TicketAttachment{}
	if err := DB.Where("id = ?", id).First(a).Error; err != nil {
		return nil, err
	}
	return a, nil
}

// GetAttachmentsByMessageIds loads attachments bound to the given messages.
func GetAttachmentsByMessageIds(messageIds []int) ([]*TicketAttachment, error) {
	var list []*TicketAttachment
	if len(messageIds) == 0 {
		return list, nil
	}
	err := DB.Where("message_id IN ?", messageIds).Order("id asc").Find(&list).Error
	return list, err
}

var ErrTicketAttachmentUnavailable = errors.New("one or more ticket attachments are unavailable")

// bindTicketAttachments runs inside the message transaction, after locking its
// ticket. UPDATE obtains the same attachment row locks as retention cleanup.
// A partial bind is an error so the caller rolls back the entire message.
func bindTicketAttachments(tx *gorm.DB, attachmentIds []int, ticket *Ticket, message *TicketMessage) error {
	if len(attachmentIds) == 0 {
		return nil
	}
	query := tx.Model(&TicketAttachment{}).
		Where("id IN ? AND user_id = ? AND message_id = 0", attachmentIds, message.UserId)
	if !message.IsAdmin {
		query = query.Where("site_id = ?", ticket.SiteId)
	}
	res := query.Updates(map[string]interface{}{
		"ticket_id":  ticket.Id,
		"message_id": message.Id,
		"site_id":    ticket.SiteId,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != int64(len(attachmentIds)) {
		return ErrTicketAttachmentUnavailable
	}
	return nil
}

// GetTicketAttachments returns all attachments belonging to a ticket.
func GetTicketAttachments(ticketId int) ([]*TicketAttachment, error) {
	var list []*TicketAttachment
	err := DB.Where("ticket_id = ?", ticketId).Order("id asc").Find(&list).Error
	return list, err
}

// GetAttachmentsByTicketIds returns all attachments bound to any of the tickets.
func GetAttachmentsByTicketIds(ticketIds []int) ([]*TicketAttachment, error) {
	var list []*TicketAttachment
	if len(ticketIds) == 0 {
		return list, nil
	}
	err := DB.Where("ticket_id IN ?", ticketIds).Find(&list).Error
	return list, err
}

// CleanupResult summarizes a ticket cleanup run.
type CleanupResult struct {
	DeletedRows     int      `json:"deleted_rows"`
	DeletedFiles    int      `json:"deleted_files"`
	FailedFiles     int      `json:"failed_files"`
	DeletedTickets  int      `json:"deleted_tickets,omitempty"`
	DeletedMessages int      `json:"deleted_messages,omitempty"`
	Errors          []string `json:"errors,omitempty"`
}

// CollectAttachmentsForCleanup returns attachments eligible for removal:
//   - orphaned uploads never bound to a message older than orphanBefore, and/or
//   - attachments belonging to closed tickets closed before closedBefore.
//
// A zero cutoff disables that branch.
func CollectAttachmentsForCleanup(siteScope int, orphanBefore, closedBefore int64) ([]*TicketAttachment, error) {
	var list []*TicketAttachment
	seen := map[int]bool{}

	appendUnique := func(rows []*TicketAttachment) {
		for _, r := range rows {
			if !seen[r.Id] {
				seen[r.Id] = true
				list = append(list, r)
			}
		}
	}

	if orphanBefore > 0 {
		var rows []*TicketAttachment
		q := DB.Where("message_id = 0 AND created_at < ?", orphanBefore)
		if siteScope != SiteScopeAll {
			q = q.Where("site_id = ?", siteScope)
		}
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		appendUnique(rows)
	}

	if closedBefore > 0 {
		var closedTicketIds []int
		tq := DB.Model(&Ticket{}).Where("status = ? AND closed_at > 0 AND closed_at < ?", TicketStatusClosed, closedBefore)
		if siteScope != SiteScopeAll {
			tq = tq.Where("site_id = ?", siteScope)
		}
		if err := tq.Pluck("id", &closedTicketIds).Error; err != nil {
			return nil, err
		}
		if len(closedTicketIds) > 0 {
			var rows []*TicketAttachment
			if err := DB.Where("ticket_id IN ?", closedTicketIds).Find(&rows).Error; err != nil {
				return nil, err
			}
			appendUnique(rows)
		}
	}

	return list, nil
}
