-- Phase 10 (docs/IMPLEMENTATION_PLAN.md, docs/ARCHITECTURE.md §8.5,
-- docs/PHASE_FIFO_COSTING.md): FIFO cost allocation at sale time.
-- stock_lots has stored exact receipt cost since migration 000018
-- specifically so this could be added without redesigning it -- see that
-- migration's own comment.

-- remaining_quantity is decremented as sale_item_lot_allocations consume
-- from this lot -- a stored, rebuildable projection, same status as
-- inventory_balances (read on every sale, not just for display, so it's
-- not left as a pure derived query). Backfilled to received_quantity for
-- every existing row, since nothing has ever consumed from a lot before
-- this migration.
ALTER TABLE stock_lots ADD COLUMN remaining_quantity INTEGER NOT NULL DEFAULT 0;
UPDATE stock_lots SET remaining_quantity = received_quantity;

-- A review-resolved lot (see below) has no real stock_receipt_lines row
-- to point to -- it exists only to give a previously-unresolved sale a
-- real cost, after the fact, on an owner's explicit say-so.
ALTER TABLE stock_lots ALTER COLUMN receipt_line_id DROP NOT NULL;
ALTER TABLE stock_lots ADD COLUMN source TEXT NOT NULL DEFAULT 'receipt'
    CHECK (source IN ('receipt', 'review_resolution'));

-- Links a negative_inventory review back to the specific sale item whose
-- oversell created it -- NULL for a negative_inventory review triggered
-- by an approved inventory adjustment instead (no sale, no allocation to
-- resolve). This is what lets ResolveInventoryReview find the pending
-- allocation a supplied resolved_unit_cost_kobo should close out -- see
-- docs/PHASE_FIFO_COSTING.md §5.
ALTER TABLE inventory_reviews ADD COLUMN sale_item_id UUID;
ALTER TABLE inventory_reviews
    ADD FOREIGN KEY (business_id, sale_item_id) REFERENCES sale_items (business_id, id);

-- Immutable -- docs/IMPLEMENTATION_PLAN.md Phase 10 task 2 ("Add
-- immutable sale-item lot allocations"). A row is never updated once
-- written; a correction (a reversal, or a pending allocation being
-- resolved) is always a new, separate, sign-mirrored row -- see
-- docs/PHASE_FIFO_COSTING.md §5 and §6.
CREATE TABLE sale_item_lot_allocations (
    id             UUID NOT NULL,
    business_id    UUID NOT NULL REFERENCES businesses (id),
    sale_item_id   UUID NOT NULL,
    -- NULL means "sold, cost not yet known" (an oversell past every
    -- lot's coverage) -- never a fabricated cost. See
    -- docs/PHASE_FIFO_COSTING.md §4.
    stock_lot_id   UUID,
    -- Signed: positive for an ordinary sale's consumption, negative for
    -- a reversal's give-back or a resolution's pending-quantity
    -- close-out.
    quantity       INTEGER NOT NULL CHECK (quantity <> 0),
    allocated_cost_kobo BIGINT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, sale_item_id) REFERENCES sale_items (business_id, id),
    FOREIGN KEY (business_id, stock_lot_id) REFERENCES stock_lots (business_id, id),
    CHECK ((stock_lot_id IS NULL) = (allocated_cost_kobo IS NULL))
);

CREATE INDEX sale_item_lot_allocations_business_sale_item_idx
    ON sale_item_lot_allocations (business_id, sale_item_id);
CREATE INDEX sale_item_lot_allocations_business_lot_idx
    ON sale_item_lot_allocations (business_id, stock_lot_id);

ALTER TABLE sale_item_lot_allocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE sale_item_lot_allocations FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_item_lot_allocations_tenant_isolation ON sale_item_lot_allocations
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
