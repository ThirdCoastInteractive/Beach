# Architecture 14 — `chart` + `ch`: SSR charts and ClickHouse

[← docs index](../README.md) · prev: [API codegen](13-apigen.md) · next: [Example apps](15-examples.md)

## `chart` — server-rendered SVG charts

Pure-Go geometry computed on the server, plus templ components emitting the SVG — every
chart is final HTML, drawn before it reaches the browser. **18 chart types**: line/area, sparkline, bar
(horizontal/stacked/grouped/bullet), gauge (single/stacked), billboard KPI,
scatter, box plot, calendar heatmap, heatmap, streamgraph, sankey, chord,
bollinger, timeline, bar race.

The shape of the API — a `Layout*` function computes geometry, a templ component
renders it:

```go
layout := chart.LayoutHBar(chart.HBarSeries{Series: rows, Max: max, Unit: " kW"})
```

```templ
@chart.BarChartSVG(layout)
```

The library obeys the framework's laws by construction:

- **Token-themed**: series colors cycle Stellar's `--chart-qualitative-1..12`;
  diverging scales use `--chart-diverging-*`; emphasis names meaning through palette
  roles (`--error-*` for bad, etc.), never hue alone; no hardcoded colors in chart code
  ([ui token contract](06-ui.md)).
- **Accessible**: `<title>` elements on interactive regions, `role="img"` on
  roots, color never the only encoding (trend arrows accompany trend colors).
- **Responsive without scripts**: percentage coordinates for axes and text
  (rem-stable type) plus a nested `viewBox` with `preserveAspectRatio="none"`
  stretching the plot geometry — a chart fills any container while stroke widths
  stay constant.
- **Defer-native**: dashboard widgets are [`ui.Defer`](06-ui.md#deferred-sections)
  sections — kit-styled skeleton, exact reserved dimensions, `@get` on viewport
  intersection.
- **Live, and animated, with no client script**: a chart fragment is just templ, so it
  works as a [`sim` projection](08-sim.md#projections-change-detection--patches) or
  hub-topic patch. SVG elements are DOM elements, so Datastar patches *inside* the SVG —
  a `<rect>`, `<path>`, or `<g>` by id — and CSS transitions (duration + easing from the
  design tokens) tween geometry between states. Animation is the server pushing new
  numbers, not a script playing a timeline. **bar race** is the showcase: each tick the
  server re-layouts the ranked bars and patches them; the bars slide and reorder over
  SSE, no animation code on the client.

All 18 geometry types ship from day one; the integration polish — live wiring,
empty/error states — lands per type, as the example apps render them. No type gets
polished before something eats it.

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

- **Schema conventions**, stamped into the skeleton as example migrations:
  `MergeTree` partitioned by month, `LowCardinality(String)` for enum columns,
  `Map(String,String)` label bags, `TTL` retention as config,
  `ReplacingMergeTree` for latest-state tables.
- **Ingest sources**: an `app.Events(...)` hook records request/page events;
  `sim` systems and handlers call `ing.Add` directly for domain events. Ingestion
  is fire-and-forget — ClickHouse being down degrades analytics, never the app.
- **Query → chart**: thin typed query helpers (`ch.Rows[T]`) feed `chart.Layout*`
  directly; a `PageFunc` + `ui.Defer` per dashboard widget is the whole wiring.
  Time bucketing is plain SQL (`toStartOfInterval`); the query is the API.
- **Optional everywhere**: no `CLICKHOUSE_DSN` → `ch` features off, app boots fine.
  docker-compose in the skeleton carries a commented-out clickhouse service.

SQL is the query language — the `Batcher` and `ch.Rows[T]` are the whole API. `ch` is
app analytics: high-volume observations you aggregate and chart, with Postgres still the
system of record.
