-- A stock receipt entered wrong (quantity or cost) had no way to be
-- undone -- every other posted record type (sales, expenses) already has
-- a reversal. Mirrored exactly on sales_items' own precedent: quantity's
-- sign alone marks a reversal line (no separate boolean), so the CHECK
-- that used to require a positive quantity/non-negative cost must widen
-- the same way sale_items.quantity (CHECK quantity <> 0) and
-- line_total_kobo (no lower bound at all) already do.
ALTER TABLE stock_receipt_lines DROP CONSTRAINT stock_receipt_lines_quantity_check;
ALTER TABLE stock_receipt_lines ADD CONSTRAINT stock_receipt_lines_quantity_check CHECK (quantity <> 0);
ALTER TABLE stock_receipt_lines DROP CONSTRAINT stock_receipt_lines_total_cost_kobo_check;

-- A reversal is a new header row pointing back at the receipt it undoes --
-- same reversal_of_*_id precedent as sales/expenses, deliberately not
-- enforced with a DB-level UNIQUE: "at most one reversal" is a
-- query-time check (Service.ReverseStockReceipt), the same tolerance
-- sales.ReverseSale/GetReversalOfSale already accepts.
ALTER TABLE stock_receipts ADD COLUMN reversal_of_receipt_id UUID
    REFERENCES stock_receipts (id);

ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_type_check;
ALTER TABLE inventory_events
    ADD CONSTRAINT inventory_events_type_check
    CHECK (type IN ('receipt', 'sale', 'sale_reversal', 'adjustment', 'receipt_reversal'));

-- Lets a staff member flag any activity entry they can see for an owner to
-- look at -- "staff must request review" generalized across all four
-- activity types, rather than four separate mechanisms. Deliberately not
-- folded into sale_reviews/inventory_reviews: both of those are shaped
-- around a specific system-detected condition (a stale price, a negative
-- balance) with their own foreign keys, not a free-text staff concern
-- about an arbitrary posted record. source_type/source_id point at
-- whichever table the activity feed itself unions over
-- (internal/activity.Entry.Type/ID) -- no foreign key, since the target
-- varies by type and each of those tables is already independently
-- RLS-scoped and immutable.
CREATE TABLE activity_flags (
    id              UUID NOT NULL,
    business_id     UUID NOT NULL REFERENCES businesses (id),
    source_type     TEXT NOT NULL CHECK (source_type IN ('sale', 'expense', 'inventory_adjustment', 'stock_receipt')),
    source_id       UUID NOT NULL,
    flagged_by      UUID NOT NULL REFERENCES users (id),
    reason          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolved_by     UUID REFERENCES users (id),
    resolution_note TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ,
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    CHECK ((status = 'resolved') = (resolved_by IS NOT NULL AND resolved_at IS NOT NULL))
);

CREATE INDEX activity_flags_business_status_idx ON activity_flags (business_id, status, created_at DESC);
CREATE INDEX activity_flags_business_source_idx ON activity_flags (business_id, source_type, source_id);

ALTER TABLE activity_flags ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_flags FORCE ROW LEVEL SECURITY;

CREATE POLICY activity_flags_tenant_isolation ON activity_flags
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
