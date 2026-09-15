-- name: CreateDevice :one
INSERT INTO devices (id, business_id, user_id, location_id, public_key, display_name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDeviceByID :one
SELECT * FROM devices WHERE business_id = $1 AND id = $2;

-- name: ListDevicesForBusiness :many
SELECT * FROM devices WHERE business_id = $1 ORDER BY enrolled_at DESC;

-- name: RevokeDevice :one
UPDATE devices
SET status = 'revoked', revoked_at = now(), updated_at = now()
WHERE business_id = $1 AND id = $2 AND status = 'active'
RETURNING *;
