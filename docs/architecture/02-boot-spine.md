# Architecture 2 — Boot spine: `config`, `pg`, `pgnotify`

[← docs index](../README.md) · prev: [Layout](01-layout.md) · next: [HTTP layer](03-http.md)

## config

Lifted from `manifold13.tv/internal/serviceconfig`. One struct per app, `env:` tags,
reflect-based loader, `A|B` fallbacks, modifier flags (`lower`, `trimslash`, `url`),
`.env` via godotenv, `MustLoad` aborts on missing required values. Feature blocks
(SMTP, Stripe, CF) stay app-side; the framework only defines the core block:

```go
type Core struct {
    AppEnv     string `env:"APP_ENV,lower"`      // "production" enables release behavior
    Port       string `env:"PORT"`
    DSN        string `env:"POSTGRES_DSN|DATABASE_URL"`
    ValkeyAddr string `env:"VALKEY_ADDR"`
    BaseURL    string `env:"BASE_URL,trimslash,url"`
}
```

## pg

```go
pool := pg.MustPool(ctx, cfg.DSN)              // pgxpool.New + Ping
err  := pg.Migrate(ctx, pool, migrationsFS)    // goose + pg_advisory_lock serialization
err  := pg.InTx(ctx, pool, func(tx pgx.Tx) error { ... })
```

`Migrate` keeps manifold's exact pattern (`internal/db/database.go`): embedded
migrations, `pg_advisory_lock` so N containers can boot concurrently, goose via
`stdlib.OpenDBFromPool`, early-out when current == max, and never auto-down. The
advisory lock key is derived from the app's module path so two beach apps sharing a
cluster don't serialize each other.

`InTx` is the new helper closing manifold's "boilerplate-heavy manual Begin/WithTx/
Rollback" wart. Apps keep generating their own `internal/db` with sqlc; the framework
ships the sqlc.yaml template (pgtype purge → `time.Time`/pointers, domain-ID newtype
overrides, `emit_result_struct_pointers`, pascal JSON tags) in the skeleton, not as a
runtime dependency.

## pgnotify

Lifted verbatim from `manifold13.tv/internal/pgnotify/pgnotify.go` — it is already
right:

```go
ch, err := pgnotify.Listen(ctx, pool, "channel")  // dedicated conn, auto-reconnect,
                                                  // 1s→30s backoff, drop-on-slow-consumer
err := pgnotify.Notify(ctx, pool, "channel", payload)
```

House rule, now framework doc: **payloads are ids, not data** (`{table, id, op}` JSON
or a bare id). Listeners re-query. 8KB payload limit and commit-serialized NOTIFY at
high write rates make this the only durable contract. Triggers follow manifold's
migration patterns (`00062_notifications.sql`, `00029_camera_config_notify.sql`):
`AFTER` triggers calling a `pg_notify` plpgsql function with `json_build_object`.
