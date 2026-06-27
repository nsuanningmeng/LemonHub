package common

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// MarkdownToEmailHTML converts a limited, safe subset of Markdown into HTML
// suitable for transactional/marketing emails. It is intentionally conservative:
//
//   - The whole input is HTML-escaped FIRST, neutralizing any raw HTML / script
//     injection, then a small set of well-known Markdown constructs are
//     re-introduced as safe tags.
//   - Only http(s) and mailto link schemes are allowed; anything else (e.g.
//     javascript:) is rendered as plain text.
//
// This avoids pulling in a heavyweight Markdown dependency while keeping the
// output safe for admin-authored bulk email content.
func MarkdownToEmailHTML(md string) string {
	// Normalize newlines and escape everything up front.
	src := strings.ReplaceAll(md, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	lines := strings.Split(src, "\n")
	var b strings.Builder
	inList := false
	closeList := func() {
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			closeList()
			continue
		}

		// Unordered list item: "- ", "* ", "+ "
		if m := unorderedItemRe.FindStringSubmatch(trimmed); m != nil {
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>")
			b.WriteString(renderInline(m[1]))
			b.WriteString("</li>")
			continue
		}
		closeList()

		// Headings: #, ##, ### ...
		if m := headingRe.FindStringSubmatch(trimmed); m != nil {
			level := len(m[1])
			if level > 6 {
				level = 6
			}
			b.WriteString(fmt.Sprintf("<h%d>%s</h%d>", level, renderInline(m[2]), level))
			continue
		}

		// Plain paragraph line.
		b.WriteString("<p>")
		b.WriteString(renderInline(trimmed))
		b.WriteString("</p>")
	}
	closeList()

	return b.String()
}

var (
	headingRe        = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	unorderedItemRe  = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	boldRe           = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe         = regexp.MustCompile(`(^|[^*])\*([^*]+)\*`)
	inlineCodeRe     = regexp.MustCompile("`([^`]+)`")
	linkRe           = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	safeLinkSchemeRe = regexp.MustCompile(`(?i)^(https?://|mailto:)`)
)

// renderInline applies inline Markdown transforms to a single already-trusted
// text fragment. The fragment is HTML-escaped before any tag is introduced.
func renderInline(s string) string {
	escaped := html.EscapeString(s)

	// Links: [text](url) — validate scheme, escape both parts.
	escaped = linkRe.ReplaceAllStringFunc(escaped, func(match string) string {
		sub := linkRe.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		text := sub[1]
		// html.EscapeString turned & into &amp; inside the URL; decode for scheme check only.
		rawURL := html.UnescapeString(sub[2])
		if !safeLinkSchemeRe.MatchString(rawURL) {
			// Unsafe scheme: render as plain text (already escaped).
			return text
		}
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, sub[2], text)
	})

	// Inline code.
	escaped = inlineCodeRe.ReplaceAllString(escaped, "<code>$1</code>")
	// Bold.
	escaped = boldRe.ReplaceAllString(escaped, "<strong>$1</strong>")
	// Italic (avoid eating bold markers).
	escaped = italicRe.ReplaceAllString(escaped, "$1<em>$2</em>")

	return escaped
}

// WrapEmailHTML wraps rendered body HTML in a minimal responsive email shell.
func WrapEmailHTML(title string, bodyHTML string) string {
	safeTitle := html.EscapeString(title)
	return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8">`+
		`<meta name="viewport" content="width=device-width, initial-scale=1.0"><title>%s</title></head>`+
		`<body style="margin:0;padding:0;background:#f5f6f8;">`+
		`<div style="max-width:600px;margin:0 auto;padding:24px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2329;line-height:1.6;">`+
		`<div style="background:#ffffff;border-radius:8px;padding:24px;">%s</div>`+
		`</div></body></html>`, safeTitle, bodyHTML)
}
