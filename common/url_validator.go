package common

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// trustedRedirectDomainPattern requires at least two dot-separated labels of
// valid hostname characters (letters/digits/hyphen, no leading/trailing hyphen
// per label). This rejects trust entries that would dangerously widen the
// allowlist: bare single labels ("com", "localhost"), leading/trailing dots
// (".com"), wildcards ("*.example.com"), and anything carrying a scheme, port,
// path, userinfo, or whitespace. It intentionally still accepts multi-label
// public suffixes (e.g. "co.uk") — an admin-only, deliberate misconfiguration
// that a full public-suffix list would be needed to catch.
var trustedRedirectDomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// IsValidTrustedRedirectDomain reports whether entry (already lowercased) is a
// well-formed registrable domain safe to add to the trusted redirect allowlist.
func IsValidTrustedRedirectDomain(entry string) bool {
	return len(entry) <= 253 && trustedRedirectDomainPattern.MatchString(entry)
}

// ValidateRedirectURL validates that a redirect URL is safe to use.
// It checks that:
//   - The URL is properly formatted
//   - The scheme is either http or https
//   - The domain is in the trusted domains list (exact match or subdomain)
//
// Returns nil if the URL is valid and trusted, otherwise returns an error
// describing why the validation failed.
func ValidateRedirectURL(rawURL string) error {
	// Parse the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %s", err.Error())
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: only http and https are allowed")
	}

	domain := strings.ToLower(parsedURL.Hostname())

	if MatchesTrustedRedirectDomain(domain) {
		return nil
	}

	return fmt.Errorf("domain %s is not in the trusted domains list", domain)
}

// MatchesTrustedRedirectDomain reports whether host (a lowercase hostname
// without port) is covered by the trusted redirect domain lists — the
// env-provided TRUSTED_REDIRECT_DOMAINS and the admin-configured
// TrustedRedirectDomains option. A host matches a trusted domain exactly or
// as one of its subdomains.
func MatchesTrustedRedirectDomain(host string) bool {
	for _, list := range constant.TrustedRedirectDomainLists() {
		for _, trustedDomain := range list {
			if host == trustedDomain || strings.HasSuffix(host, "."+trustedDomain) {
				return true
			}
		}
	}
	return false
}

// ParseDomainList splits a comma/semicolon/whitespace/newline separated domain
// list into trimmed, lowercased, validated entries. Used for both the
// TRUSTED_REDIRECT_DOMAINS env variable and the TrustedRedirectDomains option.
// Malformed entries (bare TLDs, leading dots, wildcards, entries with a scheme
// or path) are dropped so a careless value cannot widen the trust allowlist to
// an entire public suffix — see IsValidTrustedRedirectDomain.
func ParseDomainList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	var domains []string
	for _, field := range fields {
		domain := strings.ToLower(strings.TrimSpace(field))
		if IsValidTrustedRedirectDomain(domain) {
			domains = append(domains, domain)
		}
	}
	return domains
}
