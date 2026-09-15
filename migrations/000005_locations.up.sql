-- One default location per business, created alongside the business itself.
-- Multi-branch is out of scope for the MVP UI, but attaching devices and
-- (later) inventory to a location rather than the business avoids a later
-- branch-migration redesign.
CREATE TABLE locations (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    name        TEXT NOT NULL DEFAULT 'Main',
    is_default  BOOLEAN NOT NULL DEFAULT true,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id)
);

CREATE INDEX locations_business_status_idx ON locations (business_id, status);

-- At most one default location per business.
CREATE UNIQUE INDEX locations_one_default_per_business
    ON locations (business_id) WHERE is_default;

ALTER TABLE locations ENABLE ROW LEVEL SECURITY;
ALTER TABLE locations FORCE ROW LEVEL SECURITY;

CREATE POLICY locations_tenant_isolation ON locations
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
