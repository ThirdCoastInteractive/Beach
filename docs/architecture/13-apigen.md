# Architecture 13 — `apigen`: annotation-driven codegen from SQL

[← docs index](../README.md) · prev: [Auth](12-auth.md) · next: [Analytics](14-analytics.md)

Adapted from Atlas (`Y:\Atlas\cmd\atlas-apigen`), the most framework-worthy thing in
that codebase: a **sqlc process plugin** that reads annotations on the SQL queries
the app already writes and generates the wiring around them. SQL stays the single
source of truth — the same ethos as sqlc, templ, and `beach ecs gen`.

## The annotation grammar

Annotations are comments on named sqlc queries
(`Y:\Atlas\internal\db\sql\queries\<entity>\*.sql`):

```sql
-- name: GetDevice :one
-- @api GET /devices/{id}
-- @page page.DeviceDetail
-- @fragment page.DeviceCard
-- @expand model,rack,location
-- @sortable name,serial,u_position:desc
SELECT id, name, serial, u_position, model_id, rack_id, location_id, created_at
FROM devices WHERE id = @id;

-- name: CreateDevice :one
-- @api POST /devices
-- @requires devices:write
-- @notify devices
-- @fragment page.DeviceCard
INSERT INTO devices (...) VALUES (...) RETURNING ...;
```

| Annotation | Effect in the-beach |
| --- | --- |
| `@api METHOD /path` | Route registration + handler stub |
| `@page pkg.Component` | GET renders this templ page — generates a `PageFunc` |
| `@fragment pkg.Component` | The Datastar fragment for refresh/mutation patches |
| `@notify channel` | Mutation publishes `{table,id,op}` to Postgres NOTIFY + hub topic |
| `@requires permission` | Generated handler calls the principal check ([auth](12-auth.md)); the authz analyzer verifies coverage |
| `@expand a,b` | `?expand=` inline-join support on reads |
| `@sortable col[:dir],…` | Validated sort params on `:many` queries |
| `@readonly` / `@writeonly` | Column excluded from request body / response |
| `@handler skip` | Stub suppressed; hand-write it (analyzer still checks it) |

## What gets generated

Atlas's generator emits JSON-first Echo handlers + OpenAPI. The beach port keeps the
grammar and the plugin mechanism but generates **beach shapes**:

- `@api GET` + `@page`/`@fragment` → a generated `beach.PageFunc` (dual-purpose
  branch included, like every PageFunc).
- `@api POST/PUT/DELETE` → a generated `beach.ActionFunc`: bind + validate params
  from the query's argument struct, `@requires` principal check, sqlc call,
  `@notify` publish, `@fragment` patch.
- `@notify` → the NOTIFY trigger migration snippet and the hub topic plumbing —
  the [external-writer seam](08-sim.md#other-writers) wired automatically.
- **Optionally** (`beach.Config{API: true}` apps like Atlas): `api/openapi.yaml`
  (OpenAPI 3.1 derived from query shapes) + JSON content negotiation in the
  generated handlers via Atlas's Dispatch pattern (templ for browsers, JSON for
  machine clients) + oapi-codegen models. This is for integrations and MCP
  servers — never for the UI (see the [charter non-goal](../rfc/01-charter.md#non-goals)).

Generated handlers are stubs the same way sqlc output is: regenerate, never edit.
Anything custom goes through `@handler skip` and a hand-written handler.

## Build wiring

Same mechanism as Atlas: the generator is a standalone binary (`cmd/beach-apigen`)
registered as a sqlc plugin, so `make gen`'s existing `sqlc generate` step runs it —
no new pipeline stage:

```yaml
# skeleton sqlc.yaml
plugins:
  - name: beach-apigen
    process: { cmd: ./bin/beach-apigen }
sql:
  - engine: postgresql
    queries: [internal/db/sql/queries/<entity>, ...]   # one dir per entity (Atlas pattern)
    codegen:
      - plugin: beach-apigen
        out: api
```

Atlas's per-entity query directories (50+ in Atlas) replace manifold's per-feature
files in the skeleton — same idea, scales better with the generator.

## What we're not taking

`atlas-seed` and `intel-dump` are Atlas-domain tools (the patterns are nice; the
code is theirs). Atlas's Postgres-backed session store stays in Atlas — the-beach
keeps Valkey sessions with Atlas's hardening ([12-auth.md](12-auth.md)). Atlas's
multi-realm (operator/portal) cookie scoping is generalized into session config
rather than lifted as a hardcoded pair.
