-- Inventory item queries, annotated with the beach-apigen grammar
-- (docs/architecture/13-apigen.md). Run through `sqlc generate` with the
-- beach-apigen plugin, these annotations generate the CRUD route surface: a
-- PageFunc per @page query and an ActionFunc per mutating @api query, including
-- the @requires principal check and the @fragment patch.
--
-- The handlers in this example are ALSO hand-written (pantry runs without sqlc
-- wired into go.mod) so the binary builds and serves; the annotations document
-- the generated shape the apigen showcase would emit. Generated vs hand-written
-- is called out in the build report.

-- name: ListItems :many
-- @api GET /items
-- @page page.ItemsIndex
-- @fragment page.ItemGrid
-- @requires pantry:read
SELECT i.id, i.name, i.quantity, i.unit, i.category, i.photo_url,
       i.expires_at, i.location_id, l.name AS location_name
  FROM pantry_items i
  LEFT JOIN pantry_locations l ON l.id = i.location_id
 WHERE i.deleted_at IS NULL
 ORDER BY i.expires_at NULLS LAST, i.name;

-- name: GetItem :one
-- @api GET /items/{id}
-- @page page.ItemDetail
-- @fragment page.ItemCard
-- @requires pantry:read
SELECT i.id, i.name, i.quantity, i.unit, i.category, i.photo_url,
       i.expires_at, i.location_id, l.name AS location_name
  FROM pantry_items i
  LEFT JOIN pantry_locations l ON l.id = i.location_id
 WHERE i.id = @id AND i.deleted_at IS NULL;

-- name: CreateItem :one
-- @api POST /items
-- @requires pantry:write
-- @notify items
-- @fragment page.ItemCard
INSERT INTO pantry_items (name, quantity, unit, location_id, category, photo_url, expires_at)
VALUES (@name, @quantity, @unit, @location_id, @category, @photo_url, @expires_at)
RETURNING id, name, quantity, unit, category, photo_url, expires_at, location_id;

-- name: UpdateItem :one
-- @api PUT /items/{id}
-- @requires pantry:write
-- @notify items
-- @fragment page.ItemCard
UPDATE pantry_items
   SET name = @name, quantity = @quantity, unit = @unit,
       location_id = @location_id, category = @category,
       photo_url = @photo_url, expires_at = @expires_at, updated_at = now()
 WHERE id = @id AND deleted_at IS NULL
RETURNING id, name, quantity, unit, category, photo_url, expires_at, location_id;

-- name: DeleteItem :exec
-- @api DELETE /items/{id}
-- @requires pantry:write
-- @notify items
UPDATE pantry_items SET deleted_at = now() WHERE id = @id;
