-- name: CreatePlatformStaff :one
INSERT INTO platform_staff (id, email, display_name, password_hash, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPlatformStaffByEmail :one
SELECT * FROM platform_staff WHERE lower(email) = lower(sqlc.arg(email));

-- name: GetPlatformStaffByID :one
SELECT * FROM platform_staff WHERE id = $1;
