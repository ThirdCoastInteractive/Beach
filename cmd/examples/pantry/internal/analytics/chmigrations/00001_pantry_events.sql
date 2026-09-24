-- +goose Up
CREATE TABLE pantry_events (
    ts        DateTime,
    kind      LowCardinality(String),  -- item_added | location_added | item_viewed
    item_name String,
    category  LowCardinality(String),
    quantity  Float64,
    path      String
) ENGINE = MergeTree
ORDER BY (kind, ts);

-- +goose Down
DROP TABLE pantry_events;
