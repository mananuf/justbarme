-- One enrolled device belongs to exactly one business/location/user context
-- in the MVP. public_key carries the device's Ed25519 public key, base64
-- encoded, used later for offline event signing (out of Phase 2 scope).
CREATE TABLE devices (
    id           UUID NOT NULL,
    business_id  UUID NOT NULL REFERENCES businesses (id),
    user_id      UUID NOT NULL REFERENCES users (id),
    location_id  UUID NOT NULL,
    public_key   TEXT NOT NULL,
    display_name TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    enrolled_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_sync_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

CREATE INDEX devices_business_status_idx ON devices (business_id, status);
CREATE UNIQUE INDEX devices_business_public_key_key ON devices (business_id, public_key);

ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices FORCE ROW LEVEL SECURITY;

CREATE POLICY devices_tenant_isolation ON devices
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
