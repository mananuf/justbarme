-- name: UpsertSignupVerification :one
INSERT INTO signup_verifications (id, email, display_name, password_hash, otp_hash, attempts, expires_at)
VALUES ($1, $2, $3, $4, $5, 0, $6)
ON CONFLICT (lower(email)) DO UPDATE
    SET display_name = EXCLUDED.display_name,
        password_hash = EXCLUDED.password_hash,
        otp_hash = EXCLUDED.otp_hash,
        attempts = 0,
        expires_at = EXCLUDED.expires_at,
        updated_at = now()
RETURNING *;

-- name: GetSignupVerificationByEmail :one
SELECT * FROM signup_verifications WHERE lower(email) = lower(sqlc.arg(email));

-- name: IncrementSignupVerificationAttempts :exec
UPDATE signup_verifications SET attempts = attempts + 1, updated_at = now() WHERE id = $1;

-- name: DeleteSignupVerification :exec
DELETE FROM signup_verifications WHERE id = $1;
