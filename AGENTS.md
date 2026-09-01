# Agents

the-beach is a hypermedia-first Go web framework. The public API is `pkg/`. Architecture docs under `docs/architecture/` are the design of record.

**Do not invent a parallel of anything already in `pkg/`.** Map the task to a package, read it, then call it.

1. Catalog: [`.grok/skills/beach/references/packages.md`](.grok/skills/beach/references/packages.md)
2. Protocol: [`.grok/skills/beach/SKILL.md`](.grok/skills/beach/SKILL.md)
3. UI: [`.grok/skills/beach-ui/SKILL.md`](.grok/skills/beach-ui/SKILL.md)

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
