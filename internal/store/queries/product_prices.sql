-- name: CreatePrice :one
INSERT INTO product_prices (id, business_id, variant_id, amount_kobo, valid_from, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CloseCurrentPrice :exec
UPDATE product_prices
SET valid_to = $3
WHERE business_id = $1 AND variant_id = $2 AND valid_to IS NULL;

-- name: GetCurrentPrice :one
SELECT * FROM product_prices WHERE business_id = $1 AND variant_id = $2 AND valid_to IS NULL;

-- name: ListCurrentPricesByBusiness :many
SELECT * FROM product_prices WHERE business_id = $1 AND valid_to IS NULL;

-- name: ListPriceHistory :many
SELECT * FROM product_prices WHERE business_id = $1 AND variant_id = $2 ORDER BY valid_from DESC;

-- name: GetPriceAt :one
SELECT * FROM product_prices
    WHERE business_id = $1 AND variant_id = $2 AND valid_from <= $3 AND (valid_to IS NULL OR valid_to > $3)
    LIMIT 1;
