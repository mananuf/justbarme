-- name: CreateStockReceipt :one
INSERT INTO stock_receipts (id, business_id, location_id, received_by, received_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateStockReceiptLine :one
INSERT INTO stock_receipt_lines (id, business_id, receipt_id, variant_id, quantity, total_cost_kobo)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreateStockLot :one
INSERT INTO stock_lots (id, business_id, receipt_line_id, variant_id, location_id, received_quantity, remaining_quantity, total_cost_kobo, received_at, source)
VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, $9)
RETURNING *;

-- name: ListLotsForAllocation :many
-- FIFO order (docs/PHASE_FIFO_COSTING.md §3): oldest lot first, then lot
-- ID (UUIDv7, itself time-ordered) as a stable tiebreaker for lots
-- received in the same instant. FOR UPDATE is what makes concurrent
-- sales against the same lot actually serialize -- the second
-- transaction blocks until the first commits, then sees the already-
-- decremented remaining_quantity, same locking pattern
-- GetBillForUpdate already established for bill balances.
SELECT * FROM stock_lots
WHERE business_id = $1 AND variant_id = $2 AND location_id = $3 AND remaining_quantity > 0
ORDER BY received_at ASC, id ASC
FOR UPDATE;

-- name: DecrementLotRemainingQuantity :one
UPDATE stock_lots
SET remaining_quantity = remaining_quantity - $3
WHERE business_id = $1 AND id = $2
RETURNING *;

-- name: IncrementLotRemainingQuantity :one
UPDATE stock_lots
SET remaining_quantity = remaining_quantity + $3
WHERE business_id = $1 AND id = $2
RETURNING *;

-- name: CreateSaleItemLotAllocation :one
INSERT INTO sale_item_lot_allocations (id, business_id, sale_item_id, stock_lot_id, quantity, allocated_cost_kobo)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAllocationsBySaleItem :many
SELECT * FROM sale_item_lot_allocations
WHERE business_id = $1 AND sale_item_id = $2
ORDER BY created_at ASC;

-- name: SumNetPendingQuantityForSaleItem :one
-- Nets every pending (stock_lot_id IS NULL) allocation for this sale item
-- -- normally just one row, but a resolution's own negative close-out row
-- (docs/PHASE_FIFO_COSTING.md §5) is also stock_lot_id IS NULL, so this
-- must sum rather than assume a single row.
SELECT COALESCE(SUM(quantity), 0)::bigint AS net_pending_quantity
FROM sale_item_lot_allocations
WHERE business_id = $1 AND sale_item_id = $2 AND stock_lot_id IS NULL;

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

-- name: GetInventoryBalance :one
-- Used to compare a stock count's submitted expected_quantity against the
-- live truth at processing time (docs/PHASE_INVENTORY_COUNTS_AND_
-- ADJUSTMENTS.md: staleness is a value comparison, not a timestamp one).
-- COALESCE to 0 -- a variant with no movements yet simply has no row, not
-- an error (same convention internal/inventory.Service.GetBalances uses).
SELECT COALESCE(
    (SELECT quantity FROM inventory_balances WHERE business_id = $1 AND variant_id = $2 AND location_id = $3),
    0
)::int AS quantity;

-- name: CreateInventoryEventForAdjustment :one
INSERT INTO inventory_events (id, business_id, type, actor_id, adjustment_request_id)
VALUES ($1, $2, 'adjustment', $3, $4)
RETURNING *;

