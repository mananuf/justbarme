DROP TABLE bill_write_offs;

DROP INDEX bills_business_outstanding_idx;
ALTER TABLE bills DROP COLUMN balance_kobo;
ALTER TABLE bills DROP CONSTRAINT bills_customer_id_fkey;
ALTER TABLE bills DROP COLUMN customer_id;
ALTER TABLE bills DROP CONSTRAINT bills_table_id_fkey;
ALTER TABLE bills DROP COLUMN table_id;
ALTER TABLE bills DROP CONSTRAINT bills_status_check;
ALTER TABLE bills ADD CONSTRAINT bills_status_check
    CHECK (status IN ('open', 'settled'));

DROP TABLE customers;
DROP TABLE tables;
