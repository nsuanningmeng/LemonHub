package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/captcha"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// CaptchaCheck guards anonymous auth endpoints with the configured
// human-verification channel (Turnstile / GeeTest / ALTCHA / Tencent).
// The client token still travels in the legacy "turnstile" query parameter
// so every existing call site keeps working regardless of channel.
func CaptchaCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.TurnstileCheckEnabled {
			c.Next()
			return
		}
		session := sessions.Default(c)
		if session.Get("turnstile") != nil {
			c.Next()
			return
		}
		token := c.Query("turnstile")
		if token == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "请先完成人机验证",
			})
			c.Abort()
			return
		}
		if err := captcha.Verify(token, c.ClientIP()); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			c.Abort()
			return
		}
		session.Set("turnstile", true)
		if err := session.Save(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message": "无法保存会话信息，请重试",
				"success": false,
			})
			return
		}
		c.Next()
	}
}
