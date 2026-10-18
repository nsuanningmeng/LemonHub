package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrTicketClosed = errors.New("ticket is closed")

// CreateTicketWithMessage commits the ticket, first message and all requested
// attachments together. Failed/partial attachment binding must not look like a
// successful submission or leave an empty ticket behind.
func CreateTicketWithMessage(ticket *Ticket, message *TicketMessage, attachmentIDs []int) error {
	now := common.GetTimestamp()
	ticket.CreatedAt, ticket.UpdatedAt = now, now
	message.CreatedAt = now
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ticket).Error; err != nil {
			return err
		}
		message.TicketId = ticket.Id
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return bindTicketAttachments(tx, attachmentIDs, ticket, message)
	})
}

// ReplyToTicket serializes reply/reopen with retention cleanup on the ticket
// row. Notification callers receive the committed ticket state only on success.
func ReplyToTicket(ticket *Ticket, message *TicketMessage, attachmentIDs []int) error {
	var current Ticket
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", ticket.Id).First(&current).Error; err != nil {
			return err
		}
		if !message.IsAdmin {
			if current.UserId != message.UserId {
				return gorm.ErrRecordNotFound
			}
			if current.Status == TicketStatusClosed {
				return ErrTicketClosed
			}
		}
		message.TicketId = current.Id
		message.CreatedAt = common.GetTimestamp()
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		if err := bindTicketAttachments(tx, attachmentIDs, &current, message); err != nil {
			return err
		}
		current.LastReplyAt, current.UpdatedAt = message.CreatedAt, message.CreatedAt
		current.ClosedAt = 0
		current.LastReplyBy, current.Status = TicketReplyByUser, TicketStatusOpen
		if message.IsAdmin {
			current.LastReplyBy, current.Status = TicketReplyByAdmin, TicketStatusAwaitingUser
		}
		return tx.Model(&Ticket{}).Where("id = ?", current.Id).Updates(map[string]interface{}{
			"last_reply_at": current.LastReplyAt,
			"last_reply_by": current.LastReplyBy,
			"status":        current.Status,
			"updated_at":    current.UpdatedAt,
			"closed_at":     current.ClosedAt,
		}).Error
	})
	if err != nil {
		return err
	}
	*ticket = current
	return nil
}

// UpdateDetails holds the same ticket lock as retention cleanup. A stale
// request cannot report a successful reopen after the row has been purged.
func (ticket *Ticket) UpdateDetails(status, priority string) error {
	var current Ticket
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", ticket.Id).First(&current).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"updated_at": common.GetTimestamp()}
		if status != "" {
			if status != TicketStatusOpen && status != TicketStatusAwaitingUser && status != TicketStatusClosed {
				return errors.New("invalid ticket status")
			}
			current.Status, current.ClosedAt = status, 0
			if status == TicketStatusClosed {
				current.ClosedAt = common.GetTimestamp()
			}
			updates["status"], updates["closed_at"] = current.Status, current.ClosedAt
		}
		if priority != "" {
			if !IsValidTicketPriority(priority) {
				return errors.New("invalid ticket priority")
			}
			current.Priority = priority
			updates["priority"] = priority
		}
		current.UpdatedAt = updates["updated_at"].(int64)
		return tx.Model(&Ticket{}).Where("id = ?", current.Id).Updates(updates).Error
	})
	if err != nil {
		return err
	}
	*ticket = current
	return nil
}
