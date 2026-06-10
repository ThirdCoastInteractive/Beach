# Architecture 13 — `apigen`: annotation-driven codegen from SQL

[← docs index](../README.md) · prev: [Auth](12-auth.md) · next: [Analytics](14-analytics.md)

`beach-apigen` is a **sqlc process plugin** that reads annotations on the SQL
queries the app already writes and generates the wiring around them. SQL stays the
single source of truth — the same ethos as sqlc, templ, and `beach ecs gen`.

The generator emits **hypermedia only**. There is no OpenAPI emitter, no JSON
content negotiation, and no body-shaping annotations — machine APIs are a charter
[non-goal](../rfc/01-charter.md#non-goals). If a beach app ever genuinely needs
one, that's its own RFC.

## The annotation grammar — six annotations

Annotations are comments on named sqlc queries
(`internal/db/sql/queries/<entity>/*.sql`, one directory per entity, so each
domain's queries and generated surface stay together):

```sql
-- name: GetItem :one
-- @api GET /items/{id}
-- @page page.ItemDetail
-- @fragment page.ItemCard
SELECT id, name, quantity, location_id, expires_at
FROM items WHERE id = @id AND deleted_at IS NULL;

-- name: CreateItem :one
-- @api POST /items
-- @requires pantry:write
-- @notify items
-- @fragment page.ItemCard
INSERT INTO items (...) VALUES (...) RETURNING id, name, quantity, location_id;
```

| Annotation | Effect |
| --- | --- |
| `@api METHOD /path` | Route registration + handler stub |
| `@page pkg.Component` | GET renders this templ page — generates a `PageFunc` |
| `@fragment pkg.Component` | The Datastar fragment for refresh/mutation patches |
| `@notify channel` | Mutation publishes `{table,id,op}` to Postgres NOTIFY + hub topic |
| `@requires permission` | Generated handler calls the principal check ([auth](12-auth.md)); the authz analyzer verifies coverage |
| `@handler skip` | Stub suppressed; hand-write the handler (analyzer still checks it) |

## What gets generated

- `@api GET` + `@page`/`@fragment` → a generated `beach.PageFunc` (dual-purpose
  branch included, like every PageFunc).
- `@api POST/PUT/DELETE` → a generated `beach.ActionFunc`: bind + validate params
  from the query's argument struct, `@requires` principal check, sqlc call,
  `@notify` publish, `@fragment` patch.
- `@notify` → the NOTIFY trigger migration snippet and the hub topic plumbing —
  the [external-writer seam](08-sim.md#other-writers) wired automatically.

Generated handlers are stubs the same way sqlc output is: regenerate, never edit.
Anything custom goes through `@handler skip` and a hand-written handler.

## Build wiring

The generator is a standalone binary (`cmd/beach-apigen`) registered as a sqlc
plugin, so `make gen`'s existing `sqlc generate` step runs it — no new pipeline
stage:

```yaml
# skeleton sqlc.yaml
plugins:
  - name: beach-apigen
    process: { cmd: ./bin/beach-apigen }
sql:
  - engine: postgresql
    queries: [internal/db/sql/queries/<entity>, ...]
    codegen:
      - plugin: beach-apigen
        out: api
```
