-- name: CreateBusiness :one
INSERT INTO businesses (id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetBusinessByID :one
SELECT * FROM businesses WHERE id = $1;

-- name: UpdateBusinessBranding :one
UPDATE businesses
SET phone = $2, address = $3, receipt_wording = $4, receipt_footer = $5, payment_instructions = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateBusinessLogo :one
UPDATE businesses
SET logo_object_key = $2, updated_at = now()
WHERE id = $1
RETURNING *;
