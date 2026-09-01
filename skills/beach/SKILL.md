---
name: beach
description: >
  Work with Beach by discovering and calling existing packages
  instead of reinventing them. Use whenever writing or reviewing Go, templ,
  HTTP handlers, UI, auth, sessions, passwords, charts, Postgres, ClickHouse,
  SSE, WebSockets, ECS, sim, i18n, email, SMS, storage, sqlc, goose, Datastar,
  driftwood, rybitten, or scaffolding in this repo or a Beach app. Use when
  the user mentions Beach, pkg/, driftwood, beach-apigen, beach-vet,
  hub, sim, quantity, or runs /beach. Always inventory pkg/ before adding a
  helper, component, middleware, chart, login flow, cache, websocket, or
  mail/SMS/file client.
---

# Beach

Beach is a hypermedia-first, server-rendered Go framework. The public API is `pkg/`. Architecture docs under `docs/architecture/` are the design of record.

Agents fail here by writing a parallel of something that already exists. The fix is discovery, then a call.

## Before any new code

1. Open [references/packages.md](references/packages.md). Map the task to a row.
2. Read that package's non-test `.go` / `.templ` files — the exported types are the API.
3. Read the architecture doc named in the row.
4. Copy the wiring from the named example app (`cmd/examples/…`).
5. Write **app** code that imports and calls the package.

If you cannot name the `pkg/…` owner, discovery is not done. Do not start a sibling helper "for now".

New framework surface belongs in the owning package (or a new `pkg/` only when no row owns the job). App folders do not grow secret frameworks.

## Stack (not pluggable)

net/http (Go 1.22 ServeMux) · templ · Datastar free core · Tailwind v4 + rybitten tokens · pgx/v5 · sqlc · goose · Postgres LISTEN/NOTIFY · custom `ecs` · `coder/websocket` via `App.Socket`.

Do not add chi, gin, echo, HTMX, Alpine, React, a second CSS system, bcrypt, gorilla/sessions, Chart.js, or a sync engine.

## Laws

Enforced by types, the App builder, or `beach-vet` — see [RFC 02](../../docs/rfc/02-scope.md).

- **Four handler shapes.** `PageFunc`, `ActionFunc`, `StreamFunc`, `SocketFunc`. `app.Raw` is the escape hatch; naked `http.HandlerFunc` elsewhere fails vet.
- **`pg.InTx`** is the transaction path.
- **Typed Datastar only.** `pkg/datastar` builders; raw `data-*` strings fail vet.
- **HTML and CSS first.** Platform markup (`<details>`, `<dialog>`, `popover`, `:checked`) for local UI. Datastar is for server round-trips, signals, SSE.
- **Tokens, not literals.** No hex/OKLCH in templ or kit CSS. `ui.Icon`, not raw `<i>`.
- **Accessible by construction.** WCAG 2.1 AA is a law, not a review note: names come from the i18n catalog, controls are associated with their label/help/error, everything focusable draws a ring, no ARIA role you cannot honour, layouts reflow to 320px. Three `beach-vet` rules plus the kit's own tests hold it. [06-ui.md](../../docs/architecture/06-ui.md#accessible-by-construction), [RFC 06](../../docs/rfc/06-accessibility.md). Touching UI? Follow the `beach-a11y` skill.
- **One SSE model.** Everything live is a `hub` topic. No second pubsub.
- **No sync engine.** The wire unit is HTML. Read-path liveness is the hub. [20-sync.md](../../docs/architecture/20-sync.md).
- **NOTIFY payloads are ids**, then re-query.
- **First paint ≤14KB compressed.** Heavier blocks go through `ui.Defer` with reserved dimensions.
- **One identity type.** Session user and request principal are one story (`session` + `auth`), not parallel structs.

## App shape

Stamp with `beach new`. Layout is [01-layout.md](../../docs/architecture/01-layout.md):

```
main.go              thin: load config → construct → wire → run
config.go            config.MustLoad, embed config.Core
internal/web         server struct, handlers, .templ views
internal/store       domain + Postgres
internal/db          sqlc + sql/migrations + sql/queries
```

`main.go` only wires. Do not split handlers from the views that render them.

Imports:

```go
import (
    beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
    "github.com/ThirdCoastInteractive/Beach/pkg/pg"
)
```

## Tooling

| Command | When |
| --- | --- |
| `make gen` | after `.templ`, CSS, sqlc SQL, or `components.beach.yaml` |
| `make vet` | includes `beach-vet` |
| `make test` / `make build` | depend on `gen` |
| `beach new <app>` | new consuming app |
| `beach sql new <name>` | next goose migration (`NNNNN_name.sql`, exclusive version) |
| `beach ecs gen` | ECS components |
| `beach i18n --write` | catalog from `i18n.T("key")` calls |
| `beach i18n` | verify — run it in CI beside `beach-vet`; it is a command, not a vet rule |

Generated `*_templ.go` is committed. Verification: gen → vet → tests → browser (no console errors, change visible, no 404s). Doctrine: [10-tooling.md](../../docs/architecture/10-tooling.md).

## Other passes

| Task | Also follow |
| --- | --- |
| page, component, form, chart, token, icon, Datastar attribute | `beach-ui` |
| anything a person sees, hears or tabs to | `beach-a11y` |
| writing or changing a test | `beach-test` |

## Gaps vs invention

A local workaround in an app is not a license to fork the framework into `internal/`. If `pkg/` cannot do the job, that is a Beach card, not a new helper.
