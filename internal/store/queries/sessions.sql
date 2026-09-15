-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetActiveSessionByTokenHash :one
SELECT * FROM sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: TouchSession :exec
UPDATE sessions SET last_used_at = now() WHERE id = $1;

-- name: UpdateSessionCSRFTokenHash :exec
UPDATE sessions SET csrf_token_hash = $2 WHERE id = $1;

-- name: RevokeSession :execrows
UPDATE sessions
SET revoked_at = now(), revoked_reason = $2
WHERE id = $1 AND user_id = $3 AND revoked_at IS NULL;
