package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/captcha"
	"github.com/gin-gonic/gin"
)

// CaptchaCheck guards anonymous auth endpoints with the configured
// human-verification channel (Turnstile / GeeTest / ALTCHA / Tencent).
// The client token still travels in the legacy "turnstile" query parameter
// so every existing call site keeps working regardless of channel.
// Dashboard sessions were replaced by stateless tokens upstream, so the
// verification result is no longer memoized per session; every guarded
// request must present a fresh captcha token.
func CaptchaCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.TurnstileCheckEnabled {
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
		c.Next()
	}
}
