// Package bad contains deliberate violations, one per rule, for the analyzer
// tests. It is under testdata so the Go toolchain ignores it; the linter is
// pointed at this directory explicitly.
package bad

import (
	"net/http"

	"github.com/google/uuid"         // uuid-import
	"github.com/jackc/pgx/v5/pgtype" // pgtype-import
)

// keep the imports used so this stays parseable Go.
var _ = uuid.Nil
var _ pgtype.Text

// rawDatastar hand-writes a Datastar attribute instead of using the builders.
const rawDatastar = `<button data-on:click="@post('/x')">go</button>` // raw-datastar

// dashSpelling uses the dash form, also caught.
const dashSpelling = `<div data-bind-email></div>` // raw-datastar

// hardcoded color baked straight into markup.
const swatch = `<span style="color:#ff8800">hi</span>` // hardcoded-color

// oklch baked in.
const swatchOklch = `<span style="color:oklch(0.7 0.1 200)">hi</span>` // hardcoded-color

// a custom inline script — not the datastar bundle.
const reload = `<script>window.location.reload()</script>` // custom-script

// an image with no text alternative at all.
const avatar = `<img src="/me.jpg" class="avatar">` // a11y-img-alt

// a graphic that announces itself and then says nothing.
const chart = `<svg role="img" class="chart-svg"><path d="M0 0"/></svg>` // a11y-unnamed-role-img

// register mounts a handler the naked way.
func register(mux *http.ServeMux) {
	mux.HandleFunc("/raw", func(w http.ResponseWriter, r *http.Request) {}) // naked-handlerfunc
}

// a Tailwind spacing utility written straight into the markup.
const padded = `<div class="dw-card p-6">body</div>` // raw-spacing

// inline padding in a style attribute, the other spelling.
const inlinePad = `<section style="padding:2.5rem 1rem">body</section>` // raw-spacing

// The kit's own semantic classes must NOT fire: they carry a rung name, not a
// number, which is the whole distinction the rule is drawing.
const okSpacing = `<div class="dw-stack dw-gap-lg dw-pad-md">body</div>`

// Nor may a per-instance dimension the kit cannot know — a reserved height is
// what inline style is legitimately for.
const okReserved = `<div class="dash-widget-body" style="height:240px"></div>`
