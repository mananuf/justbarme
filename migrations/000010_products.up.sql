-- Conceptual product, e.g. "Guinness" -- see docs/ARCHITECTURE.md §8.2.
-- category_id is nullable (an uncategorized product is valid); the
-- composite foreign key is simply not enforced when it is NULL (Postgres
-- MATCH SIMPLE, the default). Never hard-deleted: a referenced product is
-- only ever deactivated (internal/catalogue.Service.UpdateProduct).
CREATE TABLE products (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    category_id UUID,
    name        TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, name),
    FOREIGN KEY (business_id, category_id) REFERENCES categories (business_id, id)
);

CREATE INDEX products_business_active_idx ON products (business_id, active);
CREATE INDEX products_business_category_idx ON products (business_id, category_id);

ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE products FORCE ROW LEVEL SECURITY;

CREATE POLICY products_tenant_isolation ON products
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
