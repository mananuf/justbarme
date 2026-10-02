DROP INDEX IF EXISTS payments_business_id_idempotency_key_key;
ALTER TABLE payments DROP COLUMN IF EXISTS idempotency_key;

DROP INDEX IF EXISTS bills_business_id_idempotency_key_key;
ALTER TABLE bills DROP COLUMN IF EXISTS idempotency_key;
