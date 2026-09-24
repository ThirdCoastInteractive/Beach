-- driftbottle's ClickHouse firehose: one append-only row per thing that
-- happened on the wall (a session minted, a stranger queued, a pair formed, a
-- message sent, a pair torn down). It is written OFF the hot path (a fire-and-
-- forget batcher) and only ever read by the public /stats aggregates. Postgres
-- holds the transcript archive; this table is analytics only.
-- ORDER BY (kind, ts) so the per-kind day buckets the /stats charts read a
-- contiguous range.

-- +goose Up
CREATE TABLE driftbottle_events (
    ts   DateTime,
    kind LowCardinality(String), -- session | queued | paired | message | unpaired
    sid  String,
    pair String,
    len  UInt32
) ENGINE = MergeTree
ORDER BY (kind, ts);

-- +goose Down
DROP TABLE driftbottle_events;
