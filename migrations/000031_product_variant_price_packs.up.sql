-- Multi-buy / bundle pricing for a variant -- "2 games for 500, otherwise
-- 300 each, repeating indefinitely" -- docs/PHASE_MULTIBUY_PRICING.md. A
-- variant's own current product_prices row already serves as the implicit
-- "pack of 1"; this table only ever holds packs of size > 1, and a
-- variant with no rows here behaves exactly as it always has (plain
-- linear per-unit pricing) -- this is purely additive.
--
-- Same append-oriented, effective-dated shape as product_prices
-- (migration 000012), for the same reason: a price change here closes the
-- old row and inserts a new one in the same transaction rather than
-- UPDATEing amount_kobo in place (internal/catalogue.Service.
-- SetVariantPricePacks). Historical correctness of past sales does not
-- actually depend on this -- internal/sales freezes the real charged
-- price onto each sale_item at the time of sale regardless -- but the
-- shape is kept consistent with product_prices anyway since it is, at
-- heart, the same kind of fact (what something costs).
CREATE TABLE product_variant_price_packs (
    id              UUID NOT NULL,
    business_id     UUID NOT NULL REFERENCES businesses (id),
    variant_id      UUID NOT NULL,
    pack_quantity   INTEGER NOT NULL CHECK (pack_quantity > 1),
    pack_price_kobo BIGINT NOT NULL CHECK (pack_price_kobo > 0),
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to        TIMESTAMPTZ,
    created_by      UUID NOT NULL REFERENCES users (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    CHECK (valid_to IS NULL OR valid_to > valid_from)
);

CREATE INDEX product_variant_price_packs_business_variant_valid_from_idx
    ON product_variant_price_packs (business_id, variant_id, valid_from DESC);

-- At most one current pack per (variant, quantity) -- the same
-- "application code alone cannot guarantee this under concurrent
-- transactions" reasoning as product_prices_one_current_per_variant.
CREATE UNIQUE INDEX product_variant_price_packs_one_current_per_qty
    ON product_variant_price_packs (business_id, variant_id, pack_quantity)
    WHERE valid_to IS NULL;

ALTER TABLE product_variant_price_packs ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_variant_price_packs FORCE ROW LEVEL SECURITY;

CREATE POLICY product_variant_price_packs_tenant_isolation ON product_variant_price_packs
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
