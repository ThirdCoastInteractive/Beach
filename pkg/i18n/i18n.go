// Package i18n is Beach's flat-key translation runtime.
//
// A catalog maps each key to a reference label and translator comment
// (catalog.json); each locale file (locales/<tag>.json) maps the same keys to
// translations for one language tag. Both are embedded at build time and loaded
// once at boot.
//
// Lookup is by literal key only:
//
//	i18n.T(ctx, "pantry.items.title")
//	i18n.T(ctx, "cart.count", n) // "%d items in cart"
//
// The active locale comes from the request context, set by Middleware. A key
// missing from the active locale falls back to the default locale; in
// development that gap is logged. Apps that never configure locales pay nothing:
// T resolves against the default locale and the feature is otherwise inert.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
)

//go:embed catalog.json locales/*.json
var builtin embed.FS

// ctxKey is the unexported context key under which the active locale tag is
// stored. Middleware sets it; localeFrom reads it.
type ctxKey struct{}

// catKey is the unexported context key under which the active Catalog is
// stored. Middleware sets it; catalogFrom reads it, letting the package-level T
// resolve app strings without an app threading its own *Catalog.
type catKey struct{}

// Catalog is a loaded set of locales with a default fallback. The zero value is
// not usable; build one with Load or rely on the package-default initialised in
// init from the embedded files.
type Catalog struct {
	// def is the default locale tag, e.g. "en-US". Lookups that miss the active
	// locale fall back here.
	def string
	// messages[tag][key] = translation. tags are stored verbatim from the file
	// name and also indexed case-folded for resolution.
	messages map[string]map[string]string
	// fold maps a lower-cased tag to its canonical stored tag.
	fold map[string]string
	// dev enables logging of fallback/missing-key gaps.
	dev bool
	log *slog.Logger
}

// builtinCat is the framework's embedded catalog, built from the embedded
// files. It is the immutable ultimate fallback for the package-level T.
var builtinCat = mustBuiltin()

// def is the package-level default catalog the package-level T resolves
// against. It starts as the framework's embedded catalog so T is usable with no
// configuration; an app may replace it once at boot via SetDefault.
var def = builtinCat

// SetDefault registers c as the catalog the package-level T resolves against
// when no catalog is carried on the request context. It lets an app expose its
// own strings through i18n.T after a single call at boot, instead of threading
// a *Catalog through every handler. A nil c restores the framework's embedded
// catalog. It is not safe for concurrent use with T; call it during startup
// before serving requests.
func SetDefault(c *Catalog) {
	if c == nil {
		def = builtinCat
		return
	}
	def = c
}

func mustBuiltin() *Catalog {
	c, err := Load(builtin, "en-US")
	if err != nil {
		// The embedded files are part of the package; a failure here is a build
		// bug, not a runtime condition.
		panic("i18n: loading embedded catalog: " + err.Error())
	}
	return c
}

// Load reads locales/<tag>.json files from fsys and returns a Catalog whose
// default locale is def. A catalog.json at the root, if present, is read only to
// validate that it parses; the runtime needs only the locale files. Missing
// locales directory yields an empty (but usable) catalog.
func Load(fsys fs.FS, defLocale string) (*Catalog, error) {
	c := &Catalog{
		def:      defLocale,
		messages: map[string]map[string]string{},
		fold:     map[string]string{},
		log:      slog.Default(),
	}

	// catalog.json is optional; parse it only to surface malformed JSON early.
	if b, err := fs.ReadFile(fsys, "catalog.json"); err == nil {
		var tmp map[string]json.RawMessage
		if err := json.Unmarshal(b, &tmp); err != nil {
			return nil, fmt.Errorf("i18n: catalog.json: %w", err)
		}
	}

	entries, err := fs.ReadDir(fsys, "locales")
	if err != nil {
		// No locales directory at all is fine — the catalog is simply empty and
		// T returns keys verbatim.
		return c, nil
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		tag := strings.TrimSuffix(name, ".json")
		b, err := fs.ReadFile(fsys, "locales/"+name)
		if err != nil {
			return nil, fmt.Errorf("i18n: read %s: %w", name, err)
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", name, err)
		}
		c.messages[tag] = m
		c.fold[strings.ToLower(tag)] = tag
	}
	return c, nil
}

