-- boardwalk's only Postgres table: a single-row store for the periodic CBOR
-- snapshot of the live ecs.Store. The game runs in memory; this row is the
-- durable copy, overwritten in place (id is pinned to 1 by the CHECK) and
-- restored on boot.

-- +goose Up
CREATE TABLE boardwalk_snapshot (
    id         int         PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    data       bytea       NOT NULL,
    tick       bigint      NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE boardwalk_snapshot;
