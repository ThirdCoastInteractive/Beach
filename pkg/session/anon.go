package session

import (
	"context"
	"net/http"
)

// AnonStore is the cookie-only session store: a stable per-browser anonymous id
// carried in a cookie, with no database behind it. A DB-backed Store keeps the
// SHA256(token) row and the CSRF/RBAC machinery; this keeps nothing — the cookie
// value *is* the identity. It exists so a fully-ephemeral app (no Postgres) still
// gets a well-defined, stable identity per visitor instead of minting its own
// cookie by hand.
//
// The id is the same 32-byte crypto/rand token the DB store mints (base64url),
// but it is never persisted: there is no row to look up, so the cookie round-trips
// as the whole session.
type AnonStore struct {
	cfg AnonConfig
}

// AnonConfig configures an AnonStore. The zero value is usable: the cookie
// defaults match the DB store's (name "beach_session", path "/", a 14-day idle
// window refreshed on every visit).
type AnonConfig struct {
	CookieName string // e.g. "beach_anon"
	CookiePath string // e.g. "/"
	// MaxAge is the cookie's lifetime in seconds; it is re-set on every request so
	// an active visitor keeps a stable id. Zero falls back to 14 days.
	MaxAge int
	// Secure marks the cookie Secure (HTTPS only). Release builds set this; a dev
	// box on plain http leaves it false.
	Secure bool
}

const anonMaxAgeDefault = 14 * 24 * 60 * 60 // 14 days, in seconds.

// withDefaults fills zero fields so a zero AnonConfig works.
func (c AnonConfig) withDefaults() AnonConfig {
	if c.CookieName == "" {
		c.CookieName = "beach_session"
	}
	if c.CookiePath == "" {
		c.CookiePath = "/"
	}
	if c.MaxAge <= 0 {
		c.MaxAge = anonMaxAgeDefault
	}
	return c
}

// NewAnonStore builds a cookie-only AnonStore from cfg.
func NewAnonStore(cfg AnonConfig) *AnonStore {
	return &AnonStore{cfg: cfg.withDefaults()}
}

// Config returns the resolved (defaulted) config.
func (s *AnonStore) Config() AnonConfig { return s.cfg }

// anonCtxKey is the unexported context key for the anonymous id.
type anonCtxKey struct{}

// withAnonID returns a copy of ctx carrying the anonymous session id.
func withAnonID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, anonCtxKey{}, id)
}

// AnonFrom returns the anonymous session id attached to ctx and whether one is
// present. It is set by AnonStore.Ensure; an unwrapped request carries none.
func AnonFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(anonCtxKey{}).(string)
	return id, ok
}

// cookie builds the Set-Cookie for an anonymous id. HttpOnly (page script can't
// read it), Lax SameSite, and the configured path/secure/max-age.
func (s *AnonStore) cookie(id string) *http.Cookie {
	return &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    id,
		Path:     s.cfg.CookiePath,
		HttpOnly: true,
		Secure:   s.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.MaxAge,
	}
}

// Ensure attaches the anonymous middleware: it reads the id from the cookie (or
// mints a fresh one), refreshes the cookie on the response so an active visitor
// keeps a stable id, and puts the id in the request context for AnonFrom. Unlike
// the DB store's OptionalAuth — anonymous-is-the-absence-of-a-user — this stage
// always produces an identity, because that is the whole point of the cookie-only
// store.
func (s *AnonStore) Ensure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if ck, err := r.Cookie(s.cfg.CookieName); err == nil && ck.Value != "" {
			id = ck.Value
		}
		if id == "" {
			minted, err := mintToken()
			if err != nil {
				// crypto/rand failing is unrecoverable; fail the request rather than
				// serving an identity-less anonymous session.
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			id = minted
		}
		// Refresh the cookie every request so the idle window slides for an active
		// visitor. Set it before the handler writes (SSE handlers flush headers
		// early), which is why this runs as outer middleware.
		http.SetCookie(w, s.cookie(id))
		next.ServeHTTP(w, r.WithContext(withAnonID(r.Context(), id)))
	})
}
