# Beach package catalog

Import prefix: `github.com/ThirdCoastInteractive/Beach/pkg/<name>`

Front door is `pkg/beach` (alias the hyphenated module path):

```go
import beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
```

Docs of record: [docs/README.md](../../../docs/README.md). Layout: [docs/architecture/01-layout.md](../../../docs/architecture/01-layout.md). Laws: [docs/rfc/02-scope.md](../../../docs/rfc/02-scope.md).

If a row exists, **call that package**. New `pkg/` code only when no row owns the job.

## Intent → package

| You are about to… | Use | Do not invent |
| --- | --- | --- |
| Boot an HTTP app, register routes, start/shutdown | `beach` | chi/echo/gin, a custom server wrapper, hand-rolled healthz/static/CSP |
| Write a page / mutation / SSE stream / WebSocket | `beach` `PageFunc`/`ActionFunc`/`StreamFunc`/`SocketFunc` | `http.HandlerFunc` (except `app.Raw`) |
| Load env config | `config` | `os.Getenv` sprawl, viper, envconfig |
| Open Postgres, migrate, transact, LISTEN/NOTIFY | `pg` | `sql.Open`, hand-rolled Begin/Commit, a notify helper |
| Emit Datastar `data-*` | `datastar` | raw `data-on:` strings, `templ.Attributes{"data-…"}` |
| SPA-style nav inside a shell | `datastar.Navigate` + `beach.Swap` | `pushState` + fetch, HTMX, a client router |
| Render a button, form, table, modal, shell, alert… | `ui/driftwood` | a local component kit, DaisyUI, shadcn, hand-rolled CSS kit |
| Icon glyph | `ui.Icon` | raw `<i class="icon-…">`, lucide/heroicons npm |
| Lazy section that must not pop-in | `ui.Defer` | spinner-then-replace, client skeleton libs |
| Chart / SVG viz | `chart` | Chart.js, D3, gnuplot, a new SVG helper |
| ClickHouse ingest or dashboard query | `ch` | a raw clickhouse-go wrapper, a query DSL |
| Live fan-out to browsers | `hub` | a custom pubsub, redis pubsub, gorilla channels |
| In-process cache invalidated by NOTIFY | `cache` | a map+mutex, redis/valkey "just for cache" |
| Login session / CSRF / cookie identity | `session` | JWT-in-cookie, gorilla/sessions, a homebrew token |
| Anonymous visitor id without Postgres | `session.AnonStore` | a custom visitor cookie |
| Hash/verify passwords | `passwords` | bcrypt wrapper, argon2 copy-paste |
| Principal, RBAC, login, register, API tokens | `auth` | casbin, a parallel user/role model |
| Translate UI strings | `i18n` | gotext, a map[string]string helper |
| Tick-driven live state | `sim` over `ecs` | a game loop, a `sync.Mutex` world, another ECS |
| Entity/component storage (no web) | `ecs` | entt/donburi, a map of structs |
| Send email | `mailer` | net/smtp helper, a Mailgun client in the app |
| Send SMS | `sms` | a Twilio client in the app |
| Store files (disk, S3/R2, CF Images) | `storage` (+ `storage/s3`, `storage/cfimages`) | a local upload helper, minio client in the app |
| Hosted video / tus upload (Cloudflare Stream) | `storage/cfstream` | a Cloudflare Stream client in the app |
| Render Markdown to sanitized HTML | `md` | a goldmark wrapper, blackfriday, a sanitizer copy |
| Record cookie preferences / GPC / DNT | `consent` | a cookie helper, OneTrust, a CMP |
| Color palette / gamut / series CSS vars | `rybitten` | a palette generator, hardcoded hex |
| Values with units (W/kW, °C/°F) | `quantity` | a units library, stringly-typed "2500W" |
| Scaffold an app | `cmd/beach` `new` | a hand-made main.go that ignores the template |
| Stamp a goose SQL migration | `cmd/beach` `sql new` | hand-numbered `0000N_*.sql` (collides under parallel agents) |
| Generate handlers from SQL | `cmd/beach-apigen` | OpenAPI, json-only CRUD, hand-wired sqlc routes |
| ECS component structs | `cmd/beach` `ecs gen` | hand-written component types out of band |
| i18n catalog from `T("key")` | `cmd/beach` `i18n` | a spreadsheet of keys |
| Lint house rules | `cmd/beach-vet` | a custom linter for the same rules |

