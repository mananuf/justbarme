DROP TABLE IF EXISTS inventory_reviews;

ALTER TABLE inventory_events DROP CONSTRAINT IF EXISTS inventory_events_adjustment_request_id_required;
ALTER TABLE inventory_events DROP CONSTRAINT IF EXISTS inventory_events_adjustment_request_id_fkey;
ALTER TABLE inventory_events DROP COLUMN IF EXISTS adjustment_request_id;

DROP TABLE IF EXISTS inventory_adjustment_requests;
DROP TABLE IF EXISTS stock_count_lines;
DROP TABLE IF EXISTS stock_counts;

ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_type_check;
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_type_check
    CHECK (type IN ('receipt', 'sale', 'sale_reversal'));
