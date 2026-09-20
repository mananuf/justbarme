-- name: ListActivity :many
-- Unified activity feed (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md
-- §3): a query-time UNION ALL across every domain's own already-posted
-- records, not a new activity_events writer table -- deliberately, same
-- reasoning that kept this codebase off the generic review_cases table
-- and the generic Phase 4 sync protocol. amount_kobo is framed as cash
-- impact (positive = money in, negative = money out) rather than each
-- table's own storage sign convention -- a sale's total_kobo is already
-- money-in-positive, but an expense's amount_kobo (positive = money out
-- by storage convention, negative for its reversal) is flipped here so
-- every row in this feed means the same thing. Every branch aliases its
-- table and qualifies every column (even business_id) explicitly --
-- sqlc's static analyzer otherwise reports a false-positive "business_id
-- is ambiguous" on a UNION ALL of several tables that all happen to carry
-- that column, even though each branch's own FROM is unambiguous under
-- real Postgres.
WITH feed AS (
    SELECT
        s.id, 'sale' AS type, s.seller_id AS actor_id, s.occurred_at,
        CASE WHEN s.reversal_of_sale_id IS NOT NULL THEN 'Sale reversed' ELSE 'Sale recorded' END AS summary,
        s.total_kobo AS amount_kobo
    FROM sales s WHERE s.business_id = $1
    UNION ALL
    SELECT
        e.id, 'expense' AS type, e.recorded_by AS actor_id, e.occurred_at,
        CASE WHEN e.reversal_of_expense_id IS NOT NULL THEN 'Expense reversed' ELSE e.description END AS summary,
        -e.amount_kobo AS amount_kobo
    FROM expenses e WHERE e.business_id = $2
    UNION ALL
    SELECT
        r.id, 'inventory_adjustment' AS type, r.requested_by AS actor_id, r.created_at AS occurred_at,
        r.reason_category || ': ' || r.reason_note AS summary,
        0::bigint AS amount_kobo
    FROM inventory_adjustment_requests r
    WHERE r.business_id = $3 AND r.status = 'approved'
    UNION ALL
    SELECT
        sr.id, 'stock_receipt' AS type, sr.received_by AS actor_id, sr.received_at AS occurred_at,
        'Stock received' AS summary,
        -COALESCE((SELECT SUM(l.total_cost_kobo) FROM stock_receipt_lines l WHERE l.receipt_id = sr.id), 0)::bigint AS amount_kobo
    FROM stock_receipts sr WHERE sr.business_id = $4
)
SELECT * FROM feed
WHERE (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('activity_type')::text IS NULL OR type = sqlc.narg('activity_type'))
  AND (sqlc.narg('start_at')::timestamptz IS NULL OR occurred_at >= sqlc.narg('start_at'))
  AND (sqlc.narg('end_at')::timestamptz IS NULL OR occurred_at < sqlc.narg('end_at'))
ORDER BY occurred_at DESC
LIMIT sqlc.arg(row_limit);

-- The Activity page's GitHub-style daily heatmap needs a per-day count
-- across all four activity sources combined. A single UNION-inside-a-CTE
-- query feeding a GROUP BY/date_trunc aggregate reliably tripped sqlc's
-- static analyzer (a real limitation of the tool, not a Postgres
-- correctness issue -- the same UNION shape runs fine as a plain, non-
-- aggregated SELECT in ListActivity above). Four separate, single-table
-- day-count queries, merged into one day->count map in Go
-- (internal/activity), sidestep it entirely and are individually simpler
-- to read besides.

-- name: CountSalesByDay :many
SELECT date_trunc('day', occurred_at AT TIME ZONE $2::text)::date AS day, COUNT(*)::bigint AS n
FROM sales WHERE business_id = $1 AND occurred_at >= $3 AND occurred_at < $4
GROUP BY day;

-- name: CountExpensesByDay :many
SELECT date_trunc('day', occurred_at AT TIME ZONE $2::text)::date AS day, COUNT(*)::bigint AS n
FROM expenses WHERE business_id = $1 AND occurred_at >= $3 AND occurred_at < $4
GROUP BY day;

-- name: CountApprovedAdjustmentsByDay :many
SELECT date_trunc('day', created_at AT TIME ZONE $2::text)::date AS day, COUNT(*)::bigint AS n
FROM inventory_adjustment_requests
WHERE business_id = $1 AND status = 'approved' AND created_at >= $3 AND created_at < $4
GROUP BY day;

-- name: CountStockReceiptsByDay :many
SELECT date_trunc('day', received_at AT TIME ZONE $2::text)::date AS day, COUNT(*)::bigint AS n
FROM stock_receipts WHERE business_id = $1 AND received_at >= $3 AND received_at < $4
GROUP BY day;
