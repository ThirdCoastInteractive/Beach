-- +goose Up
-- Wall print stacking order. NULL means "use the post id" so existing
-- rows keep their current newest-on-top scatter.
ALTER TABLE hush_posts ADD COLUMN pin_z integer;

-- +goose Down
ALTER TABLE hush_posts DROP COLUMN pin_z;