## Packages

### `pkg/beach` — HTTP front door

**Read:** [03-http.md](../../../docs/architecture/03-http.md), [09-lifecycles.md](../../../docs/architecture/09-lifecycles.md), RFC [05-websockets.md](../../../docs/rfc/05-websockets.md)

**Copy:** any `cmd/examples/*/main.go`

**API:** `New(Config)`, `App.Page/Action/Stream/Socket/Raw`, `PageFunc`, `ActionFunc`, `StreamFunc`, `SocketFunc`, `Ctx`, `View`, `Swap`, `Patch`/`Patches` (`Redirect`, `Script`, fragment), `Bind`, `Invalid`, guards (`Authed`, `Role`, `Can`, `TokenAuthed`, `InScope`), `CSPDefault`, `DialSocket`, `Relay`. Config wires `Sessions`, `AnonymousSessions`, `Principals`, `Tokens`, `Hub`, `Healthz`, `Static`, `Middleware`, `SSECompression`, `Sockets`.

The app's `internal/web` package holds the server struct, handler methods, and `.templ` views together (see the example apps).

### `pkg/config` — env loader

**Read:** [02-boot-spine.md](../../../docs/architecture/02-boot-spine.md)

**API:** embed `config.Core`; `Load[T]()`, `MustLoad[T]()`, `LoadEnvFile`. Tags: `env:"A|B"`, `default`, `required`, `lower`/`upper`/`trim`/`trimslash`/`url`.

### `pkg/pg` — Postgres

**Read:** [02-boot-spine.md](../../../docs/architecture/02-boot-spine.md)

**API:** `Pool` / `MustPool`, `Migrate` (advisory lock + goose), `InTx`, `Listen`, `Notify`. NOTIFY payloads are **ids only**. Domain queries live in the app's `internal/db` (sqlc). `pgtype` stays inside `internal/db`.

### `pkg/datastar` — the only `data-*` emitter

**Read:** [03-http.md](../../../docs/architecture/03-http.md), [06-ui.md](../../../docs/architecture/06-ui.md)

**API:** `On`, `OnClick`, `OnSubmit`, `OnInterval`, `Init`, `Bind`, `Signals`, `Signal`, `Show`, `Text`, `Class`, `ClassToggle`, `AttrBind`, `Indicator`, `Ref`, `Navigate`, `PopstateNav`, `ActiveWhen`, `PathSignal`, `IsDatastar`, `NewSSE`. Pass `datastar.Attrs` on driftwood props.

### `pkg/ui` — Icon + Defer

**Read:** [06-ui.md](../../../docs/architecture/06-ui.md)

**API:** `Icon`, `Defer`. No Kit interface, no registry, no `c.Kit()`.

### `pkg/ui/driftwood` — the component kit

**Read:** [16-components.md](../../../docs/architecture/16-components.md), then `pkg/ui/driftwood/*.templ` + `props.go`

**Copy:** `pkg/ui/specimen`, `cmd/examples/pantry`

**API:** package-level templ funcs (`Button`, `Field`, `Input`, `Form`, `Card`, `Shell`, `Table`, `Modal`, `Alert`, …). Props structs in `props.go`. Call `@driftwood.Card(p) { … }`.

Living inventory: `ui/specimen` mounted at `/specimen` by every example.

### `pkg/chart` — SSR SVG charts

**Read:** [14-analytics.md](../../../docs/architecture/14-analytics.md)

