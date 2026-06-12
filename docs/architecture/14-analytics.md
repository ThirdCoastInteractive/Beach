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
and animates with `chart.js` absent (hush proves it — its custom shell deliberately
doesn't load it).

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
  `ToLineSeriesData`) turn the row slice into a `chart` input via small projector
  funcs — no reflection, no struct-tag coupling. A `PageFunc` returning a
  `chart.Chart*Fragment` behind `ui.Defer` is the whole dashboard-widget wiring;
  `ToSeries` feeds the bar race (`chart.BarRaceInput.Bars`). Time bucketing is
  plain SQL (`toStartOfInterval`); the query is the API.
- **Optional everywhere**: no `CLICKHOUSE_DSN` → `ch` features off, app boots fine.
  docker-compose in the skeleton carries a commented-out clickhouse service.

SQL is the query language — the `Batcher` and `ch.Rows[T]` are the whole API. `ch` is
app analytics: high-volume observations you aggregate and chart, with Postgres still the
system of record.
