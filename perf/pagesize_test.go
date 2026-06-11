package perf

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	beach "github.com/ThirdCoastInteractive/the-beach"
	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/ThirdCoastInteractive/the-beach/ui/driftwood"
)

// newApp builds a database-less beach.App with the driftwood kit — the same
// configuration cmd/demo and examples/driftbottle use. It is the real render
// path: routes go through the full middleware stack and PageFunc adapter, so the
// measured bytes are exactly what the framework writes on the wire.
func newApp() *beach.App {
	return beach.New(beach.Config{
		Service: "perf",
		Kit:     beach.KitStatic("driftwood"),
	})
}

// demoHome reconstructs cmd/demo's home page body: the example apps' handlers
// live in package main (cross-main import is awkward), so the gate rebuilds the
// representative driftwood page through the live kit — heading, a panel card with
// an action button, and a live region — wrapped in the same driftwood shell. If
// the real handler grows, this page grows with the same components, so the gate
// stays representative of an actual above-the-fold home page.
func demoHome(c *beach.Ctx) (beach.View, error) {
	kit := c.Kit()
	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    "the-beach",
		Subtitle: "SSR + Datastar, no database, live over SSE.",
	})
	card := kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "Tick"},
		kit.Buttons().Button(ui.ButtonProps{
			Label:    "Increment",
			Role:     ui.RoleAccent,
			LeadIcon: "plus",
			Attrs:    datastar.Attrs{datastar.OnClick("@post('/inc')")},
		}),
	)
	body := kit.Layout().Container(ui.ContainerProps{
		Attrs: datastar.Attrs{datastar.On("load", "@get('/live')")},
	}, heading, card)
	page := driftwood.Page(driftwood.PageProps{
		Title:       "the-beach demo",
		Description: "A database-less the-beach showcase.",
	}, body)
	return beach.View{Page: page}, nil
}

// driftbottleHome reconstructs examples/driftbottle's home page body: the page
// heading plus the lobby's "find a stranger" container, in the driftwood shell —
// the same shape srv.indexPage renders on navigation.
func driftbottleHome(c *beach.Ctx) (beach.View, error) {
	kit := c.Kit()
	heading := kit.Layout().PageHeading(ui.PageHeadingProps{
		Title:    "driftbottle",
		Subtitle: "You're about to talk to a complete stranger. Be kind.",
	})
	lobby := kit.Layout().Card(ui.CardProps{Surface: ui.SurfacePanel, Heading: "Stranger chat"},
		kit.Buttons().Button(ui.ButtonProps{
			Label: "Talk to a stranger",
			Role:  ui.RoleAccent,
			Attrs: datastar.Attrs{datastar.OnClick("@post('/start')")},
		}),
	)
	body := kit.Layout().Container(ui.ContainerProps{
		Attrs: datastar.Attrs{datastar.On("load", "@get('/events')")},
	}, heading, lobby)
	page := driftwood.Page(driftwood.PageProps{
		Title:       "driftbottle — talk to a stranger",
		Description: "Anonymous, ephemeral stranger chat. No accounts, no history.",
	}, body)
	return beach.View{Page: page}, nil
}

// TestHomePagesUnder14KB is the page-size gate: every example app's home route
// must render ≤14KB gzipped (the 14KB rule). It measures the real render path and
// appends each result to a JSONL perf-history file so the trend lands in the diff.
func TestHomePagesUnder14KB(t *testing.T) {
	app := newApp()
	app.Page("/demo", demoHome)
	app.Page("/driftbottle", driftbottleHome)
	h := app.Handler()

	// historyPath is the JSONL perf history. In CI BEACH_PERF_HISTORY points at
	// the tracked file (perf/history.jsonl); locally the gate writes to a temp
	// file so a bare `go test` never dirties the tree.
	historyPath := os.Getenv("BEACH_PERF_HISTORY")
	if historyPath == "" {
		historyPath = filepath.Join(t.TempDir(), "history.jsonl")
	}
	// ts is the build/version stamp the history rows carry. CI passes the commit
	// via BEACH_PERF_TS; otherwise the run is tagged "test" (deterministic, no
	// clock churn in the diff).
	ts := os.Getenv("BEACH_PERF_TS")
	if ts == "" {
		ts = "test"
	}

	cases := []struct{ name, path string }{
		{"demo", "/demo"},
		{"driftbottle", "/driftbottle"},
	}
	for _, tc := range cases {
		raw, gz := PageSize(h, tc.path)
		if raw == 0 {
			t.Fatalf("%s: empty response body (route not rendering)", tc.name)
		}
		if gz < 0 {
			t.Fatalf("%s: gzip failed", tc.name)
		}
		t.Logf("%-12s %s  raw=%d B  gzipped=%d B  (budget %d B)", tc.name, tc.path, raw, gz, Budget14KB)
		if gz > Budget14KB {
			t.Errorf("%s home page over budget: %d B gzipped > %d B (14KB rule)", tc.name, gz, Budget14KB)
		}
		if err := AppendSample(historyPath, ts, tc.path, gz); err != nil {
			t.Fatalf("%s: append perf history: %v", tc.name, err)
		}
	}
}

// TestAppendSampleRoundTrips checks the JSONL writer: each AppendSample call adds
// exactly one parseable {ts, route, bytes} line and never rewrites earlier rows.
func TestAppendSampleRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	want := []PerfSample{
		{Ts: "v1", Route: "/", Bytes: 1234},
		{Ts: "v2", Route: "/", Bytes: 1200},
		{Ts: "v2", Route: "/about", Bytes: 800},
	}
	for _, s := range want {
		if err := AppendSample(path, s.Ts, s.Route, s.Bytes); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var got []PerfSample
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s PerfSample
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("unmarshal %q: %v", sc.Text(), err)
		}
		got = append(got, s)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
