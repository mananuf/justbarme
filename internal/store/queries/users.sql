-- name: CreateUser :one
INSERT INTO users (id, email, phone, display_name, password_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(sqlc.arg(email));

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;
