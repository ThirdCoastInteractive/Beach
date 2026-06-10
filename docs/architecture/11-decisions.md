# Architecture 11 — Decisions log

[← docs index](../README.md) · prev: [Tooling & testing](10-tooling.md)

| Decision           | Choice                                          | Why                                                                                                                                                |
| ------------------ | ----------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| HTTP framework     | Echo v4 underneath, typed handler shapes on top | Echo proven across five manifold services; `PageFunc`/`ActionFunc`/`StreamFunc` make the house rules structural; `app.Raw` is the escape hatch    |
| ECS engine         | Custom standalone `ecs` package                 | Language-neutral core (schema, entity handle, CBOR snapshots) reusable beyond Go/the-beach; change ticks first-class; Ark stays the perf yardstick |
| World concurrency  | Single writer goroutine + command channel       | No locks, deterministic, testable                                                                                                                  |
| System of record   | Postgres, component-mirror tables               | Legible SQL, sqlc-friendly, no world blob                                                                                                          |
| Cross-process bus  | LISTEN/NOTIFY, ids only                         | Single-node honest; NATS seam identified at hub/pgnotify                                                                                           |
| SSE model          | Topics (hub), render-once-per-topic             | Unifies manifold's two models; interest management built in                                                                                        |
| SSE compression    | Gzip in the SSE writer, sync-flush per event    | Middleware buffering was the only real blocker; fragment streams compress 80%+; not `deflate` (raw-vs-zlib ambiguity); per-conn memory is the measured cost |
| Sessions           | Valkey                                          | Lifted, production-proven, RevokeAll cheap                                                                                                         |
| Scaffold           | `beach new` stamps files into the app           | Apps own their Makefile/sqlc.yaml; no runtime magic, no hidden codegen                                                                             |
| manifold migration | Out of scope                                    | Framework proves itself greenfield first                                                                                                           |
