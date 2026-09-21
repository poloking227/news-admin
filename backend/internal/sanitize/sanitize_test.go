package sanitize

import (
	"strings"
	"testing"
)

func TestBodyHTMLStripsXSS(t *testing.T) {
	raw := `<p>healthy <strong>text</strong></p><script>alert(1)</script><img src="x" onerror="alert(2)"><p>styled text</p><iframe src="evil"></iframe><a href="javascript:alert(3)">link</a><a href="https://ok.example.com">ok</a>`
	got := BodyHTML(raw)

	if strings.Contains(got, "<script") || strings.Contains(got, "<iframe") || strings.Contains(got, "onerror") {
		t.Errorf("script/iframe/event handler survived: %s", got)
	}
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: URL survived: %s", got)
	}
	if strings.Contains(got, `style=`) {
		t.Errorf("style attribute survived: %s", got)
	}
	if !strings.Contains(got, "<strong>text</strong>") {
		t.Errorf("whitelisted <strong> lost: %s", got)
	}
	if !strings.Contains(got, `href="https://ok.example.com"`) {
		t.Errorf("safe https link lost: %s", got)
	}
}

func TestBodyHTMLAllowsRichText(t *testing.T) {
	raw := `<h2>标题</h2><ul><li><em>em</em></li><li>item</li></ul><blockquote>quote</blockquote><p><img src="https://cdn.example.com/a.jpg" alt="pic"></p>`
	got := BodyHTML(raw)
	for _, want := range []string{"<h2>", "<ul>", "<li>", "<em>em</em>", "<blockquote>", `alt="pic"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing whitelisted element %q: %s", want, got)
		}
	}
}

func TestToTextExtractsVisibleText(t *testing.T) {
	html := "<p>Hello <strong>world</strong></p><ul><li>one</li></ul>"
	want := "Hello worldone"
	if got := ToText(html); got != want {
		t.Errorf("ToText() = %q, want %q", got, want)
	}
}
