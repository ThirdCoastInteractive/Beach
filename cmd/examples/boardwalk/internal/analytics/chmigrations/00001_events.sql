-- +goose Up
CREATE TABLE boardwalk_events (
    ts    DateTime,
    kind  LowCardinality(String),  -- join | roll | pass_go | buy | rent | tax | chance
    seat  Int32,
    token LowCardinality(String),
    tile  Int32,
    name  LowCardinality(String),
    delta Int64
) ENGINE = MergeTree
ORDER BY (kind, ts);

-- +goose Down
DROP TABLE boardwalk_events;
