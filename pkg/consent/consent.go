// Package consent is the first-party cookie-preference record.
//
// Necessary cookies (session, CSRF, this record) run without a prompt and are
// not stored in the value. Optional categories stay off until the visitor
// allows them. Global Privacy Control and DNT win over an "allow" click:
// optional categories stay off, and the prompt is skipped.
//
// The cookie value is `v{version}` followed by zero or more `.{slug}=1` or
// `.{slug}=0` pairs, for example `v1.analytics=1`. Unknown or empty values are
// unset. Bumping [Config.Version] treats older cookies as unset so the prompt
// can return when a new optional category is introduced.
package consent

import (
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCookieName = "beach_consent"
	defaultVersion    = 1
	defaultMaxAge     = 180 * 24 * time.Hour
)

// Config names the preference cookie and how long a recorded choice lasts.
// The zero value is usable: name "beach_consent", version 1, 180-day MaxAge.
type Config struct {
	CookieName string        // default "beach_consent"
	Version    int           // default 1; bump treats old cookies as unset
	MaxAge     time.Duration // default 180 days
}

func (c Config) withDefaults() Config {
	if c.CookieName == "" {
		c.CookieName = defaultCookieName
	}
	if c.Version == 0 {
		c.Version = defaultVersion
	}
	if c.MaxAge == 0 {
		c.MaxAge = defaultMaxAge
	}
	return c
}

// Choice is the visitor's recorded preference. Set is false when they have
// not chosen yet (and GPC/DNT have not already answered for them).
type Choice struct {
	Set bool
	// Allow is the recorded state of optional category slugs. Necessary is
	// always on and not stored. A missing slug is denied.
	Allow map[string]bool
}

// Parse decodes the cookie value. Unknown or empty values are unset. A value
// whose version does not match is unset.
func Parse(raw string, version int) Choice {
	raw = strings.TrimSpace(raw)
	if raw == "" || version <= 0 || raw[0] != 'v' {
		return Choice{}
	}
	rest := raw[1:]
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 {
		return Choice{}
	}
	verStr := rest[:i]
	if verStr[0] == '0' && len(verStr) > 1 {
		return Choice{}
	}
	ver, err := strconv.Atoi(verStr)
	if err != nil || ver != version {
		return Choice{}
	}
	rest = rest[i:]
	allow := make(map[string]bool)
	for rest != "" {
		if rest[0] != '.' {
			return Choice{}
		}
		rest = rest[1:]
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			return Choice{}
		}
		slug := rest[:eq]
		if !validSlug(slug) {
			return Choice{}
		}
		rest = rest[eq+1:]
		if rest == "" {
			return Choice{}
		}
		var allowed bool
		switch rest[0] {
		case '1':
			allowed = true
		case '0':
			allowed = false
		default:
			return Choice{}
		}
		rest = rest[1:]
		if rest != "" && rest[0] != '.' {
			return Choice{}
		}
		if _, dup := allow[slug]; dup {
			return Choice{}
		}
		allow[slug] = allowed
	}
	return Choice{Set: true, Allow: allow}
}

func validSlug(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '_'):
		default:
			return false
		}
	}
	return true
}

// Encode serializes a recorded choice for version. Keys are sorted so the
// value is stable. A nil or empty map is `v{version}` with no pairs.
func Encode(allow map[string]bool, version int) string {
	var b strings.Builder
	b.WriteByte('v')
	b.WriteString(strconv.Itoa(version))
	if len(allow) == 0 {
		return b.String()
	}
	keys := make([]string, 0, len(allow))
	for k := range allow {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b.WriteByte('.')
		b.WriteString(k)
		b.WriteByte('=')
		if allow[k] {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

// FromRequest reads the preference cookie. Missing or malformed is unset.
func FromRequest(r *http.Request, cfg Config) Choice {
	if r == nil {
		return Choice{}
	}
	cfg = cfg.withDefaults()
	c, err := r.Cookie(cfg.CookieName)
	if err != nil || c == nil {
		return Choice{}
	}
	return Parse(c.Value, cfg.Version)
}

// BrowserOptOut reports a Global Privacy Control or DNT signal.
func BrowserOptOut(r *http.Request) bool {
	if r == nil {
		return false
	}
	return r.Header.Get("Sec-GPC") == "1" || r.Header.Get("DNT") == "1"
}

// Allows is true only when the visitor has recorded yes for category and the
// browser has not sent GPC or DNT. It is false if the choice is unset or
// category is missing from Allow.
func Allows(r *http.Request, cfg Config, category string) bool {
	if BrowserOptOut(r) {
		return false
	}
	c := FromRequest(r, cfg)
	return c.Set && c.Allow[category]
}

// NeedsPrompt is true when the first-visit prompt should show. GPC and DNT
// count as a recorded no, so the prompt is skipped. A recorded Choice.Set
// also skips it.
func NeedsPrompt(r *http.Request, cfg Config) bool {
	if r == nil {
		return true
	}
	if BrowserOptOut(r) {
		return false
	}
	return !FromRequest(r, cfg).Set
}

// DomainForHost picks the cookie Domain attribute so preference is shared
// across subdomains of the same site, matching the session cookie. Localhost,
// IPs, and single-label hosts get a host-only cookie (empty Domain).
func DomainForHost(configured, host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" || !strings.Contains(host, ".") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return ""
	}
	if configured != "" {
		base := strings.TrimPrefix(configured, ".")
		if host == base || strings.HasSuffix(host, "."+base) {
			return configured
		}
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return "." + parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return ""
}

// Cookie builds the preference Set-Cookie.
func Cookie(cfg Config, allow map[string]bool, domain string, secure bool) *http.Cookie {
	cfg = cfg.withDefaults()
	return &http.Cookie{
		Name:     cfg.CookieName,
		Value:    Encode(allow, cfg.Version),
		Path:     "/",
		Domain:   domain,
		MaxAge:   int(cfg.MaxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ExpireCookie builds a deletion Set-Cookie for name on the same domain.
func ExpireCookie(name, domain string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}
