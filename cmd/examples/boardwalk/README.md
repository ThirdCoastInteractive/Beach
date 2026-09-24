# boardwalk

Beach example: `sim` / `ecs` realtime board game. Live slice is an in-memory
store on a single-writer sim loop. Postgres holds a CBOR snapshot restored on
boot. ClickHouse is the action firehose behind `/stats`.

```
cp .env.example .env
# from Beach repo root:
make up-boardwalk
```

http://localhost:8080/ — spectator board, unauthenticated. Take a seat, roll.
`/specimen` is the kit gallery. `/stats` is the dashboard.

Both DSNs are required. A missing store aborts boot.
