-- name: CreateActivityFlag :one
INSERT INTO activity_flags (id, business_id, source_type, source_id, flagged_by, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListOpenActivityFlags :many
SELECT * FROM activity_flags WHERE business_id = $1 AND status = 'open' ORDER BY created_at DESC;

-- name: GetActivityFlagByID :one
SELECT * FROM activity_flags WHERE business_id = $1 AND id = $2;

-- name: ResolveActivityFlag :one
UPDATE activity_flags
SET status = 'resolved', resolved_by = $3, resolved_at = now(), resolution_note = $4
WHERE business_id = $1 AND id = $2 AND status = 'open'
RETURNING *;
