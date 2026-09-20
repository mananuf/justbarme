-- name: SumSalesByDay :many
-- Backs the Sales report's daily-revenue heatmap (docs/PHASE_EXPENSES_
-- DASHBOARD_ACTIVITY_REPORTS.md §5). Bucketed in the business's own
-- timezone (AT TIME ZONE), not UTC or the caller's -- same reasoning as
-- salesSummary's "today" boundary.
SELECT
    date_trunc('day', occurred_at AT TIME ZONE $2::text)::date AS day,
    COALESCE(SUM(total_kobo), 0)::bigint AS total_kobo,
    COUNT(*) FILTER (WHERE reversal_of_sale_id IS NULL)::bigint AS sale_count
FROM sales
WHERE business_id = $1
  AND occurred_at >= $3 AND occurred_at < $4
GROUP BY day
ORDER BY day;

-- name: ListProductQuantitiesSold :many
-- "Units sold" excludes a reversal's compensating negative-quantity lines
-- -- same reasoning SumSalesTotalSince already applies to sale counts.
SELECT
    si.variant_id, v.name AS variant_name, p.name AS product_name,
    COALESCE(SUM(si.quantity) FILTER (WHERE si.quantity > 0), 0)::bigint AS units_sold
FROM sale_items si
JOIN sales s ON s.business_id = si.business_id AND s.id = si.sale_id
JOIN product_variants v ON v.business_id = si.business_id AND v.id = si.variant_id
JOIN products p ON p.business_id = v.business_id AND p.id = v.product_id
WHERE si.business_id = $1
  AND s.occurred_at >= $2 AND s.occurred_at < $3
GROUP BY si.variant_id, v.name, p.name
ORDER BY units_sold DESC;

-- name: ListStaffSales :many
SELECT
    seller_id,
    COALESCE(SUM(total_kobo), 0)::bigint AS total_kobo,
    COUNT(*) FILTER (WHERE reversal_of_sale_id IS NULL)::bigint AS sale_count
FROM sales
WHERE business_id = $1
  AND occurred_at >= $2 AND occurred_at < $3
GROUP BY seller_id
ORDER BY total_kobo DESC;

-- name: SumExpensesByCategory :many
SELECT
    e.category_id, c.name AS category_name,
    COALESCE(SUM(e.amount_kobo), 0)::bigint AS total_kobo,
    COUNT(*) FILTER (WHERE e.reversal_of_expense_id IS NULL)::bigint AS expense_count
FROM expenses e
JOIN expense_categories c ON c.business_id = e.business_id AND c.id = e.category_id
WHERE e.business_id = $1
  AND e.occurred_at >= $2 AND e.occurred_at < $3
GROUP BY e.category_id, c.name
ORDER BY total_kobo DESC;

-- name: ListStockDiscrepancies :many
-- Reuses Phase 7's own data directly (docs/PHASE_EXPENSES_DASHBOARD_
-- ACTIVITY_REPORTS.md §3) -- no new schema. A count line's date is its
-- parent count's started_at, since a line itself carries no timestamp.
SELECT
    cl.id, cl.count_id, cl.variant_id, cl.expected_quantity, cl.physical_quantity, cl.variance, cl.is_stale,
    sc.started_at, sc.counted_by,
    v.name AS variant_name, p.name AS product_name
FROM stock_count_lines cl
JOIN stock_counts sc ON sc.business_id = cl.business_id AND sc.id = cl.count_id
JOIN product_variants v ON v.business_id = cl.business_id AND v.id = cl.variant_id
JOIN products p ON p.business_id = v.business_id AND p.id = v.product_id
WHERE cl.business_id = $1 AND cl.variance != 0
  AND sc.started_at >= $2 AND sc.started_at < $3
ORDER BY sc.started_at DESC;
