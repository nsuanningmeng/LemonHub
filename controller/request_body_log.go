package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetRequestBodyLog 管理员按 request_id 查看已记录的完整请求体。
func GetRequestBodyLog(c *gin.Context) {
	requestId := strings.TrimSpace(c.Param("request_id"))
	if requestId == "" {
		common.ApiErrorMsg(c, "request_id is required")
		return
	}
	record, err := model.GetRequestBodyLogByRequestId(requestId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "request body not recorded for this request",
			})
			return
		}
		common.ApiError(c, err)
		return
	}

	// Enforce the same role hierarchy as the toggle (SetUserRequestBodyRecord) and
	// GetUser: a sub-root admin must not read the raw request body of a peer/higher
	// user. The body is a more sensitive data class (raw client payload) than the
	// admin log content, so fail closed if the target user can't be verified.
	targetUser, err := model.GetUserById(record.UserId, false)
	if err != nil {
		common.ApiErrorMsg(c, "无权查看该用户的请求体")
		return
	}
	if !canManageTargetRole(c.GetInt("role"), targetUser.Role) {
		common.ApiErrorMsg(c, "无权查看该用户的请求体")
		return
	}

	recordManageAudit(c, "log.request_body_view", map[string]interface{}{
		"request_id": requestId,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    record,
	})
}

// ClearRequestBodyRecords 管理员清空全部已记录的请求体（隐私/空间清理）。
func ClearRequestBodyRecords(c *gin.Context) {
	deleted, err := model.ClearAllRequestBodyLogs()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "log.request_body_clear", map[string]interface{}{
		"deleted": deleted,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted_count": deleted,
		},
	})
}

type setUserRequestBodyRecordRequest struct {
	Id      int  `json:"id"`
	Enabled bool `json:"enabled"`
}

// SetUserRequestBodyRecord 管理员为指定用户开启/关闭完整请求体记录。
// 采用 read-modify-write 整个 Setting 结构，避免遗漏或覆盖其他设置字段。
func SetUserRequestBodyRecord(c *gin.Context) {
	var req setUserRequestBodyRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Id == 0 {
		common.ApiErrorMsg(c, "invalid params")
		return
	}

	user, err := model.GetUserById(req.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if !canManageTargetRole(myRole, user.Role) {
		common.ApiErrorMsg(c, "无权操作同级或更高权限的用户")
		return
	}

	setting := user.GetSetting()
	if setting.RecordRequestBody == req.Enabled {
		// 幂等：无变化直接返回成功。
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}
	setting.RecordRequestBody = req.Enabled
	if err := model.UpdateUserSetting(user.Id, setting); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.InvalidateUserCache(user.Id); err != nil {
		common.SysLog("failed to invalidate user cache after request-body-record toggle: " + err.Error())
	}

	// Turning recording OFF also purges this user's already-stored bodies: keeping raw
	// payloads (secrets/PII) around after the admin explicitly disabled collection would
	// be surprising, and nothing else evicts them once the user stops generating traffic.
	if !req.Enabled {
		if deleted, err := model.DeleteRequestBodyLogsByUserId(user.Id); err != nil {
			common.SysLog("failed to purge request body logs after disabling recording: " + err.Error())
		} else if deleted > 0 {
			common.SysLog(fmt.Sprintf("purged %d recorded request bodies for user %d after disabling recording", deleted, user.Id))
		}
	}

	state := "Disabled"
	if req.Enabled {
		state = "Enabled"
	}
	recordManageAuditFor(c, user.Id, "user.request_body_record", map[string]interface{}{
		"state":    state,
		"username": user.Username,
		"id":       strconv.Itoa(user.Id),
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
