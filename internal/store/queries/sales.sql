-- name: CreateBill :one
INSERT INTO bills (id, business_id, location_id, status, opened_by, opened_at, table_id, customer_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetBillByID :one
SELECT * FROM bills WHERE business_id = $1 AND id = $2;

-- name: CreateSale :one
INSERT INTO sales (id, business_id, bill_id, seller_id, idempotency_key, occurred_at, received_at, total_kobo, reversal_of_sale_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetSaleByIdempotencyKey :one
SELECT * FROM sales WHERE business_id = $1 AND idempotency_key = $2;

-- name: GetSaleByID :one
SELECT * FROM sales WHERE business_id = $1 AND id = $2;

-- name: GetReversalOfSale :one
SELECT * FROM sales WHERE business_id = $1 AND reversal_of_sale_id = $2;

-- name: ListSales :many
SELECT * FROM sales WHERE business_id = $1 ORDER BY occurred_at DESC LIMIT $2 OFFSET $3;

-- name: SumSalesTotalSince :one
-- total nets every row including reversals (a reversal's total_kobo is
-- negative by construction -- see migration 000019); count deliberately
-- excludes reversal rows, since a correction isn't "one more sale" for
-- display purposes.
SELECT
    COALESCE(SUM(total_kobo), 0)::bigint AS total,
    COUNT(*) FILTER (WHERE reversal_of_sale_id IS NULL)::bigint AS count
    FROM sales WHERE business_id = $1 AND occurred_at >= $2;

-- name: CreateSaleItem :one
INSERT INTO sale_items (id, business_id, sale_id, variant_id, description, quantity, unit_price_kobo, line_total_kobo)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListSaleItemsBySaleID :many
SELECT * FROM sale_items WHERE business_id = $1 AND sale_id = $2;

-- name: CreatePayment :one
INSERT INTO payments (id, business_id, bill_id, amount_kobo, method, actor_id, reversal_of_payment_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPaymentsByBillID :many
SELECT * FROM payments WHERE business_id = $1 AND bill_id = $2;

-- name: UpdateBillStatus :one
UPDATE bills SET status = $3, updated_at = now() WHERE business_id = $1 AND id = $2
RETURNING *;

-- name: CreateSaleReview :one
INSERT INTO sale_reviews (id, business_id, sale_id, sale_item_id, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListSaleReviews :many
SELECT * FROM sale_reviews WHERE business_id = $1 AND status = $2 ORDER BY created_at DESC;

-- name: ResolveSaleReview :one
UPDATE sale_reviews
    SET status = 'resolved', resolved_by = $3, resolved_note = $4, resolved_at = now()
    WHERE business_id = $1 AND id = $2 AND status = 'open'
RETURNING *;

-- name: CreateInventoryEventForSale :one
INSERT INTO inventory_events (id, business_id, type, actor_id, sale_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
