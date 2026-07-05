package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newCallbackCtx(host string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/api/user/epay", nil)
	req.Host = host
	c.Request = req
	return c
}

// Host that matches the configured ServerAddress host is trusted → the request's
// own scheme://host is used for the callback.
func TestGetCallbackAddressForRequest_TrustedServerHostUsesRequestHost(t *testing.T) {
	oldSA, oldCB := system_setting.ServerAddress, operation_setting.CustomCallbackAddress
	system_setting.ServerAddress = "https://main.example.com"
	operation_setting.CustomCallbackAddress = ""
	defer func() {
		system_setting.ServerAddress, operation_setting.CustomCallbackAddress = oldSA, oldCB
	}()

	got := GetCallbackAddressForRequest(newCallbackCtx("main.example.com"))
	// No TLS / no X-Forwarded-Proto on the synthetic request → scheme is http.
	if got != "http://main.example.com" {
		t.Fatalf("trusted host should use request host, got %q", got)
	}
}

// An untrusted (unregistered, non-ServerAddress) Host must NOT be reflected into
// the callback — it falls back to the configured address (Host-spoof defense).
func TestGetCallbackAddressForRequest_UntrustedHostFallsBack(t *testing.T) {
	oldSA, oldCB := system_setting.ServerAddress, operation_setting.CustomCallbackAddress
	system_setting.ServerAddress = "https://main.example.com"
	operation_setting.CustomCallbackAddress = ""
	defer func() {
		system_setting.ServerAddress, operation_setting.CustomCallbackAddress = oldSA, oldCB
	}()

	got := GetCallbackAddressForRequest(newCallbackCtx("evil.attacker.com"))
	if got != "https://main.example.com" {
		t.Fatalf("untrusted host should fall back to ServerAddress, got %q", got)
	}
}

// CustomCallbackAddress is honored for untrusted hosts, and a trailing slash is
// trimmed so callers can append "/api/..." without producing "//api".
func TestGetCallbackAddressForRequest_CustomCallbackTrimmed(t *testing.T) {
	oldSA, oldCB := system_setting.ServerAddress, operation_setting.CustomCallbackAddress
	system_setting.ServerAddress = "https://main.example.com"
	operation_setting.CustomCallbackAddress = "https://callback.example.com/"
	defer func() {
		system_setting.ServerAddress, operation_setting.CustomCallbackAddress = oldSA, oldCB
	}()

	got := GetCallbackAddressForRequest(newCallbackCtx("evil.attacker.com"))
	if got != "https://callback.example.com" {
		t.Fatalf("custom callback should be used and trimmed, got %q", got)
	}
}

// A forged X-Forwarded-Proto must NOT pollute the generated base URL's authority
// even when the Host itself is trusted (open-redirect / payment-URL-poisoning defense).
func TestGetRequestBaseURL_RejectsForgedForwardedProto(t *testing.T) {
	oldSA := system_setting.ServerAddress
	system_setting.ServerAddress = "https://tenant.example"
	defer func() { system_setting.ServerAddress = oldSA }()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Host = "tenant.example" // trusted: equals ServerAddress host
	req.Header.Set("X-Forwarded-Proto", "https://evil.example")
	c.Request = req

	got := GetRequestBaseURL(c)
	if strings.Contains(got, "evil.example") {
		t.Fatalf("forged X-Forwarded-Proto polluted base URL: %q", got)
	}
	if got != "http://tenant.example" {
		t.Fatalf("expected clean fallback scheme, got %q", got)
	}
}

// A well-formed X-Forwarded-Proto from the proxy is still honored.
func TestGetRequestBaseURL_HonorsValidForwardedProto(t *testing.T) {
	oldSA := system_setting.ServerAddress
	system_setting.ServerAddress = "https://tenant.example"
	defer func() { system_setting.ServerAddress = oldSA }()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Host = "tenant.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	c.Request = req

	if got := GetRequestBaseURL(c); got != "https://tenant.example" {
		t.Fatalf("valid forwarded proto not honored, got %q", got)
	}
}

// Hosts listed in TRUSTED_REDIRECT_DOMAINS (env) or the TrustedRedirectDomains
// option (admin-configured) are trusted → payment return URLs follow the domain
// the user is visiting (multi-domain deployments). Subdomains of a trusted
// domain match; unrelated lookalike hosts still fall back.
func TestGetRequestBaseURL_TrustedRedirectDomainsFollowRequestHost(t *testing.T) {
	oldSA := system_setting.ServerAddress
	system_setting.ServerAddress = "https://main.example.com"
	constant.SetTrustedRedirectDomains([]string{"alias.example.org"})
	constant.SetTrustedRedirectDomainsFromOption([]string{"panel.example.net"})
	defer func() {
		system_setting.ServerAddress = oldSA
		constant.SetTrustedRedirectDomains(nil)
		constant.SetTrustedRedirectDomainsFromOption(nil)
	}()

	assert.Equal(t, "http://alias.example.org", GetRequestBaseURL(newCallbackCtx("alias.example.org")))
	assert.Equal(t, "http://pay.alias.example.org", GetRequestBaseURL(newCallbackCtx("pay.alias.example.org")))
	assert.Equal(t, "http://panel.example.net", GetRequestBaseURL(newCallbackCtx("panel.example.net")))
	// Suffix must match on a label boundary: "evilalias.example.org" is not a subdomain.
	assert.Equal(t, "https://main.example.com", GetRequestBaseURL(newCallbackCtx("evilalias.example.org")))
}

// Nil context (non-request code paths) must not panic and must fall back.
func TestGetCallbackAddressForRequest_NilContext(t *testing.T) {
	oldSA, oldCB := system_setting.ServerAddress, operation_setting.CustomCallbackAddress
	system_setting.ServerAddress = "https://main.example.com/"
	operation_setting.CustomCallbackAddress = ""
	defer func() {
		system_setting.ServerAddress, operation_setting.CustomCallbackAddress = oldSA, oldCB
	}()

	if got := GetCallbackAddressForRequest(nil); got != "https://main.example.com" {
		t.Fatalf("nil context should fall back to trimmed ServerAddress, got %q", got)
	}
}