**Copy:** pantry dashboards, boardwalk bar race, `ui/specimen` chart gallery

**API:** `Layout*` geometry + `Chart*Fragment` / SVG templ. Types include bar, stacked/grouped/bullet, line, sparkline, bollinger, box, bundle, calendar, chord, gauge/billboard, geoclock, geomap, globe, heatmap, horizon, ridgeline, difference, timeline, phase, sankey, scatter, stream, single-line, bar-race. Token series via `ColorVar`. Client interaction (tips/toolbar) is already in `pkg/beach/view/static/js/chart*.js`.

### `pkg/ch` — ClickHouse

**Read:** [14-analytics.md](../../../docs/architecture/14-analytics.md)

**Copy:** pantry `internal/analytics`, boardwalk event firehose

**API:** `Connect`/`MustConn`, `Migrate`, `NewBatcher[T]`, `Rows[T]`, timeseries (`Bucket`/`GapFill`/`AsOf`/`Rollup`), chart adapters (`ToHBarSeries`, `ToLineSeries`, …). No query DSL — SQL in, projector funcs out. Nil conn is a documented no-op so charts can render empty.

### `pkg/hub` — SSE topic fan-out

**Read:** [04-hub.md](../../../docs/architecture/04-hub.md), [20-sync.md](../../../docs/architecture/20-sync.md)

**Copy:** driftbottle

**API:** `New`, `Publish`, `Subscribe`, `Ticker`, `Event` (`Mode`: morph/append/prepend, `Target`). Render once per topic, drop-on-full, catch-up on reconnect.

### `pkg/cache` — in-process cache

**Read:** [05-services.md](../../../docs/architecture/05-services.md)

**API:** `Snapshot[T]`, `Keyed[K,V]`, `InvalidateOn` / `InvalidateOnParse` (NOTIFY).

### `pkg/session` — Postgres sessions + anon cookies

**Read:** [05-services.md](../../../docs/architecture/05-services.md), [12-auth.md](../../../docs/architecture/12-auth.md)

**API:** `NewStore`, `ValidateCSRF`, `UserFrom`; `NewAnonStore`, `AnonFrom`. SHA256 token PK, rotation, sliding TTL, RevokeAll, per-session CSRF. Embeds its own goose migrations.

### `pkg/passwords` — argon2id

**Read:** [12-auth.md](../../../docs/architecture/12-auth.md)

**API:** PHC strings, `Hash`, `Verify`, `NeedsRehash`. Defaults 128MB / t=4 / p=4. Zero deps beyond `x/crypto`.

### `pkg/auth` — principal, login, tokens

**Read:** [12-auth.md](../../../docs/architecture/12-auth.md)

**Copy:** pantry

**API:** `Principal`, `AnonymousPrincipal`, `NewAuthenticator`, `RegisterUser`, `MintPretoken`/`VerifyPretoken`, API `Token`. Permissions are `resource:action`, resolved at login. Guards live on `beach`. Embeds identity + token migrations.

### `pkg/i18n` — translations

**Read:** [05-services.md](../../../docs/architecture/05-services.md)

**API:** `Load`, `SetDefault`, `T(ctx, key)`, `WithLocale`, `WithCatalog`, middleware. Literal keys only (`T(ctx, "save")`, never a variable). App catalog via `SetDefault` or context; `beach i18n --write` regenerates `catalog.json`.

### `pkg/ecs` — standalone archetype ECS

**Read:** [07-ecs.md](../../../docs/architecture/07-ecs.md), RFC [03-ecs.md](../../../docs/rfc/03-ecs.md)

**Copy:** boardwalk. Schema: `components.beach.yaml` → `beach ecs gen`.

**API:** `Store`, `Entity`, `Register`, `Add`/`Get`/`Set`/`Mutate`/`Remove`/`Has`, `Query`/`Query2`/`Changed`, relations, snapshots, `View` (immutable off-loop read). Zero beach imports.

