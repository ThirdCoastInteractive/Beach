// Command barrace is a standalone, database-less showcase of server-driven SVG
// animation: a live bar race where the server re-layouts ~6 labeled bars every
// second, reorders them by value, and patches the whole SVG over SSE. There is
// no client animation code — the bars carry stable per-label element ids, so
// Datastar morphs each rect/text in place and CSS transitions on the SVG
// geometry properties (x / y / width) tween the slide-and-reorder motion.
//
// Run it:
//
//	go build -o bin/barrace ./cmd/barrace
//	PORT=8080 ./bin/barrace        # or just ./bin/barrace to bind :8080
//
// Then open http://localhost:8080/. The race starts the instant the page is
// interactive (data-on-load opens the stream) and advances once per second.
//
// Determinism: every frame's bar values are derived purely from an integer tick
// counter via a fixed seeded formula (tickValues). No wall-clock randomness is
// involved, so two processes at the same tick render byte-identical frames and
// the animation is reproducible.
package main

import (
	"bytes"
	"context"
	"embed"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"math"
	"os"
	"sync/atomic"
	"time"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/chart"
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/hub"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
	"github.com/a-h/templ"
)

// staticFS holds the assets the page needs: the generated Stellar stylesheet
// (served at /static/css/stellar.css per framework convention) and the Datastar
// client the driftwood shell loads from /static/js/datastar.js.
//
//go:embed static
var staticFS embed.FS

const (
	// raceTopic is the hub topic the ticker publishes frames to and the page's
	// stream subscribes to.
	raceTopic = "barrace"

	// raceTarget is the DOM id of the SVG wrapper that every frame morphs in
	// place (outer morph by its own id).
	raceTarget = "race"
)

// labels are the fixed contestants in the race. Their order here is stable; the
// chart sorts by value each frame, so a label's rank (and therefore its screen
// row) changes over time while its element id stays put.
var labels = []string{
	"Driftwood",
	"Stellar",
	"Datastar",
	"Hub",
	"ECS",
	"Chart",
}

// tick is the monotonically increasing frame counter. The action-free demo only
// ever advances it from the ticker, but it is atomic so a catch-up replay can
// read a consistent value concurrently.
var tick atomic.Int64

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("barrace: static sub-fs: %v", err)
	}

	h := hub.New()

	app := beach.New(beach.Config{
		Service: "barrace",
		Static:  sub,
		Kit:     beach.KitStatic("driftwood"),
		Hub:     h,
		Logger:  logger,
	})

	app.Page("/", indexPage)
	app.Stream("/race", raceStream)

	// The ticker is the animation engine: each second it advances the frame
	// counter, re-layouts the bars from the deterministic per-tick values, and
	// publishes the freshly rendered SVG to every subscriber. The client does no
	// animation work — it just morphs the new SVG in, and CSS transitions tween.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Ticker(ctx, raceTopic, time.Second, func() (hub.Event, bool) {
		n := tick.Add(1)
		return hub.Event{Bytes: renderToBytes(raceFragment(n))}, true
	})

	logger.Info("barrace: starting", "addr", addr,
		"routes", "GET / , GET /race , GET /static/...")
	if err := app.Start(addr); err != nil {
		log.Fatalf("barrace: %v", err)
	}
}

// indexPage renders the showcase page on navigation and patches just the race
// SVG on a Datastar request. data-on-load opens the SSE stream the moment the
// page is interactive, so the race begins without any user action.
func indexPage(c *beach.Ctx) (beach.View, error) {
	kit := c.Kit()

	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    "Bar race",
		Subtitle: "Server-driven SVG animation. No client animation code.",
	})

	card := kit.Layout().Card(ui.CardProps{
		Surface: ui.SurfacePanel,
		Heading: "Live standings",
	},
		raceFragment(tick.Load()),
	)

	body := kit.Layout().Container(ui.ContainerProps{
		Attrs: datastar.Attrs{datastar.Init("@get('/race')")},
	},
		heading,
		card,
	)

	page := driftwood.Page(driftwood.PageProps{
		Title:       "the-beach — bar race",
		Description: "A database-less, server-animated bar race over SSE.",
	}, body)

	return beach.View{
		Page:     page,
		Fragment: raceFragment(tick.Load()),
		Target:   raceTarget,
	}, nil
}

