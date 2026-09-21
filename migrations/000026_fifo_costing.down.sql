DROP TABLE sale_item_lot_allocations;
ALTER TABLE inventory_reviews DROP COLUMN sale_item_id;
ALTER TABLE stock_lots DROP COLUMN source;
ALTER TABLE stock_lots ALTER COLUMN receipt_line_id SET NOT NULL;
ALTER TABLE stock_lots DROP COLUMN remaining_quantity;
