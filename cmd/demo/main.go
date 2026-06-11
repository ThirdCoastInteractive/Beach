// Command demo is a runnable, database-less showcase of the-beach: one page
// rendered through the driftwood ui.Kit, one Datastar action that mutates server
// state and patches a fragment, and one SSE stream driven by a hub.Ticker that
// updates a live clock/counter every second.
//
// Run it:
//
//	go build -o bin/demo ./cmd/demo
//	PORT=8080 ./bin/demo        # or just ./bin/demo to bind :8080
//
// Then open http://localhost:8080/. The "Tick" card shows a server clock that
// advances every second over SSE; the button increments a server-held counter
// and patches the same fragment.
package main

import (
	"bytes"
	"context"
	"embed"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/hub"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
	"github.com/a-h/templ"
)

// staticFS holds the two assets the page needs: the generated Stellar stylesheet
// (served at /static/css/stellar.css per the framework convention) and the
// Datastar client the driftwood shell loads from /static/js/datastar.js. The kit
// also links /static/css/driftwood.css; it is optional cosmetic polish and 404s
// harmlessly when absent.
//
//go:embed static
var staticFS embed.FS

// liveTopic is the single hub topic the clock ticker publishes to and the page's
// stream subscribes to.
const liveTopic = "tick"

// liveTarget is the DOM id of the fragment region the action and the ticker both
// patch (by outer morph on the element's own id).
const liveTarget = "live"

// counter is the shared server state the action increments. The ticker reads it
// so the live clock fragment always reflects the latest count.
var counter atomic.Int64

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	// The demo's own static tree (stellar.css + datastar.js), rooted so that
	// "css/stellar.css" resolves directly under /static.
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("demo: static sub-fs: %v", err)
	}

	// The hub backs the SSE stream; a Ticker fans a freshly rendered clock
	// fragment to every subscriber once a second.
	h := hub.New()

	app := beach.New(beach.Config{
		Service: "demo",
		Static:  sub,
		Kit:     beach.KitStatic("driftwood"),
		Hub:     h,
		Logger:  logger,
	})

	app.Page("/", indexPage)
	app.Page("/specimen", specimenPage)
	app.Action("/inc", incAction)
	app.Stream("/live", liveStream)

	// Start the clock ticker in the background; it publishes until the process
	// context is cancelled (i.e. for the life of the server).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Ticker(ctx, liveTopic, time.Second, func() (hub.Event, bool) {
		return hub.Event{Bytes: renderToBytes(liveFragment(counter.Load()))}, true
	})

	logger.Info("demo: starting", "addr", addr,
		"routes", "GET / , POST /inc , GET /live , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("demo: %v", err)
	}
}

// indexPage renders the showcase page on navigation and patches just the live
// fragment on a Datastar request (the framework owns that branch). The full
// document is the driftwood shell; the body is a heading, a styled card, the
// action button, and the live fragment region.
func indexPage(c *beach.Ctx) (beach.View, error) {
	kit := c.Kit()

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    "the-beach",
		Subtitle: "SSR + Datastar, no database, live over SSE.",
	})

	// The live region: it carries id="live" so both the action patch and the
	// per-second ticker morph it in place by its own id.
	card := kit.Layout().Card(ui.CardProps{
		Surface: ui.SurfacePanel,
		Heading: "Tick",
	},
		liveFragment(counter.Load()),
		kit.Buttons().Button(ui.ButtonProps{
			Label:    "Increment",
			Role:     ui.RoleAccent,
			LeadIcon: "plus",
			// @post drives the ActionFunc; Datastar applies the returned patches.
			Attrs: datastar.Attrs{datastar.OnClick("@post('/inc')")},
		}),
	)

	// data-on-load opens the SSE stream as soon as the page is interactive, so the
	// clock starts advancing without any user action.
	body := kit.Layout().Container(ui.ContainerProps{
		Attrs: datastar.Attrs{datastar.Init("@get('/live')")},
	},
		heading,
		card,
	)

	page := driftwood.Page(driftwood.PageProps{
		Title:       "the-beach demo",
		Description: "A database-less the-beach showcase.",
	}, body)

	return beach.View{
		Page:     page,
		Fragment: liveFragment(counter.Load()),
		Target:   liveTarget,
	}, nil
}

// specimenPage renders the toolkit specimen through the active driftwood kit:
// ui.SpecimenPage builds the page body (token sheet, icon grid, every Core
// component, and the 18-chart gallery), and the driftwood shell wraps it with the
// /static/css/stellar.css link. It is intentionally over the 14KB budget — the
// specimen exists to show everything at once, not to ship to users.
func specimenPage(c *beach.Ctx) (beach.View, error) {
	kit := c.Kit()
	body := ui.SpecimenPage(kit)
	page := driftwood.Page(driftwood.PageProps{
		Title:       "the-beach — specimen",
		Description: "Living specimen of the driftwood kit: tokens, icons, components, charts.",
	}, body)
	return beach.View{Page: page}, nil
}

// incAction increments the shared counter and patches the live fragment with the
// new value. It is Datastar-only; a plain navigation to /inc is a 404.
func incAction(c *beach.Ctx) (beach.Patches, error) {
	n := counter.Add(1)
	return beach.Patches{
		{Fragment: liveFragment(n), Target: liveTarget},
	}, nil
}

// liveStream declares the SSE subscription for the live region: subscribe to the
// clock topic and replay one immediate frame as catch-up so the fragment is
// correct the instant the stream opens (before the first tick lands).
func liveStream(c *beach.Ctx) (beach.Sub, error) {
	return beach.Sub{
		Topics: []string{liveTopic},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: liveFragment(counter.Load()), Target: liveTarget})
		},
	}, nil
}

// liveFragment renders the live region: a fixed id="live" wrapper showing the
// server clock and the current counter. Both the action and the ticker patch
// this element by its own id (outer morph), so they stay in sync.
func liveFragment(count int64) templ.Component {
	now := time.Now().Format("15:04:05")
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		html := `<div id="` + liveTarget + `" class="dw-live">` +
			`<p class="dw-clock">Server time: <strong>` + templ.EscapeString(now) + `</strong></p>` +
			`<p class="dw-count">Increments: <strong>` + templ.EscapeString(itoa(count)) + `</strong></p>` +
			`</div>`
		_, err := io.WriteString(w, html)
		return err
	})
}

// renderToBytes renders a templ component to bytes for a hub.Event. The hub fans
// these pre-rendered bytes to every subscriber; the framework's stream loop
// PatchElements them, morphing the #live element by its id.
func renderToBytes(c templ.Component) []byte {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		// A render error on a static fragment is a programming error; emit nothing
		// rather than a partial element.
		return nil
	}
	return buf.Bytes()
}

// itoa formats an int64 without pulling in fmt for one call.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }
