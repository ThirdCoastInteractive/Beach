-- name: GetItem :one
-- @api GET /items/{id}
-- @page page.ItemDetail
-- @fragment page.ItemCard
SELECT id, name, quantity, location_id, expires_at
FROM items WHERE id = @id AND deleted_at IS NULL;

-- name: CreateItem :one
-- @api POST /items
-- @requires pantry:write
-- @notify items
-- @fragment page.ItemCard
INSERT INTO items (name, quantity, location_id)
VALUES (@name, @quantity, @location_id)
RETURNING id, name, quantity, location_id;

-- name: ListItems :many
-- this query is plain sqlc with no @api — apigen ignores it
SELECT id, name FROM items WHERE deleted_at IS NULL ORDER BY name;

-- name: DeleteItem :exec
-- @api DELETE /items/{id}
-- @fragment page.ItemList
DELETE FROM items WHERE id = @id;
