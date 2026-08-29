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
