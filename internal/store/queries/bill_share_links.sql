-- name: UpsertBillShareLink :one
INSERT INTO bill_share_links (id, business_id, bill_id, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (bill_id) DO UPDATE
    SET token_hash = EXCLUDED.token_hash,
        created_by = EXCLUDED.created_by,
        created_at = now(),
        expires_at = EXCLUDED.expires_at,
        revoked_at = NULL
RETURNING *;

-- name: RevokeBillShareLinkByBillID :one
UPDATE bill_share_links
SET revoked_at = now()
WHERE business_id = $1 AND bill_id = $2 AND revoked_at IS NULL
RETURNING *;

-- name: GetActiveBillShareLinkByTokenHash :one
SELECT * FROM bill_share_links
WHERE token_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now());
