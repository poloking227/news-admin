// Package sanitize provides the server-side HTML whitelist policy for article
// body content: rich-text tags survive, everything else including scripts,
// styles, and event attributes is stripped. The whitelist mirrors the
// frontend editor constraints so that rendered content stays consistent
// across both sides.
package sanitize

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// policy is the shared article-content policy. It permits the rich-text
// surface plus safe links and images; inline styles and event handlers are
// always removed, and link/image URLs are restricted to http(s).
var policy = bluemonday.NewPolicy().
	AllowElements(
		"p", "br", "strong", "b", "em", "i", "u", "s",
		"ul", "ol", "li", "blockquote", "pre", "code",
		"h1", "h2", "h3", "h4", "h5", "h6",
	).
	AllowAttrs("href").OnElements("a").
	AllowURLSchemes("http", "https").
	AllowAttrs("src", "alt", "title").OnElements("img")

// BodyHTML strips anything outside the whitelist from the raw editor input.
func BodyHTML(raw string) string {
	return policy.Sanitize(raw)
}

// ToText extracts the visible text of an HTML fragment by walking its token
// stream and concatenating text nodes (the searchable body_text is derived
// from the sanitized HTML).
func ToText(htmlContent string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(htmlContent))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
		}
	}
}