// Dev marks the catalog as running in development, enabling gap logging. It
// returns the receiver for chaining.
func (c *Catalog) Dev(on bool) *Catalog {
	c.dev = on
	return c
}

// Locales returns the sorted list of loaded locale tags.
func (c *Catalog) Locales() []string {
	tags := make([]string, 0, len(c.messages))
	for t := range c.messages {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// Lookup resolves key in the named locale, falling back to the default locale,
// then to the framework's embedded catalog, then to the key itself. ok reports
// whether a translation was found in any of them.
//
// The embedded fallback is what makes an app's catalog *extend* the framework's
// rather than replace it. Without it, an app that configures Locales at all —
// which is every app past the scaffold's first hour — renders every framework
// string as its own raw key: the skip link reads "ui.a11y.skip_to_content", the
// close button on every dialog is nameless, and the kit's whole naming law
// quietly stops holding. An app that wants different wording still gets it, by
// defining the key itself: its own catalog is consulted first.
func (c *Catalog) Lookup(locale, key string) (msg string, ok bool) {
	if c == nil {
		return key, false
	}
	if tag, found := c.resolveTag(locale); found {
		if m, ok := c.messages[tag][key]; ok {
			return m, true
		}
	}
	// Fall back to the default locale.
	if c.def != "" {
		if tag, found := c.resolveTag(c.def); found {
			if m, ok := c.messages[tag][key]; ok {
				if c.dev && !sameTag(locale, c.def) {
					c.log.Warn("i18n: locale fallback", "key", key, "locale", locale, "fallback", c.def)
				}
				return m, true
			}
		}
	}
	// The framework's own strings, for a key the app's catalog does not define.
	// Guarded against recursion, since builtinCat is itself a *Catalog.
	if c != builtinCat {
		if m, found := builtinCat.Lookup(locale, key); found {
			return m, true
		}
	}
	if c.dev {
		c.log.Warn("i18n: missing key", "key", key, "locale", locale)
	}
	return key, false
}

// resolveTag maps a requested tag to a stored one, case-insensitively, also
// honoring a primary-language fallback (e.g. "en" matches "en-US" when no exact
// tag exists).
func (c *Catalog) resolveTag(tag string) (string, bool) {
	if tag == "" {
		return "", false
	}
	lower := strings.ToLower(tag)
	if t, ok := c.fold[lower]; ok {
		return t, true
	}
	// Primary-language fallback: "en" -> first "en-*".
	prefix := lower + "-"
	for l, t := range c.fold {
		if strings.HasPrefix(l, prefix) {
			return t, true
		}
	}
	return "", false
}

func sameTag(a, b string) bool { return strings.EqualFold(a, b) }

// T translates key against the active locale carried by ctx, substituting args
// fmt-style when the message contains verbs. The catalog is resolved in order:
// one carried on ctx (set by Middleware), then the package default (see
// SetDefault), then the framework's embedded catalog. A key absent everywhere
// is returned verbatim so missing translations are visible, never fatal.
func T(ctx context.Context, key string, args ...any) string {
	c := catalogFrom(ctx)
	if c == nil {
		c = def
	}
	return c.T(ctx, key, args...)
}

// T is the Catalog-scoped form of the package-level T.
func (c *Catalog) T(ctx context.Context, key string, args ...any) string {
	locale := localeFrom(ctx)
	if locale == "" {
		locale = c.def
	}
	msg, _ := c.Lookup(locale, key)
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// WithLocale returns a copy of ctx carrying tag as the active locale.
func WithLocale(ctx context.Context, tag string) context.Context {
	return context.WithValue(ctx, ctxKey{}, tag)
}

// localeFrom reads the active locale tag from ctx; "" when unset.
func localeFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// WithCatalog returns a copy of ctx carrying c as the catalog the package-level
// T resolves against for this request, taking precedence over the package
// default. Middleware sets it so app strings resolve through i18n.T without an
// app threading its own *Catalog.
func WithCatalog(ctx context.Context, c *Catalog) context.Context {
	return context.WithValue(ctx, catKey{}, c)
}

// catalogFrom reads the catalog carried by ctx; nil when unset.
func catalogFrom(ctx context.Context) *Catalog {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(catKey{}).(*Catalog); ok {
		return c
	}
	return nil
}

// Locale returns the active locale tag carried by ctx, or "" when unset. It is
// the exported reader paired with WithLocale.
func Locale(ctx context.Context) string { return localeFrom(ctx) }
