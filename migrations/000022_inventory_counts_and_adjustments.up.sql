-- Inventory counts, staff adjustment requests/approvals, and negative-
-- stock/stale-count reviews -- docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md,
-- the rest of Phase 7 that docs/PHASE_STOCK_RECEIVING.md deliberately
-- deferred. Narrow, purpose-built tables (mirroring sale_reviews over the
-- generic review_cases docs/ARCHITECTURE.md §8.7 sketches), not the fully
-- generic version.

-- Widen inventory_events for the one new operation type this phase adds.
-- Each addition is its own migration, deliberately, per the narrow-CHECK
-- reasoning in migration 000018.
ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_type_check;
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_type_check
    CHECK (type IN ('receipt', 'sale', 'sale_reversal', 'adjustment'));

-- A physical stock count. idempotency_key makes a queued-offline count
-- submission safe to retry (docs/ARCHITECTURE.md's offline-first
-- principle, same precedent as sales.idempotency_key) -- both
-- inventory:count and inventory:adjustment_request are already
-- offline-safe capabilities (internal/tenancy/capabilities.go), predating
-- this migration. started_at is informational only (when the counter
-- actually walked the shelf) -- staleness is decided by comparing the
-- submitted expected_quantity against the live balance at processing
-- time, never by comparing timestamps (docs/ARCHITECTURE.md §10.2's
-- "do not use client timestamps to establish canonical ordering," applied
-- here too even though this isn't the sync protocol).
CREATE TABLE stock_counts (
    id              UUID NOT NULL,
    business_id     UUID NOT NULL REFERENCES businesses (id),
    location_id     UUID NOT NULL,
    counted_by      UUID NOT NULL REFERENCES users (id),
    idempotency_key UUID NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, idempotency_key),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

CREATE INDEX stock_counts_business_created_at_idx ON stock_counts (business_id, created_at DESC);

ALTER TABLE stock_counts ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_counts FORCE ROW LEVEL SECURITY;

CREATE POLICY stock_counts_tenant_isolation ON stock_counts
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- One row per counted variant. expected_quantity is what the counting
-- device believed the balance was (a live fetch, or -- if offline --
-- web/src/lib/db.ts's catalogueCache); is_stale is set once, at server
-- processing time, and never rewritten afterward -- counts are preserved
-- observations, not something silently corrected in place
-- (docs/ARCHITECTURE.md §8.4).
CREATE TABLE stock_count_lines (
    id                UUID NOT NULL,
    business_id       UUID NOT NULL REFERENCES businesses (id),
    count_id          UUID NOT NULL,
    variant_id        UUID NOT NULL,
    expected_quantity INTEGER NOT NULL,
    physical_quantity INTEGER NOT NULL CHECK (physical_quantity >= 0),
    variance          INTEGER NOT NULL,
    is_stale          BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, count_id) REFERENCES stock_counts (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id)
);

CREATE INDEX stock_count_lines_count_idx ON stock_count_lines (count_id);
CREATE INDEX stock_count_lines_business_variant_idx ON stock_count_lines (business_id, variant_id);

ALTER TABLE stock_count_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_count_lines FORCE ROW LEVEL SECURITY;

CREATE POLICY stock_count_lines_tenant_isolation ON stock_count_lines
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Any signed inventory change that isn't a receipt or a sale: complimentary
-- give-aways, breakage, spoilage, staff use, a plain manual correction, or
-- the non-stale variance from a stock count (source_count_line_id set,
-- reason_category='count_correction'). Staff may submit
-- (inventory:adjustment_request, offline-safe); only Owner may decide
-- (inventory:adjustment_approve, deliberately not offline-safe -- approval
-- always requires online server authorization). Approving is what actually
-- posts the inventory_movement; rejecting never does.
CREATE TABLE inventory_adjustment_requests (
    id                   UUID NOT NULL,
    business_id          UUID NOT NULL REFERENCES businesses (id),
    location_id          UUID NOT NULL,
    variant_id           UUID NOT NULL,
    requested_by         UUID NOT NULL REFERENCES users (id),
    idempotency_key      UUID NOT NULL,
    quantity_delta       INTEGER NOT NULL CHECK (quantity_delta <> 0),
    reason_category      TEXT NOT NULL CHECK (reason_category IN
        ('complimentary', 'broken', 'spoiled', 'staff_use', 'manual', 'count_correction')),
    reason_note          TEXT NOT NULL,
    source_count_line_id UUID,
    status               TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    decided_by           UUID REFERENCES users (id),
    decided_at           TIMESTAMPTZ,
    resolution_note      TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, idempotency_key),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    FOREIGN KEY (business_id, source_count_line_id) REFERENCES stock_count_lines (business_id, id),
    CHECK ((status = 'pending') = (decided_at IS NULL))
);

CREATE INDEX inventory_adjustment_requests_business_status_idx
    ON inventory_adjustment_requests (business_id, status);

ALTER TABLE inventory_adjustment_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_adjustment_requests FORCE ROW LEVEL SECURITY;

CREATE POLICY inventory_adjustment_requests_tenant_isolation ON inventory_adjustment_requests
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Now that inventory_adjustment_requests exists, link an 'adjustment'
-- inventory_events row back to the request that authorized it (required
-- for that type, same pattern as sale_id on 'sale'/'sale_reversal' from
-- migration 000019).
ALTER TABLE inventory_events ADD COLUMN adjustment_request_id UUID;
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_adjustment_request_id_fkey
    FOREIGN KEY (business_id, adjustment_request_id) REFERENCES inventory_adjustment_requests (business_id, id);
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_adjustment_request_id_required
    CHECK (type <> 'adjustment' OR adjustment_request_id IS NOT NULL);

-- Mirrors sale_reviews' shape exactly. negative_inventory is automatic/
-- system-detected only (no staff-initiated path) -- a human who suspects a
-- discrepancy submits a count instead, which is the mechanism designed for
-- that. stale_stock_count opens when a count's expected_quantity no longer
-- matches the live balance by the time it's processed.
CREATE TABLE inventory_reviews (
    id                    UUID NOT NULL,
    business_id           UUID NOT NULL REFERENCES businesses (id),
    type                  TEXT NOT NULL CHECK (type IN ('negative_inventory', 'stale_stock_count')),
    variant_id            UUID NOT NULL,
    location_id           UUID NOT NULL,
    related_movement_id   UUID,
    related_count_line_id UUID,
    status                TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolved_by           UUID REFERENCES users (id),
    resolved_note         TEXT,
    resolved_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id),
    FOREIGN KEY (business_id, related_movement_id) REFERENCES inventory_movements (business_id, id),
    FOREIGN KEY (business_id, related_count_line_id) REFERENCES stock_count_lines (business_id, id),
    CHECK ((status = 'open') = (resolved_at IS NULL)),
    CHECK (type <> 'negative_inventory' OR related_movement_id IS NOT NULL),
    CHECK (type <> 'stale_stock_count' OR related_count_line_id IS NOT NULL)
);

CREATE INDEX inventory_reviews_business_status_idx ON inventory_reviews (business_id, status);

ALTER TABLE inventory_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_reviews FORCE ROW LEVEL SECURITY;

CREATE POLICY inventory_reviews_tenant_isolation ON inventory_reviews
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