// raceStream declares the SSE subscription for the race region: subscribe to the
// race topic and replay one immediate frame as catch-up so the SVG is correct
// the instant the stream opens (before the first published tick lands).
func raceStream(c *beach.Ctx) (beach.Sub, error) {
	return beach.Sub{
		Topics: []string{raceTopic},
		CatchUp: func(_ string, p beach.Patcher) error {
			return p.Patch(beach.Patch{Fragment: raceFragment(tick.Load()), Target: raceTarget})
		},
	}, nil
}

// raceFragment renders the race region for frame n: a fixed id="race" wrapper
// holding a <style> (the CSS transitions that tween the motion) and the bar-race
// SVG. The wrapper id is stable so every frame morphs it in place; the bars
// inside carry stable per-label ids so each rect/text morphs individually and
// the transitions animate the slide and reorder.
func raceFragment(n int64) templ.Component {
	lay := chart.LayoutBarRace(chart.BarRaceInput{
		Title: "the-beach modules — live load",
		Bars:  frameData(n),
		Top:   len(labels),
		Width: 520,
		RowH:  44,
	})

	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div id="`+raceTarget+`" class="br-wrap">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, raceStyle); err != nil {
			return err
		}
		if err := chart.BarRaceSVG(w, lay); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}

// raceStyle is the entire "animation engine" on the client: scoped CSS that
// transitions the SVG geometry properties the server changes each frame. Modern
// browsers animate the presentation attributes x/y/width/height as CSS
// properties, so morphing a rect to a new geometry tweens instead of snapping.
// Durations and easings come from Stellar tokens only.
const raceStyle = `<style>
.br-wrap .chart rect,
.br-wrap .chart text {
  transition:
    x var(--anim-duration-slow) var(--anim-ease-emphasized),
    y var(--anim-duration-slow) var(--anim-ease-emphasized),
    width var(--anim-duration-slow) var(--anim-ease-standard),
    fill var(--anim-duration-base) var(--anim-ease-standard);
}
.br-wrap .chart { width: 100%; height: auto; }
@media (prefers-reduced-motion: reduce) {
  .br-wrap .chart rect,
  .br-wrap .chart text { transition: none; }
}
</style>`

// frameData builds the labeled values for frame n. Every value is a pure
// function of n and the label index, so frames are deterministic and the race
// is reproducible from the tick counter alone.
func frameData(n int64) []chart.Datum {
	out := make([]chart.Datum, len(labels))
	for i, label := range labels {
		out[i] = chart.Datum{Label: label, Value: tickValue(n, i)}
	}
	return out
}

// tickValue is the deterministic per-frame value for the i-th label at frame n.
// It is a smooth, bounded, label-phased wave plus a slow drift, so bars rise and
// fall at different rates and overtake one another — producing reorders — without
// any randomness. The result is always positive.
func tickValue(n int64, i int) float64 {
	t := float64(n) / 6.0           // slow time base
	phase := float64(i) * 1.4       // per-label phase offset so they desync
	freq := 1.0 + 0.25*float64(i%3) // a few distinct oscillation rates
	base := 50.0 + 18.0*float64((i*7+3)%5)
	wave := 40.0 * math.Sin(t*freq+phase)
	drift := 22.0 * math.Sin(t*0.35+phase*0.5)
	v := base + wave + drift
	if v < 4 {
		v = 4 // keep every bar visible and positive
	}
	return math.Round(v)
}

// renderToBytes renders a templ component to bytes for a hub.Event. The hub fans
// these pre-rendered bytes to every subscriber; the framework's stream loop
// PatchElements them, morphing the #race wrapper by its id.
func renderToBytes(c templ.Component) []byte {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		return nil
	}
	return buf.Bytes()
}
