-- hush's ClickHouse observation store: one append-only row per thing that
-- happened (views, posts, votes, signups, logins). Postgres stays the system
-- of record; this table only feeds the admin analytics dashboard.
-- ORDER BY (kind, ts) so the per-kind day buckets the dashboard charts read a
-- contiguous range.

-- +goose Up
CREATE TABLE hush_events (
    ts      DateTime,
    kind    LowCardinality(String),
    handle  String,
    post_id Int64,
    ip      String,
    country LowCardinality(String),
    city    String,
    ua      String,
    path    String
) ENGINE = MergeTree
ORDER BY (kind, ts);

-- +goose Down
DROP TABLE hush_events;
