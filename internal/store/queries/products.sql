-- name: CreateProduct :one
INSERT INTO products (id, business_id, category_id, name, template_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetProductByID :one
SELECT * FROM products WHERE business_id = $1 AND id = $2;

-- name: ListProducts :many
SELECT * FROM products WHERE business_id = $1 ORDER BY name;

-- name: UpdateProduct :one
UPDATE products
SET name = $3, category_id = $4, active = $5, updated_at = now()
WHERE business_id = $1 AND id = $2
RETURNING *;
