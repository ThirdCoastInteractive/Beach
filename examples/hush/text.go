package main

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// maxPlainRunes is the post-length budget, counted on the rendered plain text so
// markup never costs the author characters: **bold** is 4 visible runes, not 8.
const maxPlainRunes = 512

// mdPolicy is the deliberately tiny formatting allow-list: bold, italics, lists,
// paragraphs, line breaks. Everything that controls layout (headings, links,
// images, blockquotes, code blocks, ...) is absent, so the sanitizer drops it —
// "just helpful things," enforced by omission rather than a rule list.
var mdPolicy = bluemonday.NewPolicy().
	AllowElements("em", "strong", "ul", "ol", "li", "p", "br")

// stripPolicy removes every tag, leaving plain text. We count its output so the
// 512-rune budget measures what a reader sees, not the markup.
var stripPolicy = bluemonday.StrictPolicy()

// renderPost converts raw markdown to sanitized HTML and reports whether it is
// postable: non-empty and within the plain-text budget. Any UTF-8 is accepted;
// goldmark renders what it can (raw HTML in the source is dropped by default),
// and mdPolicy keeps only the allowed tags.
func renderPost(raw string) (html string, ok bool) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(raw), &buf); err != nil {
		return "", false
	}
	safe := mdPolicy.SanitizeBytes(buf.Bytes())
	plain := strings.TrimSpace(stripPolicy.Sanitize(string(safe)))
	n := utf8.RuneCountInString(plain)
	if n == 0 || n > maxPlainRunes {
		return "", false
	}
	return string(safe), true
}
