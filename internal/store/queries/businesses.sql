-- name: CreateBusiness :one
INSERT INTO businesses (id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetBusinessByID :one
SELECT * FROM businesses WHERE id = $1;
