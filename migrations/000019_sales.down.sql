ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_sale_id_required;
ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_sale_id_fkey;
ALTER TABLE inventory_events DROP COLUMN sale_id;
ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_type_check;
ALTER TABLE inventory_events ADD CONSTRAINT inventory_events_type_check
    CHECK (type IN ('receipt'));

DROP TABLE sale_reviews;
DROP TABLE payments;
DROP TABLE sale_items;
DROP TABLE sales;
DROP TABLE bills;
