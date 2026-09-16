-- Stock receiving -- see docs/PHASE_STOCK_RECEIVING.md and
-- docs/ARCHITECTURE.md §8.4. A deliberately thin slice of the full
-- inventory model there: receipts, lots, movements, and a balance
-- projection only -- no counts, adjustment requests, or review cases yet,
-- since those need staff and sales to exist to mean anything.

-- Receiving stock is the entry point: a bar owner buys N units (often
-- packaged in a crate/carton) at some total cost, and this is the header
-- row for that purchase.
CREATE TABLE stock_receipts (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    location_id UUID NOT NULL,
    received_by UUID NOT NULL REFERENCES users (id),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

CREATE INDEX stock_receipts_business_received_at_idx ON stock_receipts (business_id, received_at DESC);

ALTER TABLE stock_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_receipts FORCE ROW LEVEL SECURITY;

CREATE POLICY stock_receipts_tenant_isolation ON stock_receipts
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- One receipt can cover several variants delivered together; each line is
-- exactly one variant's quantity and total cost for that delivery. Per
-- docs/ARCHITECTURE.md §8.4: "Store exact line total in kobo, not only a
-- rounded unit cost" -- per-unit cost is a display-time calculation
-- (total_cost_kobo / quantity), never stored, so repeated rounding never
-- drifts from the truth.
CREATE TABLE stock_receipt_lines (
    id              UUID NOT NULL,
    business_id     UUID NOT NULL REFERENCES businesses (id),
    receipt_id      UUID NOT NULL,
    variant_id      UUID NOT NULL,
    quantity        INTEGER NOT NULL CHECK (quantity > 0),
    total_cost_kobo BIGINT NOT NULL CHECK (total_cost_kobo >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, receipt_id) REFERENCES stock_receipts (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id)
);

CREATE INDEX stock_receipt_lines_receipt_idx ON stock_receipt_lines (receipt_id);
CREATE INDEX stock_receipt_lines_business_variant_idx ON stock_receipt_lines (business_id, variant_id);

ALTER TABLE stock_receipt_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_receipt_lines FORCE ROW LEVEL SECURITY;

CREATE POLICY stock_receipt_lines_tenant_isolation ON stock_receipt_lines
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- One immutable costed lot per receipt line -- docs/ARCHITECTURE.md §8.4.
-- Duplicates receipt-line data today (nothing consumes a lot yet -- no
-- sales exist), kept as its own table specifically so a later FIFO
-- allocator (Phase 10) can consume lots without a schema redesign: "the
-- lot's original values are immutable," never updated or deleted here.
CREATE TABLE stock_lots (
    id                UUID NOT NULL,
    business_id       UUID NOT NULL REFERENCES businesses (id),
    receipt_line_id   UUID NOT NULL,
    variant_id        UUID NOT NULL,
    location_id       UUID NOT NULL,
    received_quantity INTEGER NOT NULL CHECK (received_quantity > 0),
    total_cost_kobo   BIGINT NOT NULL CHECK (total_cost_kobo >= 0),
    received_at       TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, receipt_line_id) REFERENCES stock_receipt_lines (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

CREATE INDEX stock_lots_business_variant_idx ON stock_lots (business_id, variant_id);

ALTER TABLE stock_lots ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_lots FORCE ROW LEVEL SECURITY;

CREATE POLICY stock_lots_tenant_isolation ON stock_lots
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Operation headers -- docs/ARCHITECTURE.md §8.4 "inventory_events". type
-- is deliberately narrow: only 'receipt' exists until sales, reversals,
-- adjustments, and count adjustments are built, each as its own migration
-- widening this CHECK rather than a silently open-ended column.
CREATE TABLE inventory_events (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    type        TEXT NOT NULL CHECK (type IN ('receipt')),
    actor_id    UUID NOT NULL REFERENCES users (id),
    receipt_id  UUID,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, receipt_id) REFERENCES stock_receipts (business_id, id),
    CHECK (type <> 'receipt' OR receipt_id IS NOT NULL)
);

ALTER TABLE inventory_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_events FORCE ROW LEVEL SECURITY;

CREATE POLICY inventory_events_tenant_isolation ON inventory_events
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Append-only signed quantity movements -- docs/ARCHITECTURE.md §8.4.
-- Receipts are always positive; nothing else writes here yet.
CREATE TABLE inventory_movements (
    id             UUID NOT NULL,
    business_id    UUID NOT NULL REFERENCES businesses (id),
    event_id       UUID NOT NULL,
    variant_id     UUID NOT NULL,
    location_id    UUID NOT NULL,
    quantity_delta INTEGER NOT NULL CHECK (quantity_delta <> 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, event_id) REFERENCES inventory_events (business_id, id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

CREATE INDEX inventory_movements_business_variant_location_idx
    ON inventory_movements (business_id, variant_id, location_id);

ALTER TABLE inventory_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_movements FORCE ROW LEVEL SECURITY;

CREATE POLICY inventory_movements_tenant_isolation ON inventory_movements
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Rebuildable projection -- docs/ARCHITECTURE.md §8.4: "inventory_balances
-- is a rebuildable projection keyed by business, location, and variant."
-- Maintained as an atomic increment alongside each movement in the same
-- transaction, never treated as the source of truth -- inventory_movements
-- is, and this table could in principle be dropped and rebuilt from it.
CREATE TABLE inventory_balances (
    business_id UUID NOT NULL REFERENCES businesses (id),
    variant_id  UUID NOT NULL,
    location_id UUID NOT NULL,
    quantity    INTEGER NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, variant_id, location_id),
    FOREIGN KEY (business_id, variant_id) REFERENCES product_variants (business_id, id),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id)
);

ALTER TABLE inventory_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_balances FORCE ROW LEVEL SECURITY;

CREATE POLICY inventory_balances_tenant_isolation ON inventory_balances
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
