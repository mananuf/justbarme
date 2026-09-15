-- name: CreateDefaultLocation :one
INSERT INTO locations (id, business_id, name, is_default)
VALUES ($1, $2, 'Main', true)
RETURNING *;

-- name: GetLocationByID :one
SELECT * FROM locations WHERE business_id = $1 AND id = $2;

-- name: GetDefaultLocation :one
SELECT * FROM locations WHERE business_id = $1 AND is_default = true;
