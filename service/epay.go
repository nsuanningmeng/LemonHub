package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}

// GetCallbackAddressForRequest returns the base URL used to build payment
// provider callbacks (notify/webhook) for the CURRENT request. When the request
// arrives on a trusted, registered site domain, that SAME domain is used so a
// multi-domain deployment keeps each user's payment callbacks on the domain they
// actually visited. For an untrusted Host (or a nil context) it falls back to the
// statically configured custom callback address / server address — preserving the
// previous single-domain behavior and preventing Host-header spoofing of the
// callback target. The result never carries a trailing slash, so callers may
// append a path directly (e.g. addr + "/api/...").
func GetCallbackAddressForRequest(c *gin.Context) string {
	if c != nil && IsRequestHostTrusted(c) {
		return strings.TrimRight(GetRequestBaseURL(c), "/")
	}
	return strings.TrimRight(GetCallbackAddress(), "/")
}
