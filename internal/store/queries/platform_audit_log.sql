-- name: CreatePlatformAuditEntry :one
INSERT INTO platform_audit_log (id, platform_staff_id, action, target_business_id, reason, request_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListPlatformAuditLog :many
SELECT * FROM platform_audit_log ORDER BY created_at DESC LIMIT $1;
