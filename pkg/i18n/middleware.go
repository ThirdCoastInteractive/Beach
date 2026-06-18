package i18n

import (
	"net/http"
	"strconv"
	"strings"
)

// CookieName is the cookie consulted first when resolving a request's locale.
const CookieName = "locale"

// Middleware resolves the request locale — cookie, then Accept-Language, then
// the catalog's default — and stores both the locale and c itself on the
// request context. Carrying c lets the package-level T resolve this catalog's
// strings without an app threading its own *Catalog through handlers. An App
// with no configured locales never installs this; T then resolves against the
// package default and the feature is inert.
func (c *Catalog) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := c.Resolve(r)
		ctx := WithLocale(r.Context(), tag)
		ctx = WithCatalog(ctx, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Resolve picks the best locale tag for r: an explicit, recognized cookie wins;
// otherwise the highest-quality Accept-Language entry that maps to a loaded
// locale; otherwise the default.
func (c *Catalog) Resolve(r *http.Request) string {
	if ck, err := r.Cookie(CookieName); err == nil && ck.Value != "" {
		if tag, ok := c.resolveTag(ck.Value); ok {
			return tag
		}
	}
	if tag, ok := c.matchAcceptLanguage(r.Header.Get("Accept-Language")); ok {
		return tag
	}
	return c.def
}

// matchAcceptLanguage parses an Accept-Language header and returns the loaded
// locale tag with the highest q-weight, honoring header order as a tiebreak.
func (c *Catalog) matchAcceptLanguage(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	bestTag := ""
	bestQ := -1.0
	for i, part := range strings.Split(header, ",") {
		lang, q := parseLangQ(part)
		if lang == "" || lang == "*" {
			continue
		}
		tag, ok := c.resolveTag(lang)
		if !ok {
			continue
		}
		// Subtract a tiny epsilon by index so earlier entries win ties without
		// disturbing distinct q values.
		score := q - float64(i)*1e-6
		if score > bestQ {
			bestQ, bestTag = score, tag
		}
	}
	if bestTag == "" {
		return "", false
	}
	return bestTag, true
}

// parseLangQ splits one Accept-Language element into its language tag and
// q-weight (default 1.0).
func parseLangQ(part string) (lang string, q float64) {
	q = 1.0
	fields := strings.Split(strings.TrimSpace(part), ";")
	lang = strings.TrimSpace(fields[0])
	for _, f := range fields[1:] {
		f = strings.TrimSpace(f)
		if v, ok := strings.CutPrefix(f, "q="); ok {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				q = parsed
			}
		}
	}
	return lang, q
}
