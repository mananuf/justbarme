-- name: CreateTable :one
INSERT INTO tables (id, business_id, label)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTableByID :one
SELECT * FROM tables WHERE business_id = $1 AND id = $2;

-- name: ListTables :many
SELECT * FROM tables WHERE business_id = $1 AND active = true ORDER BY label;

-- name: CreateCustomer :one
INSERT INTO customers (id, business_id, name, phone, email, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCustomerByID :one
SELECT * FROM customers WHERE business_id = $1 AND id = $2;

-- name: ListCustomers :many
SELECT * FROM customers WHERE business_id = $1 ORDER BY name;

-- name: GetBillForUpdate :one
SELECT * FROM bills WHERE business_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateBillBalanceAndStatus :one
UPDATE bills SET balance_kobo = $3, status = $4, updated_at = now()
    WHERE business_id = $1 AND id = $2
RETURNING *;

-- name: ListOpenBills :many
SELECT * FROM bills WHERE business_id = $1 AND status IN ('open', 'closed_unpaid') ORDER BY opened_at;

-- name: ListOutstandingBills :many
SELECT * FROM bills WHERE business_id = $1 AND balance_kobo <> 0 ORDER BY opened_at;

-- name: CreateBillWriteOff :one
INSERT INTO bill_write_offs (id, business_id, bill_id, amount_kobo, reason, actor_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListWriteOffsByBillID :many
SELECT * FROM bill_write_offs WHERE business_id = $1 AND bill_id = $2 ORDER BY created_at;

-- name: GetPaymentByID :one
SELECT * FROM payments WHERE business_id = $1 AND id = $2;

-- name: GetReversalOfPayment :one
SELECT * FROM payments WHERE business_id = $1 AND reversal_of_payment_id = $2;

-- name: ListSalesByBillID :many
SELECT * FROM sales WHERE business_id = $1 AND bill_id = $2 ORDER BY occurred_at;

-- name: SumBillItemQuantityByVariant :one
-- Net quantity of one variant currently on a bill, across every round
-- posted so far (a removal round's negative quantity nets against the
-- rounds that added it) -- the cap RemoveBillItem enforces so you can
-- never remove more than is actually there.
SELECT COALESCE(SUM(sale_items.quantity), 0)::int AS quantity
FROM sale_items
JOIN sales ON sales.business_id = sale_items.business_id AND sales.id = sale_items.sale_id
WHERE sale_items.business_id = $1 AND sales.bill_id = $2 AND sale_items.variant_id = $3;

-- name: SumSalesTotalByBillID :one
SELECT COALESCE(SUM(total_kobo), 0)::bigint AS total FROM sales WHERE business_id = $1 AND bill_id = $2;

-- name: SumPaymentsByBillID :one
SELECT COALESCE(SUM(amount_kobo), 0)::bigint AS total FROM payments WHERE business_id = $1 AND bill_id = $2;

-- name: SumWriteOffsByBillID :one
-- Together with SumSalesTotalByBillID and SumPaymentsByBillID (sales minus
-- payments minus write-offs), this is the rebuildable-projection source of
-- truth persisted into bills.balance_kobo by internal/sales.Service.
SELECT COALESCE(SUM(amount_kobo), 0)::bigint AS total FROM bill_write_offs WHERE business_id = $1 AND bill_id = $2;
