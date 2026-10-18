package model

import (
	"errors"

	"gorm.io/gorm"
)

// DeleteEligibleTicketAttachment rechecks a candidate under locks; the scan
// that selected it is only a hint. The returned file may be removed only after
// this transaction succeeds. A failed/unknown commit keeps the disk file.
func DeleteEligibleTicketAttachment(id, siteScope int, orphanBefore, closedBefore int64) (*TicketAttachment, error) {
	var deleted *TicketAttachment
	err := DB.Transaction(func(tx *gorm.DB) error {
		var candidate TicketAttachment
		if err := tx.Where("id = ?", id).First(&candidate).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var ticket Ticket
		if candidate.TicketId > 0 {
			// Reply/binding and purge use this same parent-before-child order.
			if err := lockForUpdate(tx).Where("id = ?", candidate.TicketId).First(&ticket).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
		}
		var current TicketAttachment
		if err := lockForUpdate(tx).Where("id = ?", id).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if current.TicketId != candidate.TicketId || (siteScope != SiteScopeAll && current.SiteId != siteScope) {
			return nil
		}
		orphanEligible := orphanBefore > 0 && current.MessageId == 0 && current.TicketId == 0 && current.CreatedAt < orphanBefore
		closedEligible := closedBefore > 0 && ticket.Id > 0 && ticket.Status == TicketStatusClosed && ticket.ClosedAt > 0 && ticket.ClosedAt < closedBefore
		if !orphanEligible && !closedEligible {
			return nil
		}
		if siteScope != SiteScopeAll && ticket.Id > 0 && ticket.SiteId != siteScope {
			return nil
		}
		if err := tx.Delete(&current).Error; err != nil {
			return err
		}
		deleted = &current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

// PurgeClosedTicket deletes all ticket metadata in one transaction, only while
// the locked ticket still meets retention. Reopen/reply cannot race the purge.
func PurgeClosedTicket(id, siteScope int, closedBefore int64) ([]*TicketAttachment, int64, bool, error) {
	var attachments []*TicketAttachment
	var messages int64
	deleted := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := lockForUpdate(tx).Where("id = ?", id).First(&ticket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if closedBefore <= 0 || ticket.Status != TicketStatusClosed || ticket.ClosedAt <= 0 || ticket.ClosedAt >= closedBefore || (siteScope != SiteScopeAll && ticket.SiteId != siteScope) {
			return nil
		}
		if err := tx.Where("ticket_id = ?", ticket.Id).Find(&attachments).Error; err != nil {
			return err
		}
		if err := tx.Where("ticket_id = ?", ticket.Id).Delete(&TicketAttachment{}).Error; err != nil {
			return err
		}
		result := tx.Where("ticket_id = ?", ticket.Id).Delete(&TicketMessage{})
		if result.Error != nil {
			return result.Error
		}
		messages = result.RowsAffected
		if err := tx.Delete(&ticket).Error; err != nil {
			return err
		}
		deleted = true
		return nil
	})
	if err != nil {
		return nil, 0, false, err
	}
	return attachments, messages, deleted, nil
}
