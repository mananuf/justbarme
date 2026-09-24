-- name: ListAllBusinesses :many
SELECT * FROM businesses ORDER BY created_at DESC;

-- name: SetBusinessStatus :one
UPDATE businesses SET status = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: CountUsers :one
SELECT count(*) FROM users;
