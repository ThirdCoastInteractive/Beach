# driftbottle

Beach example: anonymous stranger chat. Matchmaking and the live feed are
in-process hub topics. Postgres archives transcripts. ClickHouse is the
event firehose.

```
cp .env.example .env
# from Beach repo root:
make up-driftbottle
```

http://localhost:8080/ — pair with a second browser. `/specimen` is the kit
gallery.

Both DSNs are required. A missing store aborts boot.