-- name: CreateStockCount :one
INSERT INTO stock_counts (id, business_id, location_id, counted_by, idempotency_key, started_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetStockCountByIdempotencyKey :one
SELECT * FROM stock_counts WHERE business_id = $1 AND idempotency_key = $2;

-- name: CreateStockCountLine :one
INSERT INTO stock_count_lines (id, business_id, count_id, variant_id, expected_quantity, physical_quantity, variance, is_stale)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListStockCountLines :many
SELECT * FROM stock_count_lines WHERE business_id = $1 AND count_id = $2;

-- name: CreateInventoryAdjustmentRequest :one
INSERT INTO inventory_adjustment_requests
    (id, business_id, location_id, variant_id, requested_by, idempotency_key, quantity_delta, reason_category, reason_note, source_count_line_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetInventoryAdjustmentRequestByIdempotencyKey :one
SELECT * FROM inventory_adjustment_requests WHERE business_id = $1 AND idempotency_key = $2;

-- name: ListPendingInventoryAdjustmentRequestsDetailed :many
SELECT
    r.id, r.location_id, r.variant_id, r.requested_by, r.quantity_delta,
    r.reason_category, r.reason_note, r.source_count_line_id, r.status, r.created_at,
    v.name AS variant_name, p.name AS product_name
FROM inventory_adjustment_requests r
JOIN product_variants v ON v.business_id = r.business_id AND v.id = r.variant_id
JOIN products p ON p.business_id = v.business_id AND p.id = v.product_id
WHERE r.business_id = $1 AND r.status = 'pending'
ORDER BY r.created_at DESC;

-- name: ApproveInventoryAdjustmentRequest :one
UPDATE inventory_adjustment_requests
    SET status = 'approved', decided_by = $3, decided_at = now(), resolution_note = $4
    WHERE business_id = $1 AND id = $2 AND status = 'pending'
RETURNING *;

-- name: RejectInventoryAdjustmentRequest :one
UPDATE inventory_adjustment_requests
    SET status = 'rejected', decided_by = $3, decided_at = now(), resolution_note = $4
    WHERE business_id = $1 AND id = $2 AND status = 'pending'
RETURNING *;

-- name: CreateInventoryReview :one
-- sale_item_id is only set when this negative_inventory review was opened
-- by a sale's oversell (internal/sales.postSaleRound) -- NULL for one
-- opened by an approved inventory adjustment instead, which has no sale
-- and therefore no pending cost allocation to ever resolve. See
-- docs/PHASE_FIFO_COSTING.md §5.
INSERT INTO inventory_reviews (id, business_id, type, variant_id, location_id, related_movement_id, related_count_line_id, sale_item_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListOpenInventoryReviewsDetailed :many
SELECT
    r.id, r.type, r.variant_id, r.location_id, r.related_movement_id, r.related_count_line_id,
    r.status, r.created_at,
    v.name AS variant_name, p.name AS product_name,
    cl.expected_quantity AS count_expected_quantity, cl.physical_quantity AS count_physical_quantity
FROM inventory_reviews r
JOIN product_variants v ON v.business_id = r.business_id AND v.id = r.variant_id
JOIN products p ON p.business_id = v.business_id AND p.id = v.product_id
LEFT JOIN stock_count_lines cl ON cl.business_id = r.business_id AND cl.id = r.related_count_line_id
WHERE r.business_id = $1 AND r.status = 'open'
ORDER BY r.created_at DESC;

-- name: ResolveInventoryReview :one
UPDATE inventory_reviews
    SET status = 'resolved', resolved_by = $3, resolved_note = $4, resolved_at = now()
    WHERE business_id = $1 AND id = $2 AND status = 'open'
RETURNING *;

-- name: ListInventoryMovementsDetailed :many
-- Stock history (docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md §3): a
-- flat, chronological, human-readable feed for one variant -- joins in
-- the event type and, for adjustments, the reason that authorized it.
SELECT
    m.id, m.quantity_delta, m.created_at,
    e.type AS event_type, e.actor_id,
    ar.reason_category AS adjustment_reason_category, ar.reason_note AS adjustment_reason_note,
    ar.decided_by AS adjustment_decided_by
FROM inventory_movements m
JOIN inventory_events e ON e.business_id = m.business_id AND e.id = m.event_id
LEFT JOIN inventory_adjustment_requests ar ON ar.business_id = m.business_id AND ar.id = e.adjustment_request_id
WHERE m.business_id = $1 AND m.variant_id = $2
ORDER BY m.created_at DESC
LIMIT $3;
