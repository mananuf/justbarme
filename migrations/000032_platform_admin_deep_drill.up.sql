-- docs/PHASE_PLATFORM_ADMIN_DEEP_DRILL.md: widens platform admin from
-- aggregate-only oversight to full line-item read/corrective-action
-- access. Two schema changes, both additive.

-- target_resource records WHICH specific record a deep-drill read or
-- corrective action touched (e.g. "sale:3f2a...", "stock_receipt:9c1b...")
-- -- the existing columns (target_business_id, a free-text reason) are
-- enough for "suspended this business" but not for "viewed/reversed this
-- specific record". Deliberately a plain string, not a polymorphic
-- foreign key per resource type, matching this table's own existing
-- "nullable, free-text" style (see reason) rather than inventing a new
-- schema shape for a handful of resource types.
ALTER TABLE platform_audit_log ADD COLUMN target_resource TEXT;

-- Lets a superadmin correct an inventory balance that has no explaining
-- receipt/adjustment trail (e.g. a bug left it wrong) -- still posted
-- through the existing adjustment-request ledger (RequestAdjustment +
-- ApproveAdjustmentRequest), never a raw UPDATE on inventory_balances, so
-- "a balance is a rebuildable projection over its own movements" stays
-- true even for this. Excluded from permittedAdjustmentReasons
-- (internal/httpapi/inventory_counts_handlers.go) the same way
-- count_correction already is -- a business owner/staff member can never
-- submit this reason directly.
ALTER TABLE inventory_adjustment_requests DROP CONSTRAINT inventory_adjustment_requests_reason_category_check;
ALTER TABLE inventory_adjustment_requests ADD CONSTRAINT inventory_adjustment_requests_reason_category_check
    CHECK (reason_category IN
        ('complimentary', 'broken', 'spoiled', 'staff_use', 'manual', 'count_correction', 'platform_correction'));
