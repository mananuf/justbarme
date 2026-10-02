-- name: CreatePricePack :one
INSERT INTO product_variant_price_packs (id, business_id, variant_id, pack_quantity, pack_price_kobo, valid_from, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CloseCurrentPricePacksForVariant :exec
UPDATE product_variant_price_packs
SET valid_to = $3
WHERE business_id = $1 AND variant_id = $2 AND valid_to IS NULL;

-- name: ListCurrentPricePacksByVariant :many
SELECT * FROM product_variant_price_packs
    WHERE business_id = $1 AND variant_id = $2 AND valid_to IS NULL
    ORDER BY pack_quantity;

-- name: ListCurrentPricePacksByBusiness :many
-- Backs Service.ListCatalogue's one-query-per-business composition, the
-- same shape ListCurrentPricesByBusiness already uses for the flat prices
-- themselves.
SELECT * FROM product_variant_price_packs
    WHERE business_id = $1 AND valid_to IS NULL
    ORDER BY variant_id, pack_quantity;
