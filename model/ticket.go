package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Ticket status values.
const (
	TicketStatusOpen         = "open"          // 新建/用户回复后，等待管理员处理
	TicketStatusAwaitingUser = "awaiting_user" // 管理员已回复，等待用户
	TicketStatusClosed       = "closed"        // 已关闭
)

// Last-reply author values.
const (
	TicketReplyByUser  = "user"
	TicketReplyByAdmin = "admin"
)

// Ticket priority values.
const (
	TicketPriorityLow    = "low"
	TicketPriorityNormal = "normal"
	TicketPriorityHigh   = "high"
	TicketPriorityUrgent = "urgent"
)

// IsValidTicketPriority reports whether p is an accepted priority value.
func IsValidTicketPriority(p string) bool {
	switch p {
	case TicketPriorityLow, TicketPriorityNormal, TicketPriorityHigh, TicketPriorityUrgent:
		return true
	}
	return false
}

type Ticket struct {
	Id          int    `json:"id"`
	SiteId      int    `json:"site_id" gorm:"type:int;default:0;index"`
	UserId      int    `json:"user_id" gorm:"index"`
	Type        string `json:"type" gorm:"type:varchar(64);index"`
	Title       string `json:"title" gorm:"type:varchar(255)"`
	Status      string `json:"status" gorm:"type:varchar(32);index"`
	Priority    string `json:"priority" gorm:"type:varchar(16)"`
	LastReplyAt int64  `json:"last_reply_at" gorm:"bigint;index"`
	LastReplyBy string `json:"last_reply_by" gorm:"type:varchar(16)"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;index"`
	ClosedAt    int64  `json:"closed_at" gorm:"bigint"`

	// Transient display fields (not persisted).
	Username   string `json:"username,omitempty" gorm:"-"`
	UserEmail  string `json:"user_email,omitempty" gorm:"-"`
	MessageNum int    `json:"message_num,omitempty" gorm:"-"`
}

func (t *Ticket) Insert() error {
	now := common.GetTimestamp()
	if t.CreatedAt == 0 {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = TicketStatusOpen
	}
	if t.Priority == "" {
		t.Priority = "normal"
	}
	return DB.Create(t).Error
}

func GetTicketById(id int) (*Ticket, error) {
	if id <= 0 {
		return nil, errors.New("invalid ticket id")
	}
	ticket := &Ticket{}
	err := DB.Where("id = ?", id).First(ticket).Error
	if err != nil {
		return nil, err
	}
	return ticket, nil
}

// GetUserTickets returns the caller's own tickets, newest activity first.
func GetUserTickets(userId int, status string, pageInfo *common.PageInfo) ([]*Ticket, int64, error) {
	var tickets []*Ticket
	var total int64
	query := DB.Model(&Ticket{}).Where("user_id = ?", userId)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("updated_at desc, id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Find(&tickets).Error
	if err != nil {
		return nil, 0, err
	}
	return tickets, total, nil
}

// GetAllTickets returns tickets for the admin console, scoped by site.
func GetAllTickets(siteScope int, status, priority, ticketType string, userId int, keyword string, pageInfo *common.PageInfo) ([]*Ticket, int64, error) {
	var tickets []*Ticket
	var total int64
	query := DB.Model(&Ticket{})
	if siteScope != SiteScopeAll {
		query = query.Where("site_id = ?", siteScope)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if priority != "" {
		query = query.Where("priority = ?", priority)
	}
	if ticketType != "" {
		query = query.Where("type = ?", ticketType)
	}
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if keyword != "" {
		// Escape LIKE metacharacters so an admin's literal search term isn't
		// reinterpreted as a wildcard pattern. Parameterization already prevents
		// SQL injection; this preserves exact substring semantics.
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
		query = query.Where("title LIKE ?", "%"+escaped+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("updated_at desc, id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Find(&tickets).Error
	if err != nil {
		return nil, 0, err
	}
	return tickets, total, nil
}

// UpdateStatus transitions a ticket to a new status, stamping closed_at on close.
func (t *Ticket) UpdateStatus(status string) error {
	now := common.GetTimestamp()
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": now,
	}
	if status == TicketStatusClosed {
		updates["closed_at"] = now
	} else {
		updates["closed_at"] = 0
	}
	if err := DB.Model(&Ticket{}).Where("id = ?", t.Id).Updates(updates).Error; err != nil {
		return err
	}
	t.Status = status
	t.UpdatedAt = now
	return nil
}

// UpdatePriority changes a ticket's priority (admin action).
func (t *Ticket) UpdatePriority(priority string) error {
	now := common.GetTimestamp()
	if err := DB.Model(&Ticket{}).Where("id = ?", t.Id).Updates(map[string]interface{}{
		"priority":   priority,
		"updated_at": now,
	}).Error; err != nil {
		return err
	}
	t.Priority = priority
	t.UpdatedAt = now
	return nil
}

// GetClosedTicketIdsBefore returns ids of tickets closed before the cutoff, used by
// the retention sweep to purge long-closed tickets. A zero cutoff returns nothing.
func GetClosedTicketIdsBefore(siteScope int, closedBefore int64, limit int) ([]int, error) {
	if closedBefore <= 0 {
		return nil, nil
	}
	var ids []int
	q := DB.Model(&Ticket{}).
		Where("status = ? AND closed_at > 0 AND closed_at < ?", TicketStatusClosed, closedBefore)
	if siteScope != SiteScopeAll {
		q = q.Where("site_id = ?", siteScope)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// DeleteTicketsByIds hard-deletes ticket rows by id.
func DeleteTicketsByIds(ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := DB.Where("id IN ?", ids).Delete(&Ticket{})
	return res.RowsAffected, res.Error
}

// GetTicketUsersByIds batch-loads minimal user display info (id/username/email),
// avoiding per-row lookups in the admin ticket list.
func GetTicketUsersByIds(ids []int) (map[int]*User, error) {
	result := make(map[int]*User)
	if len(ids) == 0 {
		return result, nil
	}
	var users []*User
	if err := DB.Select("id", "username", "email").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		result[u.Id] = u
	}
	return result, nil
}

// MarkReplied updates reply bookkeeping and status after a new message.
func (t *Ticket) MarkReplied(replyBy string, newStatus string) error {
	now := common.GetTimestamp()
	updates := map[string]interface{}{
		"last_reply_at": now,
		"last_reply_by": replyBy,
		"status":        newStatus,
		"updated_at":    now,
	}
	if newStatus != TicketStatusClosed {
		updates["closed_at"] = 0
	}
	if err := DB.Model(&Ticket{}).Where("id = ?", t.Id).Updates(updates).Error; err != nil {
		return err
	}
	t.LastReplyAt = now
	t.LastReplyBy = replyBy
	t.Status = newStatus
	t.UpdatedAt = now
	return nil
}
