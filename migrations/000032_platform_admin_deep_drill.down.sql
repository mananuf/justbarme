ALTER TABLE inventory_adjustment_requests DROP CONSTRAINT inventory_adjustment_requests_reason_category_check;
ALTER TABLE inventory_adjustment_requests ADD CONSTRAINT inventory_adjustment_requests_reason_category_check
    CHECK (reason_category IN
        ('complimentary', 'broken', 'spoiled', 'staff_use', 'manual', 'count_correction'));

ALTER TABLE platform_audit_log DROP COLUMN target_resource;
