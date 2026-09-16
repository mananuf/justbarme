-- name: CreateStockReceipt :one
INSERT INTO stock_receipts (id, business_id, location_id, received_by, received_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateStockReceiptLine :one
INSERT INTO stock_receipt_lines (id, business_id, receipt_id, variant_id, quantity, total_cost_kobo)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreateStockLot :one
INSERT INTO stock_lots (id, business_id, receipt_line_id, variant_id, location_id, received_quantity, total_cost_kobo, received_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateInventoryEvent :one
INSERT INTO inventory_events (id, business_id, type, actor_id, receipt_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateInventoryMovement :one
INSERT INTO inventory_movements (id, business_id, event_id, variant_id, location_id, quantity_delta)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpsertInventoryBalanceDelta :one
INSERT INTO inventory_balances (business_id, variant_id, location_id, quantity, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (business_id, variant_id, location_id) DO UPDATE
    SET quantity = inventory_balances.quantity + EXCLUDED.quantity, updated_at = now()
RETURNING *;

-- name: ListInventoryBalances :many
SELECT * FROM inventory_balances WHERE business_id = $1;
