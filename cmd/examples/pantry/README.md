# pantry

Beach example: grocery ERP. Annotated sqlc queries are the apigen showcase
(handlers here are also hand-written so the binary builds without the plugin
wired). Auth is argon2id with two roles. ClickHouse feeds deferred `chart`
widgets.

```
cp .env.example .env
# from Beach repo root:
make up-pantry
```

http://localhost:8080/ — seed `admin` / `password`. `/specimen` is the kit
gallery.

Postgres and ClickHouse are required. A missing store aborts boot.
