package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// paymentReturnPath builds the absolute URL a browser is redirected to after a
// payment (success/cancel/return). It uses the CURRENT request's domain when that
// domain is trusted — so a user on a secondary/white-label domain returns to the
// same site they paid from — falling back to the configured ServerAddress for an
// untrusted Host or a nil context (GetRequestBaseURL handles both internally).
func paymentReturnPath(c *gin.Context, suffix string) string {
	base := strings.TrimRight(service.GetRequestBaseURL(c), "/")
	return base + common.ThemeAwarePath(suffix)
}
