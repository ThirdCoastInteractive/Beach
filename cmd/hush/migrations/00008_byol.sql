-- +goose Up
-- BYOL: a post can carry a multi-image layout (the studio composition), and
-- every post can be pinned on the shared wall — position, rotation, and width
-- as fractions of the board. NULL pins mean "scatter me from my id" so existing
-- rows keep working without a backfill.

ALTER TABLE hush_posts
    ADD COLUMN layout  jsonb           NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN pin_x   double precision,
    ADD COLUMN pin_y   double precision,
    ADD COLUMN pin_rot double precision,
    ADD COLUMN pin_w   double precision;

-- +goose Down
ALTER TABLE hush_posts
    DROP COLUMN layout,
    DROP COLUMN pin_x,
    DROP COLUMN pin_y,
    DROP COLUMN pin_rot,
    DROP COLUMN pin_w;
