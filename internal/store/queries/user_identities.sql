-- name: CreateUserIdentity :one
INSERT INTO user_identities (id, user_id, provider, provider_user_id, email)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserIdentity :one
SELECT * FROM user_identities WHERE provider = $1 AND provider_user_id = $2;
