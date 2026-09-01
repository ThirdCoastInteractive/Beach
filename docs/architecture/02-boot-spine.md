# Architecture 2 — Boot spine: `config`, `pg`

[← docs index](../README.md) · prev: [Layout](01-layout.md) · next: [HTTP layer](03-http.md)

## config

One struct per app, `env:` tags, a reflect-based loader, `A|B` fallbacks, modifier
flags (`lower`, `trimslash`, `url`), `.env` via godotenv, and `MustLoad`, which
aborts on missing required values. Feature blocks (SMTP, Stripe, CDN credentials)
stay app-side; the framework defines only the core block:

```go
type Core struct {
    AppEnv     string `env:"APP_ENV,lower"`      // "production" enables release behavior
    Port       string `env:"PORT"`
    DSN        string `env:"POSTGRES_DSN|DATABASE_URL"`
    BaseURL    string `env:"BASE_URL,trimslash,url"`
}
```

## pg

One package for everything Postgres-connection-shaped: the pool, migrations,
transactions, and LISTEN/NOTIFY.

```go
pool := pg.MustPool(ctx, cfg.DSN)              // pgxpool.New + Ping
err  := pg.Migrate(ctx, pool, migrationsFS)    // goose + pg_advisory_lock serialization
err  := pg.InTx(ctx, pool, func(tx pgx.Tx) error { ... })

ch, err := pg.Listen(ctx, pool, "channel")     // dedicated conn, auto-reconnect,
                                               // 1s→30s backoff, drop-on-slow-consumer
err = pg.Notify(ctx, pool, "channel", payload)
```

`Migrate` embeds the migration files in the binary, takes a `pg_advisory_lock` so
N containers can boot concurrently without racing the schema, drives goose through
`stdlib.OpenDBFromPool`, returns early when the schema is already current, and
never migrates down automatically. The advisory lock key is derived from the app's
module path so two beach apps sharing a cluster don't serialize each other.

`InTx` removes the manual Begin/WithTx/Rollback boilerplate from handlers. Apps
generate their own `internal/db` with sqlc; the framework ships the sqlc.yaml
template (pgtype purge → `time.Time`/pointers, domain-ID newtype overrides,
`emit_result_struct_pointers`, pascal JSON tags) in the skeleton, not as a runtime
dependency.

`Listen` holds a dedicated connection (never a pooled borrow), reconnects with
exponential backoff capped at 30 seconds, and drops payloads rather than block a
slow consumer — subscribers reconcile with a catch-up cursor
([04-hub.md](04-hub.md)). House rule, framework doctrine: **payloads are ids, not
data** (`{table, id, op}` JSON or a bare id). Listeners re-query. The 8KB payload
limit and commit-serialized NOTIFY at high write rates make this the only durable
contract. Triggers are `AFTER` triggers calling a `pg_notify` plpgsql function
with `json_build_object`; the skeleton stamps a worked example.
