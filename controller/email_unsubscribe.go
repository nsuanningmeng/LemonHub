package controller

import (
	"fmt"
	"html"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// One-click marketing unsubscribe (RFC 8058). Both handlers are unauthenticated
// on purpose: the signed token in the mail IS the credential, and the flow must
// work from any mailbox without a login — an unsubscribe that demands a login
// gets replaced by the "report spam" button, which is the outcome the whole
// feature exists to avoid.
//
// GET renders a tiny confirmation page whose button POSTs the same token. The
// action deliberately does NOT run on GET: link-scanning proxies (corporate
// gateways, mailbox prefetchers) follow GET links and would silently
// unsubscribe users who never clicked.

func unsubscribeHTML(body string) string {
	return "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>Unsubscribe</title></head>" +
		"<body style=\"font-family:sans-serif;max-width:480px;margin:80px auto;padding:0 16px;color:#333;text-align:center;\">" + body + "</body></html>"
}

// UnsubscribePage GET /api/unsubscribe?token= — confirmation page.
func UnsubscribePage(c *gin.Context) {
	token := c.Query("token")
	if _, err := common.ParseUnsubscribeToken(token); err != nil {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(unsubscribeHTML(
			`<h2>链接无效 / Invalid link</h2><p>退订链接无效或已过期。<br>This unsubscribe link is invalid or expired.</p>`)))
		return
	}
	safeToken := html.EscapeString(token)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(unsubscribeHTML(fmt.Sprintf(
		`<h2>退订营销邮件 / Unsubscribe</h2>`+
			`<p>确认后将不再向您发送营销/公告类邮件（验证码等账户邮件不受影响）。<br>`+
			`You will no longer receive marketing emails. Account emails (e.g. verification codes) are not affected.</p>`+
			`<form method="post" action="/api/unsubscribe?token=%s">`+
			`<button type="submit" style="padding:10px 32px;font-size:16px;border:0;border-radius:6px;background:#111;color:#fff;cursor:pointer;">确认退订 / Unsubscribe</button>`+
			`</form>`, safeToken))))
}

// UnsubscribeSubmit POST /api/unsubscribe?token= — performs the unsubscribe.
// Serves both the RFC 8058 one-click POST sent by mailbox providers and the
// confirmation form above. Idempotent.
func UnsubscribeSubmit(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.PostForm("token")
	}
	userId, err := common.ParseUnsubscribeToken(token)
	if err != nil {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(unsubscribeHTML(
			`<h2>链接无效 / Invalid link</h2><p>退订链接无效或已过期。<br>This unsubscribe link is invalid or expired.</p>`)))
		return
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		// The account is gone; there is nothing to unsubscribe. Report success so
		// mailbox providers treat the one-click POST as honored.
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(unsubscribeHTML(
			`<h2>已退订 / Unsubscribed</h2><p>您将不再收到营销邮件。<br>You will no longer receive marketing emails.</p>`)))
		return
	}
	settings := user.GetSetting()
	if !settings.MarketingEmailDisabled {
		settings.MarketingEmailDisabled = true
		if err := model.UpdateUserSetting(user.Id, settings); err != nil {
			common.SysError(fmt.Sprintf("one-click unsubscribe: failed to update settings for user %d: %s", user.Id, err.Error()))
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(unsubscribeHTML(
				`<h2>操作失败 / Something went wrong</h2><p>请稍后重试。<br>Please try again later.</p>`)))
			return
		}
		common.SysLog(fmt.Sprintf("one-click unsubscribe: user %d opted out of marketing email", user.Id))
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(unsubscribeHTML(
		`<h2>已退订 / Unsubscribed</h2><p>您将不再收到营销邮件。如需恢复，可在个人设置中重新开启。<br>You will no longer receive marketing emails. You can re-enable them in your profile settings.</p>`)))
}
