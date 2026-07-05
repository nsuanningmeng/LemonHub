package controller

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// Import guards: an Aliyun invalid-address export can be large, but a single
// request must stay bounded.
const (
	maxSuppressionImportBytes  = 2 * 1024 * 1024
	maxSuppressionImportEmails = 50000
)

var suppressionEmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// ListEmailSuppressions GET /api/email-suppression/ — paginated, newest first,
// optional ?keyword= substring filter.
func ListEmailSuppressions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	list, total, err := model.GetEmailSuppressions(c.Query("keyword"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(list)
	common.ApiSuccess(c, pageInfo)
}

type addEmailSuppressionRequest struct {
	Email  string `json:"email"`
	Reason string `json:"reason"` // hard_bounce | complaint
}

// AddEmailSuppression POST /api/email-suppression/ — manually block one address.
func AddEmailSuppression(c *gin.Context) {
	var req addEmailSuppressionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	email := strings.TrimSpace(req.Email)
	if err := common.Validate.Var(email, "required,email"); err != nil {
		common.ApiErrorMsg(c, "invalid email address")
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = model.SuppressionReasonHardBounce
	}
	if reason != model.SuppressionReasonHardBounce && reason != model.SuppressionReasonComplaint {
		common.ApiErrorMsg(c, "invalid reason")
		return
	}
	if err := model.UpsertEmailSuppression(email, reason, model.SuppressionSourceManual, "added by admin"); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "email_suppression.add", map[string]interface{}{
		"email":  email,
		"reason": reason,
	})
	common.ApiSuccess(c, nil)
}

// DeleteEmailSuppression DELETE /api/email-suppression/:id — unblock an address
// (e.g. a false positive, or a mailbox that came back to life).
func DeleteEmailSuppression(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteEmailSuppression(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "email_suppression.delete", map[string]interface{}{
		"suppression_id": id,
	})
	common.ApiSuccess(c, nil)
}

type importEmailSuppressionRequest struct {
	Content string `json:"content"` // free text; every email-shaped token is imported
}

// ImportEmailSuppressions POST /api/email-suppression/import — bulk import for
// provider console exports (e.g. Aliyun DirectMail 数据统计 → 无效地址). The body
// is free text/CSV; every email-shaped token is suppressed as a hard bounce.
func ImportEmailSuppressions(c *gin.Context) {
	var req importEmailSuppressionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(req.Content) > maxSuppressionImportBytes {
		common.ApiErrorMsg(c, "import content too large (max 2MB)")
		return
	}
	matches := suppressionEmailPattern.FindAllString(req.Content, -1)
	if len(matches) == 0 {
		common.ApiErrorMsg(c, "no email addresses found in the submitted content")
		return
	}
	if len(matches) > maxSuppressionImportEmails {
		common.ApiErrorMsg(c, "too many addresses in one import (max 50000)")
		return
	}
	seen := make(map[string]bool, len(matches))
	imported := 0
	failed := 0
	for _, email := range matches {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		if err := model.UpsertEmailSuppression(normalized, model.SuppressionReasonHardBounce, model.SuppressionSourceImport, "imported invalid-address list"); err != nil {
			failed++
			continue
		}
		imported++
	}
	recordManageAudit(c, "email_suppression.import", map[string]interface{}{
		"imported": imported,
		"failed":   failed,
	})
	common.ApiSuccess(c, gin.H{
		"imported": imported,
		"failed":   failed,
	})
}
