package beach

import (
	"sort"
	"strings"
)

// CSP is a small, composable Content-Security-Policy builder. Beach pages are
// HTML-and-CSS-first with Datastar as the only script, so the default is strict:
// self for everything, no inline script. Apps widen specific directives for the
// genuinely needed (e.g. a video CDN for media) via the Allow* methods, which
// return a copy so a base preset can be shared and extended without mutation.
type CSP struct {
	// directives maps a CSP directive name (e.g. "media-src") to its source list.
	directives map[string][]string
}

// CSPDefault is the strict baseline: everything from 'self', images and media
// also allow data: URIs, styles allow 'unsafe-inline' (components emit inline
// style attributes for reserved dimensions and token colors), and object/frame
// are denied. script-src includes 'unsafe-eval' because Datastar — the
// framework's one client dependency — evaluates its `data-*` expressions
// (`@post(...)`, signals) with the Function constructor; without it every
// Datastar action throws. No external origins; the Datastar client is served
// from 'self'. Apps widen from here.
func CSPDefault() CSP {
	return CSP{directives: map[string][]string{
		"default-src":     {"'self'"},
		"script-src":      {"'self'", "'unsafe-eval'"},
		"style-src":       {"'self'", "'unsafe-inline'"},
		"img-src":         {"'self'", "data:"},
		"media-src":       {"'self'"},
		"font-src":        {"'self'"},
		"connect-src":     {"'self'"},
		"frame-ancestors": {"'none'"},
		"object-src":      {"'none'"},
		"base-uri":        {"'self'"},
		"form-action":     {"'self'"},
	}}
}

// IsZero reports whether the CSP is the zero value (no directives), used by the
// builder to fall back to CSPDefault.
func (c CSP) IsZero() bool { return len(c.directives) == 0 }

// clone returns a deep copy so Allow* never mutates a shared base preset.
func (c CSP) clone() CSP {
	out := CSP{directives: make(map[string][]string, len(c.directives))}
	for k, v := range c.directives {
		cp := make([]string, len(v))
		copy(cp, v)
		out.directives[k] = cp
	}
	return out
}

// add appends sources to a directive (creating it if absent), de-duplicating.
func (c CSP) add(directive string, sources ...string) CSP {
	out := c.clone()
	have := map[string]bool{}
	for _, s := range out.directives[directive] {
		have[s] = true
	}
	for _, s := range sources {
		if !have[s] {
			out.directives[directive] = append(out.directives[directive], s)
			have[s] = true
		}
	}
	return out
}

// Allow appends sources to an arbitrary directive. The named-directive helpers
// below cover the common cases; this is the general form.
func (c CSP) Allow(directive string, sources ...string) CSP {
	return c.add(directive, sources...)
}

// AllowMedia widens media-src (audio/video), e.g. a Cloudflare Stream origin.
func (c CSP) AllowMedia(sources ...string) CSP { return c.add("media-src", sources...) }

// AllowImg widens img-src.
func (c CSP) AllowImg(sources ...string) CSP { return c.add("img-src", sources...) }

// AllowConnect widens connect-src (fetch / SSE / websocket endpoints).
func (c CSP) AllowConnect(sources ...string) CSP { return c.add("connect-src", sources...) }

// AllowScript widens script-src. Reach for this only for a genuinely required
// third-party script; the house style is Datastar-only.
func (c CSP) AllowScript(sources ...string) CSP { return c.add("script-src", sources...) }

// AllowFrame widens frame-src (embedded iframes).
func (c CSP) AllowFrame(sources ...string) CSP { return c.add("frame-src", sources...) }

// String renders the policy as a single header value, directives sorted for
// deterministic output (stable tests, stable diffs).
func (c CSP) String() string {
	if len(c.directives) == 0 {
		return CSPDefault().String()
	}
	names := make([]string, 0, len(c.directives))
	for k := range c.directives {
		names = append(names, k)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		srcs := c.directives[name]
		if len(srcs) == 0 {
			parts = append(parts, name)
			continue
		}
		parts = append(parts, name+" "+strings.Join(srcs, " "))
	}
	return strings.Join(parts, "; ")
}
