-- name: CreateUser :one
INSERT INTO users (id, email, phone, display_name, password_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg(email));

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: SetUserPhone :one
UPDATE users SET phone = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetUserEmail :one
UPDATE users SET email = $2, updated_at = now() WHERE id = $1 RETURNING *;
