-- Storage-location queries with beach-apigen annotations.

-- name: ListLocations :many
-- @api GET /locations
-- @page page.LocationsIndex
-- @fragment page.LocationList
-- @requires pantry:read
SELECT id, name, kind
  FROM pantry_locations
 WHERE deleted_at IS NULL
 ORDER BY name;

-- name: CreateLocation :one
-- @api POST /locations
-- @requires pantry:write
-- @notify locations
-- @fragment page.LocationList
INSERT INTO pantry_locations (name, kind)
VALUES (@name, @kind)
RETURNING id, name, kind;

-- name: DeleteLocation :exec
-- @api DELETE /locations/{id}
-- @requires pantry:admin
-- @notify locations
UPDATE pantry_locations SET deleted_at = now() WHERE id = @id;
