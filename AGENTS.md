# Agents

Beach is a hypermedia-first Go web framework. The public API is `pkg/`. Architecture docs under `docs/architecture/` are the design of record.

**Do not invent a parallel of anything already in `pkg/`.** Map the task to a package, read it, then call it.

1. Catalog: [`skills/beach/references/packages.md`](skills/beach/references/packages.md)
2. Protocol: [`skills/beach/SKILL.md`](skills/beach/SKILL.md)
3. UI: [`skills/beach-ui/SKILL.md`](skills/beach-ui/SKILL.md)

Index: [docs/README.md](docs/README.md). Layout: [docs/architecture/01-layout.md](docs/architecture/01-layout.md).

## Stack (not pluggable)

net/http · templ · Datastar free core · Tailwind v4 + rybitten · pgx · sqlc · goose · Postgres LISTEN/NOTIFY · custom `ecs` · `App.Socket` (vendored `coder/websocket`).

## Front door

```go
import beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
```

Handlers are `PageFunc` / `ActionFunc` / `StreamFunc` / `SocketFunc`. Transactions are `pg.InTx`. Datastar attributes come from `pkg/datastar`. Components come from `pkg/ui/driftwood`. Charts come from `pkg/chart`.

## Verify

`make gen` after `.templ` / CSS / sqlc / ECS schema. Then `make vet` (includes `beach-vet`) and `make test`. Browser-check UI changes.

## Other passes

| Task | Follow |
| --- | --- |
| page, component, form, chart, token, icon, Datastar | [`skills/beach-ui`](skills/beach-ui/SKILL.md) |
| anything a person sees, hears or tabs to | [`skills/beach-a11y`](skills/beach-a11y/SKILL.md) |
| writing or changing a test | [`skills/beach-test`](skills/beach-test/SKILL.md) |