### `pkg/sim` — tick loop over ecs

**Read:** [08-sim.md](../../../docs/architecture/08-sim.md)

**Copy:** boardwalk

**API:** `New`, command channel, `System`, `Ask`/`AskFunc`, `Project`/`ProjectAny`, `Commit`, write-behind `Mirror`, `Snapshot` → `ecs.View`. Single writer goroutine. `sim` is the only bridge from `ecs` to the web packages.

### `pkg/mailer` — transactional email

**Copy:** `cmd/examples/booking-manager`

**API:** `New(Config)` / `ConfigFromEnv` → Mailgun or `LogMailer`. `Email`, `HTMLFromText`.

### `pkg/sms` — transactional SMS

**Copy:** booking-manager

**API:** `New(Config)` / `ConfigFromEnv` → Twilio or `LogSender`. Mirrors mailer.

### `pkg/storage` — files

**Read:** `pkg/storage`

**API:** `Store` (`Put`/`Open`/`Stat`/`Delete`/`URL`). Backends: `storage.Local`, `storage/s3`, `storage/cfimages`.

### `pkg/storage/cfstream` — Cloudflare Stream

**API:** `New(accountID, token, customerCode)`, `CreateTus` (returns Location + UID; hand the Location to the browser unmodified), `Delete`, `IframeURL`, thumbnail URL. The browser PATCHes the file; the API token never leaves the server.

### `pkg/md` — Markdown

**API:** `Render` / `RenderWith` through goldmark then bluemonday. Profiles: `Strict` (no tags), `Post`, `Article` (headings, links, CF Images, code, quotes, tables, `:::video` → Stream iframes). Pair with `driftwood.MarkdownEditor` + `/static/js/md-editor.js`.

### `pkg/consent` — cookie preference record

**API:** `Parse` / `Encode`, `FromRequest`, `BrowserOptOut` (GPC/DNT), `Allows`, `NeedsPrompt`, `Cookie` / `ExpireCookie`, `DomainForHost`. Necessary cookies are not stored; optional slugs stay off until recorded yes. Driftwood's `ConsentBanner` / `ConsentManager` / `ConsentLink` render the prompt — they do not import this package.

### `pkg/rybitten` — RYB color

**Read:** [19-rybitten.md](../../../docs/architecture/19-rybitten.md)

**API:** cubes/gamuts, `RYBHSL2RGB`, `SeriesVars` → `--color-series-*` in `pkg/beach/view/css/input.css`. Zero deps. Do not import this from `chart` (CSS vars are the seam).

### `pkg/quantity` — measured values

**API:** `Quantity`, native unit stored verbatim, convert on display via `In` + request preference middleware. Physical readings, not money.

## Tooling (`cmd/`)

| Binary | Job |
| --- | --- |
| `cmd/beach` | `new`, `sql new`, `ecs gen`, `i18n` |
| `cmd/beach-apigen` | sqlc plugin: SQL annotations → routes, stubs, NOTIFY. [13-apigen.md](../../../docs/architecture/13-apigen.md) |
| `cmd/beach-vet` | house-rule analyzers. [10-tooling.md](../../../docs/architecture/10-tooling.md) |

Private (do not import from apps): `internal/deps`, `internal/lint`, `internal/perf`.

## Example apps (copy these, don't re-derive)

| App | Proves |
| --- | --- |
| `cmd/examples/boardwalk` | `sim`/`ecs`, bar race, snapshots |
| `cmd/examples/driftbottle` | `hub`/SSE, `App.Socket`, anon sessions |
| `cmd/examples/pantry` | `auth`, apigen, `ch`/`chart`, CRUD forms |
| `cmd/examples/booking-manager` | `mailer`/`sms`, Postgres-only |

Shape of a consuming app: [01-layout.md](../../../docs/architecture/01-layout.md#what-a-consuming-app-looks-like). Stamp one with `beach new`.
