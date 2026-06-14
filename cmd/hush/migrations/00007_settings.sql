-- +goose Up
-- Live policy: the gate-and-moderation knobs an admin edits at /admin/settings
-- without a restart. One row per key; the HUSH_* env vars seed it on first
-- boot, after which this table is authoritative.
CREATE TABLE hush_settings (
    key        text        PRIMARY KEY,
    value      text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE hush_settings;
