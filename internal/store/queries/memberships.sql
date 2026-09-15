-- name: CreateMembership :one
INSERT INTO business_memberships (business_id, user_id, role, status, joined_at)
VALUES ($1, $2, $3, 'active', now())
RETURNING *;

-- name: GetMembership :one
SELECT * FROM business_memberships
WHERE business_id = $1 AND user_id = $2;

-- name: ListMembershipsForUser :many
SELECT bm.*, b.name AS business_name
FROM business_memberships bm
JOIN businesses b ON b.id = bm.business_id
WHERE bm.user_id = $1
ORDER BY bm.created_at;
