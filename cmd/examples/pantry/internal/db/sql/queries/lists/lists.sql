-- Shopping-list queries with beach-apigen annotations.

-- name: ListLists :many
-- @api GET /lists
-- @page page.ListsIndex
-- @fragment page.ShoppingLists
-- @requires pantry:read
SELECT id, name FROM pantry_lists WHERE deleted_at IS NULL ORDER BY name;

-- name: CreateList :one
-- @api POST /lists
-- @requires pantry:write
-- @notify lists
-- @fragment page.ShoppingLists
INSERT INTO pantry_lists (name) VALUES (@name) RETURNING id, name;

-- name: AddListItem :one
-- @api POST /lists/{id}/items
-- @requires pantry:write
-- @notify lists
-- @fragment page.ShoppingList
INSERT INTO pantry_list_items (list_id, name, quantity)
VALUES (@list_id, @name, @quantity)
RETURNING id, list_id, name, quantity, checked;

-- name: ToggleListItem :exec
-- @api POST /lists/items/{id}/toggle
-- @requires pantry:write
-- @notify lists
UPDATE pantry_list_items SET checked = NOT checked WHERE id = @id;
