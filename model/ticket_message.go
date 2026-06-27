package model

import (
	"github.com/QuantumNous/new-api/common"
)

type TicketMessage struct {
	Id        int    `json:"id"`
	TicketId  int    `json:"ticket_id" gorm:"index"`
	UserId    int    `json:"user_id" gorm:"index"`
	IsAdmin   bool   `json:"is_admin"`
	Content   string `json:"content" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`

	// Transient display fields (not persisted).
	Username    string              `json:"username,omitempty" gorm:"-"`
	Attachments []*TicketAttachment `json:"attachments,omitempty" gorm:"-"`
}

func (m *TicketMessage) Insert() error {
	if m.CreatedAt == 0 {
		m.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(m).Error
}

// GetTicketMessages returns all messages of a ticket in chronological order.
func GetTicketMessages(ticketId int) ([]*TicketMessage, error) {
	var messages []*TicketMessage
	err := DB.Where("ticket_id = ?", ticketId).Order("id asc").Find(&messages).Error
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// CountTicketMessages returns the number of messages in a ticket.
func CountTicketMessages(ticketId int) (int64, error) {
	var count int64
	err := DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticketId).Count(&count).Error
	return count, err
}

// CountTicketMessagesByTicketIds returns a ticketId -> message count map in one
// grouped query, avoiding N+1 counts in the admin ticket list.
func CountTicketMessagesByTicketIds(ticketIds []int) (map[int]int, error) {
	result := make(map[int]int)
	if len(ticketIds) == 0 {
		return result, nil
	}
	type countRow struct {
		TicketId int
		Cnt      int
	}
	var rows []countRow
	if err := DB.Model(&TicketMessage{}).
		Select("ticket_id, count(*) as cnt").
		Where("ticket_id IN ?", ticketIds).
		Group("ticket_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.TicketId] = r.Cnt
	}
	return result, nil
}

// DeleteMessagesByTicketIds hard-deletes all messages belonging to the given tickets.
func DeleteMessagesByTicketIds(ticketIds []int) (int64, error) {
	if len(ticketIds) == 0 {
		return 0, nil
	}
	res := DB.Where("ticket_id IN ?", ticketIds).Delete(&TicketMessage{})
	return res.RowsAffected, res.Error
}
