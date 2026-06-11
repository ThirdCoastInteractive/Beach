package ui

import (
	"context"
	"html"
	"io"
	"sort"
	"sync"

	"github.com/a-h/templ"
)

// The icon API is ui.I(name, opts...). It renders an icon-font glyph and, as a
// side effect of being called, records the glyph name so a build-time pass can
// subset the shipped font to exactly the glyphs the code references — the font
// contains only what ui.I actually names.
//
// Rendering is markup-and-CSS only: an <i> with a class the kit's CSS maps to a
// font glyph, plus tokenized options (spin animation, size). No script.

// IconOpt configures a single icon. Options are applied to an iconConfig.
type IconOpt func(*iconConfig)

type iconConfig struct {
	spin bool
	size Size
	// label sets an accessible name; empty renders the icon aria-hidden.
	label string
}

// Spin animates the icon with Stellar's --anim-duration-*/--anim-ease-* tokens
// (e.g. a loading gear). It is a presentational state, so it stays pure CSS.
func Spin(c *iconConfig) { c.spin = true }

// IconSize sets the icon's size step.
func IconSize(s Size) IconOpt { return func(c *iconConfig) { c.size = s } }

// IconLabel gives the icon an accessible name. Without it the icon renders
// aria-hidden (decorative), which is correct when adjacent text already labels
// it (e.g. inside a Button).
func IconLabel(label string) IconOpt { return func(c *iconConfig) { c.label = label } }

var (
	iconMu   sync.Mutex
	iconUsed = map[string]struct{}{}
)

// recordIcon notes that name is referenced so the subsetter keeps its glyph.
func recordIcon(name string) {
	iconMu.Lock()
	iconUsed[name] = struct{}{}
	iconMu.Unlock()
}

// UsedIcons returns the sorted set of icon names referenced by ui.I so far. The
// build-time font-subsetting pass reads this to ship only the needed glyphs.
func UsedIcons() []string {
	iconMu.Lock()
	out := make([]string, 0, len(iconUsed))
	for n := range iconUsed {
		out = append(out, n)
	}
	iconMu.Unlock()
	sort.Strings(out)
	return out
}

// I renders an icon glyph by name with optional modifiers, e.g.
//
//	ui.I("gear", ui.Spin)
//
// It records name for build-time font subsetting and returns a templ.Component.
// Color, size, and the spin animation come from Stellar tokens via the icon
// class; nothing is hardcoded.
func I(name string, opts ...IconOpt) templ.Component {
	recordIcon(name)
	cfg := iconConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		class := "icon icon-" + html.EscapeString(name)
		if cfg.spin {
			class += " icon-spin"
		}
		if cfg.size != "" {
			class += " icon-" + string(cfg.size)
		}
		s := `<i class="` + class + `"`
		if cfg.label != "" {
			s += ` role="img" aria-label="` + html.EscapeString(cfg.label) + `"`
		} else {
			s += ` aria-hidden="true"`
		}
		s += `></i>`
		_, err := io.WriteString(w, s)
		return err
	})
}
