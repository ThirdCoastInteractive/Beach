# Architecture 14 — `chart` + `ch`: SSR charts and ClickHouse

[← docs index](../README.md) · prev: [API codegen](13-apigen.md) · next: [Example apps](15-examples.md)

## `chart` — server-rendered SVG charts

A re-port of Atlas's chart system as **one package**: pure-Go geometry (`*.go`) and
the templ render components (`svg.templ`, `fragments.templ`) live together in
`chart/` — every chart is final HTML, drawn before it reaches the browser.
**22 chart types**: line/area, sparkline, bar (horizontal/stacked/grouped/bullet),
gauge (single/stacked), billboard KPI, scatter, box plot, calendar heatmap, heatmap,
streamgraph, sankey, chord, bollinger, horizon, ridgeline, difference, bundle,
timeline — plus the SSE **bar race** (`LayoutBarRace` + `BarRaceSVG`), which is not
a static type but the server-animation showcase.

The shape of the API — a `Layout*` function computes geometry, a templ component
renders it, and a `Chart*Fragment` wrapper adds the stable-id `<figure>` a Datastar
patch or `ui.Defer` placeholder morphs into:

```go
layout := chart.LayoutHBar(chart.HBarSeries{Series: rows, Max: max, Unit: " kW"})
```

```templ
@chart.BarChartSVG(layout)                       // bare SVG
@chart.ChartBarFragment("rack-units", layout)    // id-carrying figure, patch/Defer target
```

The library obeys the framework's laws by construction:

- **Token-themed**: series colors cycle `--color-series-a…o` (15, generated from a
  [rybitten gamut](19-rybitten.md)); emphasis names meaning through the role tokens
  (`--color-bad` for bad, etc.), never hue alone; no hardcoded colors in chart code
  ([ui token contract](06-ui.md)). Eight alternate `[data-chart-theme]` palettes plus
  munsell redefine the series tokens under any element, so one widget — or one page —
  retints without touching chart code. SVG text is themed in the same sheet
  (`.chart-axis`, `.chart-label`, `.chart-gauge-value`, `.chart-kpi-*`) so labels
  read on the dark surface — the SVGs carry no color of their own.
- **Accessible**: `<title>` elements on interactive regions, `role="img"` on
  roots, color never the only encoding (trend arrows accompany trend colors).
- **Responsive without scripts**: percentage coordinates for axes and text
  (rem-stable type) plus a nested `viewBox` with `preserveAspectRatio="none"`
  stretching the plot geometry — a chart fills any container while stroke widths
  stay constant.
