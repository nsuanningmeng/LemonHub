package controller

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// maxDeliveryEventBodyBytes bounds a single callback payload.
const maxDeliveryEventBodyBytes = 1 * 1024 * 1024

// EmailDeliveryEvents POST /api/email/delivery-events?key=<token> — ingestion
// endpoint for provider delivery events (Aliyun DirectMail via EventBridge HTTP
// target or MNS HTTP endpoint). Learned bounces/complaints go straight to the
// suppression list, which is how the sender's invalid-address and spam-rate
// reputation metrics are protected without manual list hygiene.
//
// Auth: shared token (EmailDeliveryEventToken option) via ?key= or the
// X-Event-Token header; the endpoint is disabled while the option is empty.
// Always answers 2xx once authenticated — providers retry non-2xx responses,
// and retrying an unparseable payload can never make it parseable.
func EmailDeliveryEvents(c *gin.Context) {
	configured := common.EmailDeliveryEventToken
	if configured == "" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "delivery event callback is not enabled"})
		return
	}
	provided := c.Query("key")
	if provided == "" {
		provided = c.GetHeader("X-Event-Token")
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid token"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxDeliveryEventBodyBytes+1))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "processed": 0})
		return
	}
	if len(body) > maxDeliveryEventBodyBytes {
		c.JSON(http.StatusOK, gin.H{"success": true, "processed": 0, "message": "payload too large, ignored"})
		return
	}

	actions := service.ParseEmailDeliveryEvents(body)
	applied := service.ApplyEmailDeliveryActions(actions)
	if applied > 0 {
		common.SysLog(fmt.Sprintf("email delivery events: applied %d suppression(s) from callback", applied))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "processed": applied})
}
