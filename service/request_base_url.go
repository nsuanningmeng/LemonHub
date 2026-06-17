package service

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func GetRequestBaseURL(c *gin.Context) string {
	scheme := detectRequestScheme(c)
	host, ok := requestHostFromContext(c)
	if !ok {
		return fallbackServerAddress()
	}
	if isTrustedRequestHost(host) {
		return fmt.Sprintf("%s://%s", scheme, host)
	}
	return fallbackServerAddress()
}

func IsRequestHostTrusted(c *gin.Context) bool {
	host, ok := requestHostFromContext(c)
	if !ok {
		return false
	}
	return isTrustedRequestHost(host)
}

func requestHostFromContext(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil {
		return "", false
	}
	// Prefer the Host header (gin populates c.Request.Host). Standard reverse
	// proxies (nginx/Cloudflare) preserve it, so using it first stays correct
	// behind a proxy while avoiding X-Forwarded-Host spoofing from a direct
	// client (the router does not configure trusted proxies). The resolved host
	// is still whitelist-checked against registered SiteDomains below.
	if host := strings.TrimSpace(c.Request.Host); host != "" {
		return host, true
	}
	if host := firstHeaderValue(c.GetHeader("X-Forwarded-Host")); host != "" {
		return host, true
	}
	return "", false
}

func firstHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func detectRequestScheme(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return "http"
	}
	if proto := firstHeaderValue(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		return strings.ToLower(proto)
	}
	if proto := firstHeaderValue(c.GetHeader("X-Forwarded-Protocol")); proto != "" {
		return strings.ToLower(proto)
	}
	if c.Request.TLS != nil {
		return "https"
	}
	if c.Request.URL != nil && c.Request.URL.Scheme != "" {
		return strings.ToLower(c.Request.URL.Scheme)
	}
	return "http"
}

func isTrustedRequestHost(host string) bool {
	host = normalizeHostForTrust(host)
	if host == "" {
		return false
	}
	if model.GetSiteByDomainCached(host) != nil {
		return true
	}
	serverHost := normalizeHostForTrust(systemServerAddressHost())
	return serverHost != "" && host == serverHost
}

func normalizeHostForTrust(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.TrimSuffix(host, ".")
	return host
}

func systemServerAddressHost() string {
	if system_setting.ServerAddress == "" {
		return ""
	}
	parsed, err := url.Parse(system_setting.ServerAddress)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	trimmed := strings.TrimSpace(system_setting.ServerAddress)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		return ""
	}
	return trimmed
}

func fallbackServerAddress() string {
	return strings.TrimRight(system_setting.ServerAddress, "/")
}