- **Defer-native**: dashboard widgets are [`ui.Defer`](06-ui.md#deferred-sections)
  sections — kit-styled skeleton, exact reserved dimensions, `@get` on viewport
  intersection, fragment morphs in by the `Chart*Fragment` id.
- **Server-built tooltips**: layouts attach per-element tooltip HTML
  (`chart.BuildTipHTML`) as `data-tip` attributes, so hover content is rendered
  server-side like everything else; the client layer (below) only positions it.
- **Live and animated over SSE**: a chart fragment is just templ, so it works as a
  [`sim` projection](08-sim.md#projections-change-detection--patches) or hub-topic
  patch. SVG elements are DOM elements, so Datastar patches *inside* the SVG, and CSS
  transitions tween geometry between states — animation is the server pushing new
  numbers, not a script playing a timeline. The **bar race** is the showcase, live in
  [boardwalk's "Cash race" card](15-examples.md#examplesboardwalk--real-estate-game-monopoly-like):
  a hub ticker re-lays-out the ranked bars each second and publishes the rendered
  fragment; the bars slide and reorder over SSE.

### The client interaction layer

Charts ship with a built-in interaction layer — core, not opt-in. The driftwood
page head loads `/static/js/chart.js` (alongside `app.css` and `datastar.js`), a
plain ES module that imports self-contained side-effect modules — no bundler, no
exports, each wiring its own listeners and MutationObservers against the
server-rendered SVG:

| Module | Does |
| --- | --- |
| `chart-tip` | floating tooltips, positioned from the server-built `data-tip` HTML |
| `chart-line-hover` | line-chart crosshair, value handles, delta tooltip, legend series toggle, keyboard stepping |
| `chart-toggle` | click-to-pin a hover state |
| `chart-sankey-hover` / `chart-chord-hover` / `chart-bundle-hover` | flow-diagram link/node highlight |
| `chart-vb-hover` | crosshair on the nested-viewBox charts (bollinger, difference, scatter) |
| `chart-toolbar` | per-widget Grid / Legend / Theme / Expand controls on `.dash-widget` cards |

The old "no client script for charts" absolute is retired; what stands is the
rendering story — SSR SVG, SSE animation, server-built tooltip content — with
hover/tooltip polish as **progressive enhancement**: every chart renders, updates,
and animates with `chart.js` absent.

The toolbar's contract is structural: a dashboard card is an
`<article class="dash-widget">` with a `.dash-widget-header` and a
`.dash-widget-body`; `chart-toolbar` finds these (and any patched in later, via
MutationObserver) and injects the controls. The Theme control swaps the
`[data-chart-theme]` attribute — pure CSS retint. The
[specimen](16-components.md#the-specimen) gallery and pantry's dashboard both use
this structure.

## `ch` — ClickHouse client, batcher, migrations

Postgres remains the system of record, always. ClickHouse is the **observation
store**: events, metrics, time-series — high-volume append-only data you chart and
aggregate but never transact against. It's in the framework not because every app
needs it but because it's the right tool when any app does — and bolting it on
later, per app, is exactly the duplication this framework exists to end.

```go
conn := ch.MustConn(ctx, cfg.ClickHouseDSN)          // native protocol
err  := ch.Migrate(ctx, conn, chMigrationsFS)         // goose clickhouse dialect,
                                                      // serialized by a PG advisory lock
ing  := ch.NewBatcher[AppEvent](conn, "app_events",   // generic buffered ingest
          ch.Batch{Size: 1000, FlushInterval: 5 * time.Second})
ing.Add(ev)                                           // non-blocking; drops are counted,
                                                      // never block the request path
```

- **Schema conventions**: `MergeTree` partitioned by month,
  `LowCardinality(String)` for enum columns, `Map(String,String)` label bags,
  `TTL` retention as config, `ReplacingMergeTree` for latest-state tables.
- **Ingest sources**: `sim` systems and handlers call `ing.Add` directly for
  domain events. Ingestion is fire-and-forget — ClickHouse being down degrades
  analytics, never the app.
- **Query → chart**: `ch.Rows[T]` scans into a typed struct; the adapters in
  `ch/chart.go` (`ToSeries`, `ToHBarSeries`, `ToStackedBarSeries`, `ToLineSeries`,
  `ToLineSeriesData`, `ToLineSeriesByEntity`) turn the row slice into a `chart`
  input via small projector funcs — no reflection, no struct-tag coupling.
  `ToLineSeriesByEntity` is the multi-line case: one `chart.LineSeries` per
  distinct entity, taken in first-seen order, the natural pair for the per-entity
  bucket/rollup shapes below. A `PageFunc` returning a `chart.Chart*Fragment`
  behind `ui.Defer` is the whole dashboard-widget wiring; `ToSeries` feeds the bar
  race (`chart.BarRaceInput.Bars`). Time bucketing is plain SQL
  (`toStartOfInterval`); the query is the API.
- **Optional everywhere**: no `CLICKHOUSE_DSN` → `ch` features off, app boots fine.
  docker-compose in the skeleton carries a commented-out clickhouse service.

### Time-series builders — the reusable layer over hand-written SQL

The four recurring analytics shapes — bucket, gap-fill, last-value-as-of, rollup —
were being copy-pasted bespoke and mis-aligned every time. `ch/timeseries.go`
factors them into a small set of composable builders. They are emphatically **not a
query DSL or ORM**: each builder emits a plain ClickHouse SQL string — a
well-tested fragment you can read, edit, and feed to `ch.Rows` exactly like the
hand-written SQL above. SQL stays the query language; these just stop the four
shapes from being re-typed.

Each builder is a struct with a `.SQL()` method (and a package-level `*SQL(b)`
twin for callers who prefer a function call), plus a `Query*[T]` convenience that
runs the built SQL through `ch.Rows`. Going through `ch.Rows` means the
**nil-`Conn` contract is inherited for free**: a nil `Conn` yields no rows and a
nil error, so ch-optional callers stay branch-free — the optional-`ch` story above
holds for the builders too. Because the builders are pure string functions, the
tests assert bucket alignment, the `WITH FILL` gap-fill, and the `argMax` as-of
against the emitted SQL with no live ClickHouse needed.

| Builder | Shape | Emits |
| --- | --- | --- |
| `Bucket` | aggregate a value column into aligned interval buckets; one row per non-empty bucket | `toStartOfInterval(time, INTERVAL n unit)` group-by, ordered by bucket |
| `GapFill` | embeds `Bucket`; one row per bucket across `[From, To)`, holes included | the `Bucket` query plus `ORDER BY bucket WITH FILL … STEP INTERVAL … INTERPOLATE (v AS fill)` |
| `AsOf` | latest reading per entity at/before a cutoff — "current state from an append-only log" | `argMax(value, time)` grouped per entity |
| `Rollup` | fine→coarse downsample for trend / exhaustion-projection queries | delegates to `Bucket` with `Interval = Every` (a wider `toStartOfInterval`) |

Two small pieces back them: the `Agg` enum (`Avg`, `Sum`, `Min`, `Max`, `Count`,
`Last` — a fixed set, never free-form SQL, so a builder can't emit an injectable
function name) and a duration→ClickHouse-interval helper that picks the coarsest
exact unit (`15 MINUTE`, `1 HOUR`, `1 DAY`) so bucket edges land on natural clock
boundaries. `Bucket` and `Rollup` take an optional `By` entity column — one series
per entity — which lines up with the `ORDER BY entity, bucket` the builders emit
and feeds straight into `ToLineSeriesByEntity` for a multi-line chart.

```go
// Bucket raw samples to 15-minute aligned boundaries, then chart them.
sql := ch.BucketSQL(ch.Bucket{
    Table: "rack_draw", Time: "ts", Value: "kw",
    Interval: 15 * time.Minute, Agg: ch.Avg,
})
type row struct {
    Bucket time.Time `ch:"bucket"`
    V      float64   `ch:"v"`
}
rows, _ := ch.Rows[row](ctx, conn, sql, since)   // nil conn → no rows, nil err
in := ch.ToLineSeriesData("kW", rows,
    func(r row) string { return r.Bucket.Format("15:04") },
    func(r row) float64 { return r.V })
```

The placeholder order is part of each builder's contract, and the `Query*`
wrappers bind it for you: `QueryBucket`/`QueryRollup`/`QueryAsOf` each take one
positional arg (the `since`/cutoff lower bound), while `QueryGapFill` binds four —
`From, To` for the scan filter then `From, To` again for the literal `WITH FILL`
bounds (pre-aligned to the bucket grid).

SQL is the query language — the `Batcher` and `ch.Rows[T]` are the whole API. `ch` is
app analytics: high-volume observations you aggregate and chart, with Postgres still the
system of record.
