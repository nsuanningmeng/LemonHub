package controller

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ticket_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const maxTicketTitleLen = 255

// maxTicketContentBytes bounds a message body well under MySQL's TEXT column limit
// (65535 bytes) so user content can never be silently truncated on a MySQL backend,
// regardless of SQL strict mode. The check is on byte length (len), not rune count.
const maxTicketContentBytes = 60000

// validateAttachmentIds dedupes the requested attachment ids, drops non-positive
// values, and enforces the per-message cap so a single message cannot bind an
// unbounded number of uploads.
func validateAttachmentIds(ids []int, maxCount int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[int]bool, len(ids))
	unique := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if maxCount > 0 && len(unique) > maxCount {
		return nil, fmt.Errorf("too many attachments (max %d)", maxCount)
	}
	return unique, nil
}

// createTicketRequest is the body for creating a new ticket.
type createTicketRequest struct {
	Type          string `json:"type"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Priority      string `json:"priority"`
	AttachmentIds []int  `json:"attachment_ids"`
}

// ticketReplyRequest is the body for replying to a ticket (user or admin).
type ticketReplyRequest struct {
	Content       string `json:"content"`
	AttachmentIds []int  `json:"attachment_ids"`
}

// adminUpdateRequest is the body for an admin status/priority update. Both fields
// are optional; at least one must be supplied.
type adminUpdateRequest struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// adminCleanupRequest is the body for the attachment cleanup job. Pointer fields are
// optional so callers can omit them and fall back to defaults.
type adminCleanupRequest struct {
	OrphanHours        *int `json:"orphan_hours"`
	ClosedDays         *int `json:"closed_days"`
	PurgeClosedTickets bool `json:"purge_closed_tickets"`
}

// buildTicketDetail loads a ticket's messages, attaches their bound attachments,
// and resolves display usernames. Shared by the user and admin detail endpoints.
func buildTicketDetail(ticket *model.Ticket) (gin.H, error) {
	messages, err := model.GetTicketMessages(ticket.Id)
	if err != nil {
		return nil, err
	}

	messageIds := make([]int, 0, len(messages))
	for _, m := range messages {
		messageIds = append(messageIds, m.Id)
	}
	attachments, err := model.GetAttachmentsByMessageIds(messageIds)
	if err != nil {
		return nil, err
	}
	attByMsg := make(map[int][]*model.TicketAttachment, len(messages))
	for _, a := range attachments {
		attByMsg[a.MessageId] = append(attByMsg[a.MessageId], a)
	}

	usernameCache := make(map[int]string)
	for _, m := range messages {
		m.Attachments = attByMsg[m.Id]
		name, ok := usernameCache[m.UserId]
		if !ok {
			if u, err := model.GetUserById(m.UserId, false); err == nil && u != nil {
				name = u.Username
			}
			usernameCache[m.UserId] = name
		}
		if name == "" && m.IsAdmin {
			name = "Admin"
		}
		m.Username = name
	}

	return gin.H{"ticket": ticket, "messages": messages}, nil
}

// ---- User-facing handlers (UserAuth) ----

// GetUserTickets lists the caller's own tickets, optionally filtered by status.
func GetUserTickets(c *gin.Context) {
	userId := c.GetInt("id")
	status := c.Query("status")
	pageInfo := common.GetPageQuery(c)
	tickets, total, err := model.GetUserTickets(userId, status, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tickets)
	common.ApiSuccess(c, pageInfo)
}

// CreateTicket opens a new ticket with its first message.
func CreateTicket(c *gin.Context) {
	setting := ticket_setting.GetTicketSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "ticket system is disabled")
		return
	}

	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.Type = strings.TrimSpace(req.Type)
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)

	if !setting.IsValidType(req.Type) {
		common.ApiErrorMsg(c, "invalid ticket type")
		return
	}
	if title == "" {
		common.ApiErrorMsg(c, "title cannot be empty")
		return
	}
	if len([]rune(title)) > maxTicketTitleLen {
		common.ApiErrorMsg(c, "title is too long")
		return
	}
	if content == "" {
		common.ApiErrorMsg(c, "content cannot be empty")
		return
	}
	if len(content) > maxTicketContentBytes {
		common.ApiErrorMsg(c, "content is too long")
		return
	}
	attachmentIds, err := validateAttachmentIds(req.AttachmentIds, setting.MaxAttachmentsPerMessage)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	priority := strings.TrimSpace(req.Priority)
	if priority == "" {
		priority = model.TicketPriorityNormal
	} else if !model.IsValidTicketPriority(priority) {
		common.ApiErrorMsg(c, "invalid priority")
		return
	}

	userId := c.GetInt("id")
	now := common.GetTimestamp()
	ticket := &model.Ticket{
		UserId:      userId,
		SiteId:      middleware.GetRequestSiteId(c),
		Type:        req.Type,
		Title:       title,
		Status:      model.TicketStatusOpen,
		Priority:    priority,
		LastReplyBy: model.TicketReplyByUser,
		LastReplyAt: now,
	}
	if err := ticket.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}

	msg := &model.TicketMessage{
		TicketId: ticket.Id,
		UserId:   userId,
		IsAdmin:  false,
		Content:  content,
	}
	if err := msg.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(attachmentIds) > 0 {
		_, _ = model.BindAttachmentsToMessage(attachmentIds, ticket.Id, msg.Id, userId)
	}

	gopool.Go(func() {
		service.NotifyAdminsOfTicket(ticket, "created")
	})

	common.ApiSuccess(c, gin.H{"id": ticket.Id})
}

// GetTicketDetail returns a ticket the caller owns, with its messages.
func GetTicketDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid ticket id")
		return
	}
	ticket, err := model.GetTicketById(id)
	// Return an identical message for "missing" and "not yours" so a user cannot
	// enumerate which ticket ids exist.
	if err != nil || ticket.UserId != c.GetInt("id") {
		common.ApiErrorMsg(c, "ticket not found")
		return
	}

	payload, err := buildTicketDetail(ticket)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, payload)
}

// ReplyTicket appends a user reply and reopens the ticket for admin attention.
func ReplyTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid ticket id")
		return
	}
	ticket, err := model.GetTicketById(id)
	userId := c.GetInt("id")
	if err != nil || ticket.UserId != userId {
		common.ApiErrorMsg(c, "ticket not found")
		return
	}
	if ticket.Status == model.TicketStatusClosed {
		common.ApiErrorMsg(c, "ticket is closed")
		return
	}

	var req ticketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		common.ApiErrorMsg(c, "content cannot be empty")
		return
	}
	if len(content) > maxTicketContentBytes {
		common.ApiErrorMsg(c, "content is too long")
		return
	}
	attachmentIds, err := validateAttachmentIds(req.AttachmentIds, ticket_setting.GetTicketSetting().MaxAttachmentsPerMessage)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	msg := &model.TicketMessage{
		TicketId: ticket.Id,
		UserId:   userId,
		IsAdmin:  false,
		Content:  content,
	}
	if err := msg.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(attachmentIds) > 0 {
		_, _ = model.BindAttachmentsToMessage(attachmentIds, ticket.Id, msg.Id, userId)
	}
	if err := ticket.MarkReplied(model.TicketReplyByUser, model.TicketStatusOpen); err != nil {
		common.ApiError(c, err)
		return
	}

	gopool.Go(func() {
		service.NotifyAdminsOfTicket(ticket, "reply")
	})

	common.ApiSuccess(c, gin.H{"id": msg.Id})
}

// CloseTicket closes a ticket the caller owns.
func CloseTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid ticket id")
		return
	}
	ticket, err := model.GetTicketById(id)
	if err != nil || ticket.UserId != c.GetInt("id") {
		common.ApiErrorMsg(c, "ticket not found")
		return
	}
	if err := ticket.UpdateStatus(model.TicketStatusClosed); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// GetTicketConfig exposes the enabled flag and the enabled ticket types only
// (disabled types are never leaked to clients).
func GetTicketConfig(c *gin.Context) {
	setting := ticket_setting.GetTicketSetting()
	types := make([]gin.H, 0, len(setting.Types))
	if setting.Enabled {
		for _, t := range setting.Types {
			if !t.Enabled {
				continue
			}
			types = append(types, gin.H{
				"key":             t.Key,
				"name":            t.Name,
				"prompt_template": t.PromptTemplate,
			})
		}
	}
	common.ApiSuccess(c, gin.H{
		"enabled": setting.Enabled,
		"types":   types,
	})
}

// ---- Admin handlers (AdminAuth) ----

// AdminGetAllTickets lists tickets across the admin's effective site scope.
func AdminGetAllTickets(c *gin.Context) {
	scope := middleware.EffectiveSiteScope(c)
	status := c.Query("status")
	priority := c.Query("priority")
	ticketType := c.Query("type")
	userId, _ := strconv.Atoi(c.Query("user_id"))
	keyword := strings.TrimSpace(c.Query("keyword"))

	pageInfo := common.GetPageQuery(c)
	tickets, total, err := model.GetAllTickets(scope, status, priority, ticketType, userId, keyword, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Batch-load user display info and message counts to avoid N+1 queries.
	userIds := make([]int, 0, len(tickets))
	ticketIds := make([]int, 0, len(tickets))
	for _, t := range tickets {
		userIds = append(userIds, t.UserId)
		ticketIds = append(ticketIds, t.Id)
	}
	users, _ := model.GetTicketUsersByIds(userIds)
	counts, _ := model.CountTicketMessagesByTicketIds(ticketIds)
	for _, t := range tickets {
		if u := users[t.UserId]; u != nil {
			t.Username = u.Username
			t.UserEmail = u.Email
		}
		t.MessageNum = counts[t.Id]
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tickets)
	common.ApiSuccess(c, pageInfo)
}

// AdminGetTicketDetail returns any ticket within the admin's site scope.
func AdminGetTicketDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid ticket id")
		return
	}
	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope := middleware.EffectiveSiteScope(c)
	if scope != model.SiteScopeAll && ticket.SiteId != scope {
		common.ApiErrorMsg(c, "no permission to access this ticket")
		return
	}

	if u, err := model.GetUserById(ticket.UserId, false); err == nil && u != nil {
		ticket.Username = u.Username
		ticket.UserEmail = u.Email
	}

	payload, err := buildTicketDetail(ticket)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, payload)
}

// AdminReplyTicket appends an admin reply and moves the ticket to awaiting_user,
// then asynchronously notifies the ticket owner.
func AdminReplyTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid ticket id")
		return
	}
	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope := middleware.EffectiveSiteScope(c)
	if scope != model.SiteScopeAll && ticket.SiteId != scope {
		common.ApiErrorMsg(c, "no permission to access this ticket")
		return
	}

	var req ticketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		common.ApiErrorMsg(c, "content cannot be empty")
		return
	}
	if len(content) > maxTicketContentBytes {
		common.ApiErrorMsg(c, "content is too long")
		return
	}
	attachmentIds, err := validateAttachmentIds(req.AttachmentIds, ticket_setting.GetTicketSetting().MaxAttachmentsPerMessage)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	adminId := c.GetInt("id")
	msg := &model.TicketMessage{
		TicketId: ticket.Id,
		UserId:   adminId,
		IsAdmin:  true,
		Content:  content,
	}
	if err := msg.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(attachmentIds) > 0 {
		_, _ = model.BindAttachmentsToMessage(attachmentIds, ticket.Id, msg.Id, adminId)
	}
	if err := ticket.MarkReplied(model.TicketReplyByAdmin, model.TicketStatusAwaitingUser); err != nil {
		common.ApiError(c, err)
		return
	}

	ownerId := ticket.UserId
	ticketId := ticket.Id
	title := ticket.Title
	gopool.Go(func() {
		owner, err := model.GetUserById(ownerId, false)
		if err != nil || owner == nil {
			return
		}
		// Escape the user-set title — the notification is delivered as an HTML email body.
		body := fmt.Sprintf("Your ticket \"%s\" (#%d) has a new reply.", html.EscapeString(title), ticketId)
		notify := dto.NewNotify("ticket_reply", "Your ticket has a new reply", body, nil)
		if notifyErr := service.NotifyUser(owner.Id, owner.Email, owner.GetSetting(), notify); notifyErr != nil {
			common.SysLog("failed to notify ticket owner: " + notifyErr.Error())
		}
	})

	common.ApiSuccess(c, gin.H{"id": msg.Id})
}

// AdminUpdateTicketStatus transitions a ticket to a validated status.
func AdminUpdateTicketStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid ticket id")
		return
	}
	ticket, err := model.GetTicketById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope := middleware.EffectiveSiteScope(c)
	if scope != model.SiteScopeAll && ticket.SiteId != scope {
		common.ApiErrorMsg(c, "no permission to access this ticket")
		return
	}

	var req adminUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.Status = strings.TrimSpace(req.Status)
	req.Priority = strings.TrimSpace(req.Priority)
	if req.Status == "" && req.Priority == "" {
		common.ApiErrorMsg(c, "nothing to update")
		return
	}
	if req.Status != "" {
		switch req.Status {
		case model.TicketStatusOpen, model.TicketStatusAwaitingUser, model.TicketStatusClosed:
		default:
			common.ApiErrorMsg(c, "invalid status")
			return
		}
		if err := ticket.UpdateStatus(req.Status); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if req.Priority != "" {
		if !model.IsValidTicketPriority(req.Priority) {
			common.ApiErrorMsg(c, "invalid priority")
			return
		}
		if err := ticket.UpdatePriority(req.Priority); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	recordManageAudit(c, "ticket.update", map[string]interface{}{
		"ticket_id": ticket.Id,
		"status":    req.Status,
		"priority":  req.Priority,
	})
	common.ApiSuccess(c, nil)
}

// AdminCleanupAttachments deletes orphaned uploads and/or closed-ticket
// attachments within the admin's site scope and returns a CleanupResult.
func AdminCleanupAttachments(c *gin.Context) {
	var req adminCleanupRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ApiError(c, err)
		return
	}

	orphanHours := 24
	if req.OrphanHours != nil {
		orphanHours = *req.OrphanHours
	}
	closedDays := 0
	if req.ClosedDays != nil {
		closedDays = *req.ClosedDays
	}

	now := common.GetTimestamp()
	var orphanBefore int64
	if orphanHours > 0 {
		orphanBefore = now - int64(orphanHours)*3600
	}
	var closedBefore int64
	if closedDays > 0 {
		closedBefore = now - int64(closedDays)*86400
	}

	scope := middleware.EffectiveSiteScope(c)
	attachments, err := model.CollectAttachmentsForCleanup(scope, orphanBefore, closedBefore)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	result := model.CleanupResult{}
	deletedIds := make([]int, 0, len(attachments))
	for _, a := range attachments {
		if err := common.DeleteUpload(a.FilePath); err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("attachment %d: %s", a.Id, err.Error()))
			continue
		}
		result.DeletedFiles++
		deletedIds = append(deletedIds, a.Id)
	}
	if len(deletedIds) > 0 {
		rows, err := model.DeleteAttachmentRows(deletedIds)
		if err != nil {
			result.Errors = append(result.Errors, "delete rows: "+err.Error())
		} else {
			result.DeletedRows = int(rows)
		}
	}

	// Optionally purge the long-closed tickets themselves (and their messages) to
	// bound closed-ticket storage growth. Their attachments were already removed
	// above under the same closedBefore cutoff.
	if req.PurgeClosedTickets && closedBefore > 0 {
		if ticketIds, err := model.GetClosedTicketIdsBefore(scope, closedBefore, 0); err == nil && len(ticketIds) > 0 {
			if msgs, err := model.DeleteMessagesByTicketIds(ticketIds); err == nil {
				result.DeletedMessages = int(msgs)
			}
			if tk, err := model.DeleteTicketsByIds(ticketIds); err == nil {
				result.DeletedTickets = int(tk)
			}
		}
	}

	recordManageAudit(c, "ticket.cleanup", map[string]interface{}{
		"deleted_files":    result.DeletedFiles,
		"deleted_rows":     result.DeletedRows,
		"failed_files":     result.FailedFiles,
		"deleted_tickets":  result.DeletedTickets,
		"deleted_messages": result.DeletedMessages,
	})
	common.ApiSuccess(c, result)
}
