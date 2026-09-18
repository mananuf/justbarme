-- name: CreateInvitation :one
INSERT INTO invitations (id, business_id, invited_by, phone, email, role, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetInvitationByTokenHash :one
-- The public, unauthenticated lookup -- the hashed token is itself the
-- access control (invitations carries no RLS; see migration 000021).
SELECT * FROM invitations WHERE token_hash = $1;

-- name: GetInvitationByID :one
SELECT * FROM invitations WHERE business_id = $1 AND id = $2;

-- name: ListInvitationsByBusiness :many
SELECT * FROM invitations WHERE business_id = $1 ORDER BY created_at DESC;

-- name: RevokeInvitation :one
UPDATE invitations SET status = 'revoked', updated_at = now()
    WHERE business_id = $1 AND id = $2 AND status = 'pending'
RETURNING *;

-- name: AcceptInvitation :one
UPDATE invitations SET status = 'accepted', accepted_by = $2, accepted_at = now(), updated_at = now()
    WHERE id = $1 AND status = 'pending'
RETURNING *;
