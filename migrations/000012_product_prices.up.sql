-- Append-oriented effective-dated rows -- see docs/ARCHITECTURE.md §8.2.
-- amount_kobo is never UPDATEd on an existing row; a price change closes
-- the current row (sets valid_to) and inserts a new one, atomically, in the
-- same transaction -- see internal/catalogue.Service.SetVariantPrice. Sale
-- items copy the actual price charged at the time, so a historical receipt
-- never changes after a later price update.
CREATE TABLE product_prices (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    variant_id  UUID NOT NULL,
    amount_kobo BIGINT NOT NULL CHECK (amount_kobo >= 0),
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to    TIMESTAMPTZ,
    created_by  UUID NOT NULL REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    CHECK (valid_to IS NULL OR valid_to > valid_from)
);

-- Matches docs/ARCHITECTURE.md §13's baseline index list verbatim:
-- "product_prices (business_id, variant_id, valid_from desc)".
CREATE INDEX product_prices_business_variant_valid_from_idx
    ON product_prices (business_id, variant_id, valid_from DESC);

-- "One active price per variant" (docs/ARCHITECTURE.md §13) is enforced
-- here, not just by application code: a partial unique index rejects a
-- second valid_to IS NULL row for the same variant even under concurrent
-- transactions, which application-level close-then-insert logic alone
-- cannot guarantee under READ COMMITTED.
CREATE UNIQUE INDEX product_prices_one_current_per_variant
    ON product_prices (business_id, variant_id) WHERE valid_to IS NULL;

ALTER TABLE product_prices ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_prices FORCE ROW LEVEL SECURITY;

CREATE POLICY product_prices_tenant_isolation ON product_prices
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
