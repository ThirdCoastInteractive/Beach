// Package md renders Markdown to sanitized HTML.
//
// [Render] and [RenderWith] convert CommonMark (plus GFM tables and autolinks)
// through goldmark, then a bluemonday policy selected by [Profile]:
//
//   - Strict: no tags
//   - Post: short-form (em, strong, lists, p, br)
//   - Article: headings, links, CF Images, code, quotes, tables, hr, and
//     Cloudflare Stream iframes from `:::video <uid>` fences
package md

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// Profile selects the sanitizer allow-list applied after goldmark.
type Profile int

const (
	// Strict strips every tag.
	Strict Profile = iota
	// Post is the short-form subset: em, strong, ul, ol, li, p, br.
	Post
	// Article is the blog subset: Post plus headings, links, images, code,
	// blockquote, tables, hr, and Cloudflare Stream iframes.
	Article
)

const defaultImagesHost = "imagedelivery.net"

// Options configures rendering. Zero-value ImagesHost means imagedelivery.net.
// An empty StreamCustomer drops `:::video` blocks instead of emitting iframes.
type Options struct {
	Profile        Profile
	StreamCustomer string // Cloudflare Stream customer code (the bit after "customer-")
	ImagesHost     string // img src host allow-list; default imagedelivery.net
}

var (
	tokenRE     = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	streamSrc   = regexp.MustCompile(`^https://customer-[A-Za-z0-9-]+\.cloudflarestream\.com/[A-Za-z0-9-]+/iframe$`)
	streamAllow = regexp.MustCompile(`^accelerometer; gyroscope; autoplay; encrypted-media; picture-in-picture;?$`)
	allowFull   = regexp.MustCompile(`(?i)^(|true|allowfullscreen)$`)

	strictPolicy = bluemonday.StrictPolicy()
	postPolicy   = bluemonday.NewPolicy().
			AllowElements("em", "strong", "ul", "ol", "li", "p", "br")
)

// Render converts src to sanitized HTML for profile p.
func Render(src string, p Profile) (string, error) {
	return RenderWith(src, Options{Profile: p})
}

// RenderWith converts src to sanitized HTML using opt.
func RenderWith(src string, opt Options) (string, error) {
	opt.StreamCustomer = strings.TrimSpace(opt.StreamCustomer)
	opt.ImagesHost = strings.TrimSpace(opt.ImagesHost)
	md := goldmark.New(
		goldmark.WithExtensions(
			gfm{},
			videoExt{customer: opt.StreamCustomer},
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	out := string(policy(opt).SanitizeBytes(buf.Bytes()))
	return dropImgsWithoutSrc(out), nil
}

// imgTag matches a single <img> start tag so we can drop ones that lost their src.
var imgTag = regexp.MustCompile(`(?i)<img\b[^>]*>`)

func dropImgsWithoutSrc(html string) string {
	return imgTag.ReplaceAllStringFunc(html, func(t string) string {
		if strings.Contains(strings.ToLower(t), "src=") {
			return t
		}
		return ""
	})
}

func policy(opt Options) *bluemonday.Policy {
	switch opt.Profile {
	case Post:
		return postPolicy
	case Article:
		return articlePolicy(opt.ImagesHost)
	default:
		return strictPolicy
	}
}

func articlePolicy(imagesHost string) *bluemonday.Policy {
	if imagesHost == "" {
		imagesHost = defaultImagesHost
	}
	imgSrc := regexp.MustCompile(`(?i)^https://` + regexp.QuoteMeta(imagesHost) + `(/|$)`)
	p := bluemonday.NewPolicy()
	p.AllowElements("em", "strong", "ul", "ol", "li", "p", "br")
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowElements("pre", "code", "blockquote", "hr")
	p.AllowElements("table", "thead", "tbody", "tr", "th", "td")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowAttrs("src").Matching(imgSrc).OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowAttrs("src").Matching(streamSrc).OnElements("iframe")
	p.AllowAttrs("allow").Matching(streamAllow).OnElements("iframe")
	p.AllowAttrs("allowfullscreen").Matching(allowFull).OnElements("iframe")
	p.AllowAttrs("loading").Matching(regexp.MustCompile(`^(lazy|eager)$`)).OnElements("iframe", "img")
	return p
}
