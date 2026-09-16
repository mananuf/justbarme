-- The sold and stocked unit, e.g. "50cl Bottle" -- see
-- docs/ARCHITECTURE.md §8.2. All sale and stock operations reference a
-- variant, not only a product, keeping size, price, quantity, and purchase
-- cost operationally distinct. Never hard-deleted: a referenced variant is
-- only ever deactivated (internal/catalogue.Service.UpdateVariant).
CREATE TABLE product_variants (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    product_id  UUID NOT NULL,
    name        TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, product_id, name),
    FOREIGN KEY (business_id, product_id) REFERENCES products (business_id, id)
);

-- Matches docs/ARCHITECTURE.md §13's baseline index list verbatim:
-- "product_variants (business_id, product_id, active)".
CREATE INDEX product_variants_business_product_active_idx ON product_variants (business_id, product_id, active);

ALTER TABLE product_variants ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_variants FORCE ROW LEVEL SECURITY;

CREATE POLICY product_variants_tenant_isolation ON product_variants
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
