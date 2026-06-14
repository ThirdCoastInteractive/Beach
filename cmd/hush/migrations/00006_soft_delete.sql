-- +goose Up
-- Purge goes soft: a purged post keeps its row, reactions, flags, and capture
-- meta as evidence (the terms promise exactly that), it just stops existing
-- anywhere but the admin browser. deleted_by records which admin ruled.
ALTER TABLE hush_posts ADD COLUMN deleted_at timestamptz;
ALTER TABLE hush_posts ADD COLUMN deleted_by text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE hush_posts DROP COLUMN deleted_by;
ALTER TABLE hush_posts DROP COLUMN deleted_at;
