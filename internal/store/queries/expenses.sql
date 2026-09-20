-- name: GetExpenseCategoryByName :one
SELECT * FROM expense_categories WHERE business_id = $1 AND name = $2;

-- name: CreateExpenseCategory :one
INSERT INTO expense_categories (id, business_id, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListExpenseCategories :many
SELECT * FROM expense_categories WHERE business_id = $1 AND active ORDER BY name;

-- name: CreateExpense :one
INSERT INTO expenses
    (id, business_id, location_id, category_id, description, amount_kobo, payment_method, recorded_by, idempotency_key, occurred_at, reversal_of_expense_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetExpenseByIdempotencyKey :one
SELECT * FROM expenses WHERE business_id = $1 AND idempotency_key = $2;

-- name: GetExpenseByID :one
SELECT * FROM expenses WHERE business_id = $1 AND id = $2;

-- name: GetReversalOfExpense :one
SELECT * FROM expenses WHERE business_id = $1 AND reversal_of_expense_id = $2;

-- name: ListExpensesDetailed :many
SELECT
    e.id, e.location_id, e.category_id, e.description, e.amount_kobo, e.payment_method,
    e.recorded_by, e.reversal_of_expense_id, e.occurred_at, e.created_at,
    c.name AS category_name
FROM expenses e
JOIN expense_categories c ON c.business_id = e.business_id AND c.id = e.category_id
WHERE e.business_id = $1
ORDER BY e.occurred_at DESC
LIMIT $2;

-- name: SumExpensesTotalSince :one
-- Mirrors SumSalesTotalSince exactly: total nets every row including
-- reversals (a reversal's amount_kobo is negative by construction, same
-- sign convention as sales.total_kobo); count excludes reversal rows.
SELECT
    COALESCE(SUM(amount_kobo), 0)::bigint AS total,
    COUNT(*) FILTER (WHERE reversal_of_expense_id IS NULL)::bigint AS count
FROM expenses WHERE business_id = $1 AND occurred_at >= $2;
