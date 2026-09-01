package md

import (
	"strings"
	"testing"
)

type tc struct {
	src         string
	opt         Options
	contains    []string
	notContains []string
	empty       bool
}

func TestRender(t *testing.T) {
	cases := map[string]tc{
		"empty": {
			src:   "",
			opt:   Options{Profile: Article},
			empty: true,
		},
		"article bold": {
			src:      "**bold**",
			opt:      Options{Profile: Article},
			contains: []string{"<strong>bold</strong>"},
		},
		"article link": {
			src:      "[hi](https://example.com)",
			opt:      Options{Profile: Article},
			contains: []string{`<a href="https://example.com">hi</a>`},
		},
		"article heading": {
			src:      "# Title",
			opt:      Options{Profile: Article},
			contains: []string{"<h1>Title</h1>"},
		},
		"article table": {
			src: "| a | b |\n| --- | --- |\n| 1 | 2 |\n",
			opt: Options{Profile: Article},
			contains: []string{
				"<table>", "<thead>", "<th>", "a", "b",
				"<tbody>", "<td>", "1", "2", "</table>",
			},
		},
		"script tags dropped": {
			src:         "<script>alert(1)</script>",
			opt:         Options{Profile: Article},
			notContains: []string{"<script", "alert("},
		},
		"javascript link dropped": {
			src:         "[xss](javascript:alert(1))",
			opt:         Options{Profile: Article},
			contains:    []string{"xss"},
			notContains: []string{"javascript:", "alert("},
		},
		"foreign iframe dropped": {
			src:         `<iframe src="https://example.com/embed"></iframe>`,
			opt:         Options{Profile: Article},
			notContains: []string{"<iframe", "example.com/embed"},
		},
		"video with StreamCustomer kept": {
			src: ":::video abc123XYZ",
			opt: Options{Profile: Article, StreamCustomer: "acme"},
			contains: []string{
				"<iframe",
				`src="https://customer-acme.cloudflarestream.com/abc123XYZ/iframe"`,
				`allow="accelerometer; gyroscope; autoplay; encrypted-media; picture-in-picture;"`,
				"allowfullscreen",
				`loading="lazy"`,
			},
			notContains: []string{":::video"},
		},
		"video without StreamCustomer dropped": {
			src:         ":::video abc123XYZ",
			opt:         Options{Profile: Article},
			notContains: []string{"<iframe", "cloudflarestream", ":::video"},
		},
		"post drops headings and links": {
			src:         "# Title\n\n[click](https://example.com)\n",
			opt:         Options{Profile: Post},
			contains:    []string{"Title", "click"},
			notContains: []string{"<h1", "<a ", "href="},
		},
		"strict strips tags": {
			src:         "**bold** and [a](https://example.com)",
			opt:         Options{Profile: Strict},
			contains:    []string{"bold", "a"},
			notContains: []string{"<strong", "<a ", "<p>"},
		},
		"article image from ImagesHost kept": {
			src:      "![cat](https://imagedelivery.net/acct/id/public)",
			opt:      Options{Profile: Article},
			contains: []string{`<img src="https://imagedelivery.net/acct/id/public"`, `alt="cat"`},
		},
		"article foreign image dropped": {
			src:         "![x](https://evil.example/a.png)",
			opt:         Options{Profile: Article},
			notContains: []string{"<img", "evil.example"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := RenderWith(tc.src, tc.opt)
			if err != nil {
				t.Fatalf("RenderWith: %v", err)
			}
			if tc.empty {
				if got != "" {
					t.Fatalf("empty input: got %q", got)
				}
				return
			}
			for _, s := range tc.contains {
				if !strings.Contains(got, s) {
					t.Errorf("want contains %q\ngot %q", s, got)
				}
			}
			for _, s := range tc.notContains {
				if strings.Contains(got, s) {
					t.Errorf("want not contains %q\ngot %q", s, got)
				}
			}
		})
	}
}

func TestRenderConvenience(t *testing.T) {
	const src = "**hi**"
	a, err := Render(src, Article)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RenderWith(src, Options{Profile: Article})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("Render != RenderWith:\n%s\n%s", a, b)
	}
	if !strings.Contains(a, "<strong>hi</strong>") {
		t.Fatalf("got %q", a)
	}
}
