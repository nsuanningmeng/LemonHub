package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkdownToEmailHTML_Contract locks the security/escaping contract of the
// email markdown renderer: raw HTML/script is neutralized, only http(s)/mailto
// links survive, and the supported markdown constructs render to fixed tags.
func TestMarkdownToEmailHTML_Contract(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantExact   string   // when set, the full output must match exactly
		mustContain []string // substrings that must appear
		mustReject  []string // substrings that must NOT appear
	}{
		{
			name:        "raw script tag is html-escaped, never live",
			in:          "<script>alert(1)</script>",
			wantExact:   "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>",
			mustContain: []string{"&lt;script&gt;"},
			mustReject:  []string{"<script>", "</script>"},
		},
		{
			name:       "javascript link scheme renders as inert text, no anchor",
			in:         "[x](javascript:alert(1))",
			mustReject: []string{`href="javascript:`, "<a ", "javascript:"},
		},
		{
			name:        "https link becomes a safe anchor",
			in:          "[x](https://a.com)",
			wantExact:   `<p><a href="https://a.com" target="_blank" rel="noopener noreferrer">x</a></p>`,
			mustContain: []string{`<a href="https://a.com"`, `rel="noopener noreferrer"`},
		},
		{
			name:        "bold renders to strong",
			in:          "**bold**",
			wantExact:   "<p><strong>bold</strong></p>",
			mustContain: []string{"<strong>bold</strong>"},
		},
		{
			name:        "atx heading renders to h1",
			in:          "# H",
			wantExact:   "<h1>H</h1>",
			mustContain: []string{"<h1>H</h1>"},
		},
		{
			name:        "img/onerror injection cannot break out into a real tag",
			in:          "<img src=x onerror=alert(1)>",
			wantExact:   "<p>&lt;img src=x onerror=alert(1)&gt;</p>",
			mustContain: []string{"&lt;img"},
			mustReject:  []string{"<img", "<img src"},
		},
		{
			name:        "html in link text is escaped, not emitted as a tag",
			in:          "[<b>x</b>](https://a.com)",
			mustContain: []string{`<a href="https://a.com"`, "&lt;b&gt;x&lt;/b&gt;"},
			mustReject:  []string{"<b>x</b>"},
		},
		{
			name:        "quote in url cannot break the href attribute (stays encoded)",
			in:          `[x](https://a.com"onerror=)`,
			mustContain: []string{"&#34;onerror="},
			// a raw double-quote followed by onerror would escape the attribute.
			mustReject: []string{`"onerror=`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MarkdownToEmailHTML(tc.in)
			if tc.wantExact != "" {
				assert.Equal(t, tc.wantExact, got)
			}
			for _, s := range tc.mustContain {
				assert.Containsf(t, got, s, "output %q must contain %q", got, s)
			}
			for _, s := range tc.mustReject {
				assert.NotContainsf(t, got, s, "output %q must NOT contain %q", got, s)
			}
		})
	}
}

// TestWrapEmailHTML_EscapesTitle ensures the document title is HTML-escaped while
// the already-rendered body HTML is embedded verbatim.
func TestWrapEmailHTML_EscapesTitle(t *testing.T) {
	body := "<p>hello &amp; welcome</p>"
	out := WrapEmailHTML(`<script>"x"`, body)

	require.Contains(t, out, "<!DOCTYPE html>")
	// Title must be escaped, not a live tag.
	assert.Contains(t, out, "<title>&lt;script&gt;&#34;x&#34;</title>")
	assert.NotContains(t, out, "<title><script>")
	// The pre-rendered body must be embedded as-is (caller already sanitized it).
	assert.Contains(t, out, body)
}

// TestMarkdownToEmailHTML_NoScriptAcrossSamples is a focused regression guard that
// no sample input can ever yield a live <script> element.
func TestMarkdownToEmailHTML_NoLiveScript(t *testing.T) {
	samples := []string{
		"plain",
		"<script>x</script>",
		"# <script>x</script>",
		"- <script>x</script>",
		"**<script>**",
		"[t](https://a.com)<script>x</script>",
	}
	for _, s := range samples {
		out := MarkdownToEmailHTML(s)
		assert.NotContains(t, strings.ToLower(out), "<script", "input %q produced a live script tag: %q", s, out)
	}
}
