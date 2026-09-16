-- name: CreateCategory :one
INSERT INTO categories (id, business_id, name, sort_order)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetCategoryByID :one
SELECT * FROM categories WHERE business_id = $1 AND id = $2;

-- name: GetCategoryByName :one
SELECT * FROM categories WHERE business_id = $1 AND name = $2;

-- name: ListCategories :many
SELECT * FROM categories WHERE business_id = $1 ORDER BY sort_order, name;

-- name: UpdateCategory :one
UPDATE categories
SET name = $3, sort_order = $4, active = $5, updated_at = now()
WHERE business_id = $1 AND id = $2
RETURNING *;
