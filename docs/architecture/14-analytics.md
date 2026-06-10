# Architecture 14 — `chart` + `ch`: SSR charts and ClickHouse

[← docs index](../README.md) · prev: [API codegen](13-apigen.md) · next: [Example apps](15-examples.md)

Two packages with different provenance, stated honestly: `chart` is **lifted from
working Atlas code** (`Y:\Atlas\pkg\chart` + `internal\view\chartview`); `ch` is
**built fresh** to the design in Atlas's Telemetry RFC/Functional Spec
(`Y:\Atlas\docs\Proposals\Telemetry\`) — Atlas wrote ~900 lines of design and zero
lines of implementation, so there is nothing to lift, only a good blueprint.

## `chart` — server-rendered SVG charts

~3.7 KLOC of pure-Go geometry (no D3, no Plotly, no canvas, no client rendering)
plus templ components emitting the SVG. **22 chart types**: line/area, sparkline,
bar (horizontal/stacked/grouped/bullet), gauge (single/stacked), billboard KPI,
scatter, box plot, calendar heatmap, heatmap, streamgraph, sankey, chord, bundle,
bollinger, difference, horizon, timeline, ridgeline, bar race.

The shape of the API — a `Layout*` function computes geometry, a templ component
renders it:

```go
layout := chart.LayoutHBar(chart.HBarSeries{Series: rows, Max: max, Unit: " kW"})
```

```templ
@chart.BarChartSVG(layout)
```

Why it fits the-beach like a glove:

- **It already obeys the laws.** Token-themed (`--color-series-a..o`, semantic
  `--color-good/warn/bad` — the [ui token contract](06-ui.md)), no hardcoded
  colors, `<title>`/`role="img"` accessibility, CSS-only hover. Rendering is
  server-side, so charts cost zero JS and can't pop in.
- **Responsive without scripts**: percentage coordinates for axes/text (rem-stable
  type) + a nested `viewBox` with `preserveAspectRatio="none"` stretching the plot
  geometry — fills any container, stroke widths stay constant.
- **Defer-native**: Atlas already lazy-loads charts on viewport intersection with a
  skeleton placeholder — that *is* [`ui.Defer`](06-ui.md#deferred-sections); the
  wrapper component merges into it.

What the framework version adds over Atlas (the audit's gap list):

- **Live charts**: a chart fragment is just templ, so it works as a
  [`sim` projection](08-sim.md#projections-change-detection--patches) or hub-topic
  patch — re-layout on new data, publish, every subscriber's SVG morphs. Atlas's
  charts are static-at-render; the-beach's machinery makes them live for free.
- Empty/error states and the skeleton loader as kit-styled components.
- Legend series-toggling via Datastar signals (no localStorage hacks).

## `ch` — ClickHouse client, batcher, migrations

Postgres remains the system of record, always. ClickHouse is the **observation
store**: events, metrics, time-series — high-volume append-only data you chart and
aggregate but never transact against. The Atlas Telemetry design, generalized:

```go
conn := ch.MustConn(ctx, cfg.ClickHouseDSN)          // native protocol
err  := ch.Migrate(ctx, conn, chMigrationsFS)         // goose clickhouse dialect,
                                                      // serialized by a PG advisory lock
ing  := ch.NewBatcher[AppEvent](conn, "app_events",   // generic buffered ingest
          ch.Batch{Size: 1000, FlushInterval: 5 * time.Second})
ing.Add(ev)                                           // non-blocking; drops are counted,
                                                      // never block the request path
```

- **Schema conventions** (from the Atlas spec, stamped into the skeleton as example
  migrations): `MergeTree` partitioned by month, `LowCardinality(String)` for enum
  columns, `Map(String,String)` label bags, `TTL` retention as config,
  `ReplacingMergeTree` for latest-state tables.
- **Ingest sources**: an `app.Events(...)` hook records request/page events;
  `sim` systems and handlers call `ing.Add` directly for domain events. Ingestion
  is fire-and-forget — ClickHouse being down degrades analytics, never the app.
- **Query → chart**: thin typed query helpers (`ch.Rows[T]`) feed `chart.Layout*`
  directly; a `PageFunc` + `ui.Defer` per dashboard widget is the whole wiring.
  Time-bucketing helpers (`ch.Buckets(from, to, step)`) cover the common
  group-by-time case without inventing a query DSL.
- **Optional everywhere**: no `CLICKHOUSE_DSN` → `ch` features off, app boots fine.
  docker-compose in the skeleton gains a commented-out clickhouse service.

What we are deliberately not building (grug): a query builder/DSL (write SQL),
a metrics/observability product (this is app analytics, not APM), Kafka or any
queue in front of ingest (the batcher is the buffer; revisit only if a real app
outgrows it).
