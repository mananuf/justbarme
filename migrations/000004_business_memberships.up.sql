CREATE TABLE business_memberships (
    business_id  UUID NOT NULL REFERENCES businesses (id),
    user_id      UUID NOT NULL REFERENCES users (id),
    role         TEXT NOT NULL CHECK (role IN ('owner', 'staff')),
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    invited_at   TIMESTAMPTZ,
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, user_id)
);

CREATE INDEX business_memberships_business_status_user_idx
    ON business_memberships (business_id, status, user_id);

-- A user's own memberships are looked up across businesses (e.g. GET /me),
-- which is exactly the kind of query RLS below must not block for its own row.
CREATE INDEX business_memberships_user_idx ON business_memberships (user_id);

ALTER TABLE business_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_memberships FORCE ROW LEVEL SECURITY;

-- Tenant-scoped access, for handlers operating within a resolved business.
CREATE POLICY business_memberships_tenant_isolation ON business_memberships
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Self access, for handlers resolving "which businesses am I a member of"
-- before any business_id has been chosen (GET /me has no tenant header).
CREATE POLICY business_memberships_self_access ON business_memberships
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid);
