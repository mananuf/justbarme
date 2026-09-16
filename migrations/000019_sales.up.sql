-- Walk-in selling -- docs/ARCHITECTURE.md §8.3, Phase 5, scoped to a
-- single device with no staff yet (Phase 4's multi-device sync foundation
-- is deferred -- see the memory note this decision was recorded under).
-- One unified bill concept, but only the walk-in-relevant statuses exist
-- here: tabs (`closed_unpaid`, `void`, the `tables`/`customers` concept)
-- are Phase 6.

CREATE TABLE bills (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    location_id UUID NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'settled')),
    opened_by   UUID NOT NULL REFERENCES users (id),
    opened_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

CREATE INDEX bills_business_opened_at_idx ON bills (business_id, opened_at DESC);

ALTER TABLE bills ENABLE ROW LEVEL SECURITY;
ALTER TABLE bills FORCE ROW LEVEL SECURITY;
CREATE POLICY bills_tenant_isolation ON bills
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Immutable posted round. idempotency_key is client-generated (a UUID made
-- on-device at sale-creation time) so a retried POST after a lost response
-- can never double-post -- this is single-device retry-safety, not Phase
-- 4's multi-device sync. reversal_of_sale_id is null on every ordinary
-- sale; a reversal is a second row here, never an edit of the original.
CREATE TABLE sales (
    id                  UUID NOT NULL,
    business_id         UUID NOT NULL REFERENCES businesses (id),
    bill_id             UUID NOT NULL,
    seller_id           UUID NOT NULL REFERENCES users (id),
    idempotency_key     UUID NOT NULL,
    occurred_at         TIMESTAMPTZ NOT NULL,
    received_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Always SUM(sale_items.line_total_kobo): positive for an ordinary
    -- sale (all quantities positive), negative for a reversal (all
    -- quantities negative) -- never a separately-asserted magnitude, so
    -- summing total_kobo across every row in this table is always the
    -- correct net figure with no CASE WHEN needed downstream.
    total_kobo          BIGINT NOT NULL CHECK (total_kobo <> 0),
    reversal_of_sale_id UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, idempotency_key),
    FOREIGN KEY (business_id, bill_id) REFERENCES bills (business_id, id),
    FOREIGN KEY (business_id, reversal_of_sale_id) REFERENCES sales (business_id, id)
);

CREATE INDEX sales_business_occurred_at_idx ON sales (business_id, occurred_at DESC);
CREATE INDEX sales_business_bill_idx ON sales (business_id, bill_id);

ALTER TABLE sales ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales FORCE ROW LEVEL SECURITY;
CREATE POLICY sales_tenant_isolation ON sales
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- description is a name snapshot (e.g. "Trophy Lager -- 50cl Bottle"), not
-- a live join -- a later product rename must never change history.
-- quantity is negative on a reversal's compensating lines, never zero.
CREATE TABLE sale_items (
    id              UUID NOT NULL,
    business_id     UUID NOT NULL REFERENCES businesses (id),
    sale_id         UUID NOT NULL,
    variant_id      UUID NOT NULL,
    description     TEXT NOT NULL,
    quantity        INTEGER NOT NULL CHECK (quantity <> 0),
    unit_price_kobo BIGINT NOT NULL CHECK (unit_price_kobo >= 0),
    line_total_kobo BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, sale_id) REFERENCES sales (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id)
);

CREATE INDEX sale_items_sale_idx ON sale_items (sale_id);
CREATE INDEX sale_items_business_variant_idx ON sale_items (business_id, variant_id);

ALTER TABLE sale_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE sale_items FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_items_tenant_isolation ON sale_items
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Track cash/transfer/card per docs/ARCHITECTURE.md §8.3. amount_kobo is
-- negative for a reversal's refund row. Partial payment / outstanding
-- balance is Phase 6 (tabs) -- a walk-in sale here is always paid in full,
-- one payment per sale (or per reversal).
CREATE TABLE payments (
    id                     UUID NOT NULL,
    business_id            UUID NOT NULL REFERENCES businesses (id),
    bill_id                UUID NOT NULL,
    amount_kobo            BIGINT NOT NULL CHECK (amount_kobo <> 0),
    method                 TEXT NOT NULL CHECK (method IN ('cash', 'transfer', 'card')),
    actor_id               UUID NOT NULL REFERENCES users (id),
    reversal_of_payment_id UUID,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, bill_id) REFERENCES bills (business_id, id),
    FOREIGN KEY (business_id, reversal_of_payment_id) REFERENCES payments (business_id, id)
);

CREATE INDEX payments_business_bill_idx ON payments (business_id, bill_id);

ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments FORCE ROW LEVEL SECURITY;
CREATE POLICY payments_tenant_isolation ON payments
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Scoped specifically to sale-item staleness (a deactivated variant, or a
-- submitted price that was never actually in effect at the claimed sale
-- time) -- deliberately NOT the generic, multi-purpose review_cases system
-- docs/IMPLEMENTATION_PLAN.md's Phase 4 describes (that one also covers
-- stock counts and adjustments, neither of which exist yet). A review
-- never changes the sale it's attached to -- see
-- internal/sales.Service.CreateSale: the sale always posts exactly as
-- submitted, a review is a flag for the owner, never a rejection.
CREATE TABLE sale_reviews (
    id            UUID NOT NULL,
    business_id   UUID NOT NULL REFERENCES businesses (id),
    sale_id       UUID NOT NULL,
    sale_item_id  UUID,
    reason        TEXT NOT NULL CHECK (reason IN ('deactivated_variant', 'price_mismatch')),
    status        TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolved_by   UUID REFERENCES users (id),
    resolved_note TEXT,
    resolved_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, sale_id) REFERENCES sales (business_id, id),
    FOREIGN KEY (business_id, sale_item_id) REFERENCES sale_items (business_id, id),
    CHECK ((status = 'open') = (resolved_at IS NULL))
);

CREATE INDEX sale_reviews_business_status_idx ON sale_reviews (business_id, status);

ALTER TABLE sale_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE sale_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_reviews_tenant_isolation ON sale_reviews
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Widen inventory_events for the two new operation types this phase adds.
-- Each addition is its own migration, deliberately, per the narrow-CHECK
-- reasoning in migration 000018.
ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_type_check;
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_type_check
    CHECK (type IN ('receipt', 'sale', 'sale_reversal'));

ALTER TABLE inventory_events ADD COLUMN sale_id UUID;
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_sale_id_fkey
    FOREIGN KEY (business_id, sale_id) REFERENCES sales (business_id, id);
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_sale_id_required
    CHECK (type NOT IN ('sale', 'sale_reversal') OR sale_id IS NOT NULL);
