-- name: CreateIdentityVerification :one
INSERT INTO identity_verifications (id, user_id, channel, identifier, otp_hash, purpose, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetIdentityVerificationByID :one
SELECT * FROM identity_verifications WHERE id = $1;

-- name: IncrementIdentityVerificationAttempts :exec
UPDATE identity_verifications SET attempts = attempts + 1 WHERE id = $1;

-- name: DeleteIdentityVerification :exec
DELETE FROM identity_verifications WHERE id = $1;
