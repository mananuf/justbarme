DROP TABLE activity_flags;

ALTER TABLE inventory_events DROP CONSTRAINT inventory_events_type_check;
ALTER TABLE inventory_events
    ADD CONSTRAINT inventory_events_type_check
    CHECK (type IN ('receipt', 'sale', 'sale_reversal', 'adjustment'));

ALTER TABLE stock_receipts DROP COLUMN reversal_of_receipt_id;

ALTER TABLE stock_receipt_lines ADD CONSTRAINT stock_receipt_lines_total_cost_kobo_check CHECK (total_cost_kobo >= 0);
ALTER TABLE stock_receipt_lines DROP CONSTRAINT stock_receipt_lines_quantity_check;
ALTER TABLE stock_receipt_lines ADD CONSTRAINT stock_receipt_lines_quantity_check CHECK (quantity > 0);
