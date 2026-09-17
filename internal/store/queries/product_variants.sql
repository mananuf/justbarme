-- name: CreateVariant :one
INSERT INTO product_variants (id, business_id, product_id, name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetVariantByID :one
SELECT * FROM product_variants WHERE business_id = $1 AND id = $2;

-- name: GetVariantWithProductNameByID :one
-- Joins in the owning product's name for callers that need a full
-- "Product -- Variant" display snapshot (e.g. internal/sales, when
-- recording what was actually sold) -- product_variants.name alone is
-- just the variant's own name ("50cl Bottle"), never the product name.
SELECT product_variants.*, products.name AS product_name
FROM product_variants
JOIN products ON products.business_id = product_variants.business_id AND products.id = product_variants.product_id
WHERE product_variants.business_id = $1 AND product_variants.id = $2;

-- name: ListVariantsByProduct :many
SELECT * FROM product_variants WHERE business_id = $1 AND product_id = $2 ORDER BY name;

-- name: ListVariantsByBusiness :many
SELECT * FROM product_variants WHERE business_id = $1 ORDER BY product_id, name;

-- name: UpdateVariant :one
UPDATE product_variants
SET name = $3, active = $4, updated_at = now()
WHERE business_id = $1 AND id = $2
RETURNING *;
