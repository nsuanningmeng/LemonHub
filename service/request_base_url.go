package service

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
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
		if clean := sanitizeRequestHost(host); clean != "" {
			return fmt.Sprintf("%s://%s", scheme, clean)
		}
	}
	return fallbackServerAddress()
}

// sanitizeRequestHost reduces a Host header to a clean host[:port] authority that
// is safe to embed in a generated URL: lowercased, with any scheme/userinfo/path/
// query/fragment stripped, the trailing FQDN dot removed, and the port kept only
// when numeric. The host placed into the base URL is therefore exactly the
// registered domain that passed the trust whitelist — never raw, attacker-
// influenced bytes (the whitelist check itself normalizes, so the two must agree).
func sanitizeRequestHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if i := strings.Index(host, "://"); i != -1 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i != -1 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, "@"); i != -1 {
		host = host[i+1:]
	}
	hostname, port := host, ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		hostname, port = h, p
	}
	hostname = strings.TrimPrefix(hostname, "[")
	hostname = strings.TrimSuffix(hostname, "]")
	hostname = strings.TrimSuffix(hostname, ".")
	if hostname == "" {
		return ""
	}
	if strings.Contains(hostname, ":") { // IPv6 literal → re-bracket for URL authority
		hostname = "[" + hostname + "]"
	}
	if port != "" {
		if _, err := strconv.Atoi(port); err == nil {
			return hostname + ":" + port
		}
	}
	return hostname
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
	if proto := normalizeScheme(firstHeaderValue(c.GetHeader("X-Forwarded-Proto"))); proto != "" {
		return proto
	}
	if proto := normalizeScheme(firstHeaderValue(c.GetHeader("X-Forwarded-Protocol"))); proto != "" {
		return proto
	}
	if c.Request.TLS != nil {
		return "https"
	}
	if c.Request.URL != nil {
		if proto := normalizeScheme(c.Request.URL.Scheme); proto != "" {
			return proto
		}
	}
	return "http"
}

// normalizeScheme accepts ONLY the exact tokens "http" / "https" (case-insensitive),
// returning "" for anything else. This stops a forged X-Forwarded-Proto such as
// "https://evil.example" from polluting a generated base URL's authority (which a
// browser would otherwise re-parse as pointing at evil.example).
func normalizeScheme(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "https":
		return "https"
	case "http":
		return "http"
	default:
		return ""
	}
}

func isTrustedRequestHost(host string) bool {
	host = normalizeHostForTrust(host)
	if host == "" {
		return false
	}
	if model.GetSiteByDomainCached(host) != nil {
		return true
	}
	if common.MatchesTrustedRedirectDomain(host) {
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
