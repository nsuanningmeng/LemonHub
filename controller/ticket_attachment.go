package controller

import (
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ticket_setting"

	"github.com/gin-gonic/gin"
)

// multipartOverheadBytes is slack added to the per-file size cap so the multipart
// envelope (boundaries, part headers) is not counted against the file's own limit.
const multipartOverheadBytes = 8 * 1024

// sanitizeAttachmentName strips any directory components from a client-supplied
// filename so only a safe display basename is persisted. The on-disk name is
// always server-generated; this value is for display only.
func sanitizeAttachmentName(name, fallback string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	base := path.Base(name)
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		return fallback
	}
	if len([]rune(base)) > 255 {
		base = string([]rune(base)[:255])
	}
	return base
}

// UploadTicketAttachment validates and stores an uploaded image, returning a
// reference the client can later bind to a ticket message.
func UploadTicketAttachment(c *gin.Context) {
	setting := ticket_setting.GetTicketSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "ticket system is disabled")
		return
	}

	// Cap the request body BEFORE parsing the multipart form so an oversized upload
	// is rejected without buffering it into memory/temp files.
	maxBytes := setting.MaxAttachmentBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+multipartOverheadBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorMsg(c, "no file uploaded or file too large")
		return
	}

	saved, err := common.SaveTicketUpload(fileHeader, setting.AllowedMimeTypes, maxBytes)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	userId := c.GetInt("id")
	att := &model.TicketAttachment{
		UserId:     userId,
		SiteId:     middleware.GetRequestSiteId(c),
		FileName:   sanitizeAttachmentName(fileHeader.Filename, saved.StoredName),
		StoredName: saved.StoredName,
		FilePath:   saved.RelPath,
		FileSize:   saved.Size,
		MimeType:   saved.MimeType,
	}
	if err := att.Insert(); err != nil {
		// Roll back the on-disk file so a failed insert does not orphan it.
		_ = common.DeleteUpload(saved.RelPath)
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"id":        att.Id,
		"url":       "/api/ticket/attachment/" + strconv.Itoa(att.Id),
		"file_name": att.FileName,
		"mime_type": att.MimeType,
		"file_size": att.FileSize,
	})
}

// GetTicketAttachment serves an attachment to authorized callers only:
// platform admins, the uploader, or the owner of the ticket the attachment is
// bound to. Any other caller is rejected without revealing the file.
func GetTicketAttachment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid attachment id")
		return
	}
	att, err := model.GetAttachmentById(id)
	if err != nil {
		common.ApiErrorMsg(c, "attachment not found")
		return
	}

	userId := c.GetInt("id")
	authorized := false
	switch {
	case c.GetInt("role") >= common.RoleAdminUser:
		// Admins may fetch attachments, but stay consistent with the admin ticket
		// endpoints' site-scoping: a scoped (sub-site) operator may only read
		// attachments in their own site. Platform admins/root have SiteScopeAll and
		// remain global, so this is defense-in-depth with no behavior change today.
		scope := middleware.EffectiveSiteScope(c)
		if scope == model.SiteScopeAll || att.SiteId == scope {
			authorized = true
		} else if att.TicketId > 0 {
			if ticket, err := model.GetTicketById(att.TicketId); err == nil && ticket.SiteId == scope {
				authorized = true
			}
		}
	case att.UserId == userId:
		authorized = true
	case att.TicketId > 0:
		if ticket, err := model.GetTicketById(att.TicketId); err == nil && ticket.UserId == userId {
			authorized = true
		}
	}
	if !authorized {
		common.ApiErrorMsg(c, "no permission to access this attachment")
		return
	}

	absPath, err := common.ResolveUploadPath(att.FilePath)
	if err != nil {
		common.ApiErrorMsg(c, "attachment not found")
		return
	}

	c.Header("Content-Type", att.MimeType)
	c.Header("Content-Disposition", "inline")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(absPath)
}
