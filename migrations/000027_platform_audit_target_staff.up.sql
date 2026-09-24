-- Staff account management (create/revoke) has a target too, just not a
-- business: record which platform_staff row the action was performed on.
-- Nullable and additive -- every existing row and every business-targeted
-- action simply leaves it NULL.
ALTER TABLE platform_audit_log
    ADD COLUMN target_staff_id UUID REFERENCES platform_staff (id);
