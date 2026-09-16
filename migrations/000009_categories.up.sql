-- Business-owned, editable, sortable, and deactivatable -- see
-- docs/ARCHITECTURE.md §8.2. Never hard-deleted: a category referenced by a
-- product is only ever deactivated (internal/catalogue.Service.UpdateCategory).
CREATE TABLE categories (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    name        TEXT NOT NULL,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, name)
);

CREATE INDEX categories_business_active_idx ON categories (business_id, active);

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories FORCE ROW LEVEL SECURITY;

CREATE POLICY categories_tenant_isolation ON categories
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
