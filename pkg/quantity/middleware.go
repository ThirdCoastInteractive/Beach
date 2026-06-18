package quantity

import (
	"context"
	"net/http"
	"strings"
)

// ctxKey is the unexported context key under which the active unit system is
// stored. Middleware sets it; UnitSystem reads it. Mirrors pkg/i18n.
type ctxKey struct{}

// CookieName is the cookie consulted when resolving a request's unit system. Its
// value is "metric" or "imperial" (see System.String).
const CookieName = "unit_system"

// WithUnitSystem returns a copy of ctx carrying sys as the active unit system,
// which Format reads to choose a display unit. Stored quantities are unaffected.
func WithUnitSystem(ctx context.Context, sys System) context.Context {
	return context.WithValue(ctx, ctxKey{}, sys)
}

// UnitSystem returns the active unit system carried by ctx, defaulting to Metric
// when unset. It is the exported reader paired with WithUnitSystem.
func UnitSystem(ctx context.Context) System {
	if ctx == nil {
		return Metric
	}
	if v, ok := ctx.Value(ctxKey{}).(System); ok {
		return v
	}
	return Metric
}

// Middleware resolves the request's unit system — cookie first, then a coarse
// guess from Accept-Language, then Metric — and stores it on the request context
// for Format to read. It is a plain func(http.Handler) http.Handler, modelled on
// i18n's middleware, so apps that never set a preference pay nothing: Format
// falls back to Metric and the feature is inert.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithUnitSystem(r.Context(), Resolve(r))))
	})
}

// Resolve picks the unit system for r: an explicit, recognised cookie wins;
// otherwise a guess from the primary Accept-Language region (the handful of
// locales that conventionally use imperial units); otherwise Metric.
func Resolve(r *http.Request) System {
	if ck, err := r.Cookie(CookieName); err == nil {
		if sys, ok := ParseSystem(ck.Value); ok {
			return sys
		}
	}
	if sys, ok := systemFromAcceptLanguage(r.Header.Get("Accept-Language")); ok {
		return sys
	}
	return Metric
}

// ParseSystem maps a cookie or form value to a System, case-insensitively. ok
// reports whether the value was recognised. It is the inverse of System.String.
func ParseSystem(v string) (System, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "metric":
		return Metric, true
	case "imperial":
		return Imperial, true
	default:
		return Metric, false
	}
}

// imperialRegions are the locale region subtags that conventionally display
// imperial units. The list is deliberately short; the cookie is the real
// signal, this is only a first-visit default.
var imperialRegions = map[string]bool{
	"US": true, // United States
	"LR": true, // Liberia
	"MM": true, // Myanmar
}

// systemFromAcceptLanguage inspects the highest-priority language tag's region
// subtag and returns Imperial for the imperial regions. It is intentionally
// simple — full q-weight parsing lives in pkg/i18n; here the first tag's region
// is a good-enough default before the user sets a cookie.
func systemFromAcceptLanguage(header string) (System, bool) {
	if header == "" {
		return Metric, false
	}
	first := header
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	// Region is the subtag after the first '-', e.g. "en-US" -> "US".
	parts := strings.Split(first, "-")
	if len(parts) < 2 {
		return Metric, false
	}
	region := strings.ToUpper(parts[len(parts)-1])
	if imperialRegions[region] {
		return Imperial, true
	}
	return Metric, false
}
