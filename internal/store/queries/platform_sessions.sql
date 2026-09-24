-- name: CreatePlatformSession :one
INSERT INTO platform_sessions (id, staff_id, token_hash, csrf_token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetActivePlatformSessionByTokenHash :one
SELECT * FROM platform_sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: TouchPlatformSession :exec
UPDATE platform_sessions SET last_used_at = now() WHERE id = $1;

-- name: UpdatePlatformSessionCSRFTokenHash :exec
UPDATE platform_sessions SET csrf_token_hash = $2 WHERE id = $1;

-- name: RevokePlatformSession :execrows
UPDATE platform_sessions
SET revoked_at = now(), revoked_reason = $2
WHERE id = $1 AND staff_id = $3 AND revoked_at IS NULL;

-- name: RevokeAllPlatformSessionsForStaff :exec
UPDATE platform_sessions
SET revoked_at = now(), revoked_reason = $2
WHERE staff_id = $1 AND revoked_at IS NULL;
