package model

import "testing"

// TestNormalizeDomain locks down the host→domain reduction that drives sub-site
// (white-label) routing. A mismatch here silently falls back to the main site, so
// the edge cases (ports, schemes, paths, IPv6, trailing dots, case) are covered.
func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"Example.COM", "example.com"},
		{"example.com", "example.com"},
		{"example.com:3000", "example.com"},
		{"example.com.", "example.com"},          // trailing FQDN dot
		{"example.com:8080/", "example.com"},     // path
		{"https://example.com", "example.com"},   // scheme
		{"https://example.com/path?x=1", "example.com"},
		{"http://user:pass@example.com:9000/p", "example.com"}, // userinfo + port + path
		{"  Sub.Example.Com:443  ", "sub.example.com"},
		{"[::1]:8443", "::1"},   // bracketed IPv6 with port
		{"[2001:db8::1]", "2001:db8::1"}, // bracketed IPv6 no port
		{"127.0.0.1:3000", "127.0.0.1"},
	}
	for _, c := range cases {
		if got := NormalizeDomain(c.in); got != c.want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeDomainsDedup verifies normalization + order-preserving dedup used
// when binding domains to a site.
func TestNormalizeDomainsDedup(t *testing.T) {
	in := []string{"A.com", "b.com:80", "a.com", "", "  ", "B.COM"}
	got := normalizeDomains(in)
	want := []string{"a.com", "b.com"}
	if len(got) != len(want) {
		t.Fatalf("normalizeDomains(%v) = %v, want %v", in, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeDomains(%v) = %v, want %v", in, got, want)
		}
	}
}
