-- name: CreatePlatformStaff :one
INSERT INTO platform_staff (id, email, display_name, password_hash, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPlatformStaffByEmail :one
SELECT * FROM platform_staff WHERE lower(email) = lower(sqlc.arg(email));

-- name: GetPlatformStaffByID :one
SELECT * FROM platform_staff WHERE id = $1;

-- name: ListPlatformStaff :many
SELECT * FROM platform_staff ORDER BY created_at;

-- name: SetPlatformStaffStatus :one
UPDATE platform_staff SET status = $2, updated_at = now() WHERE id = $1 RETURNING *;
