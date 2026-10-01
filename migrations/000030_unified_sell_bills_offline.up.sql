-- Adds client-generated idempotency keys to bills and payments, the
-- concrete backend gap identified in docs/PHASE_UNIFIED_SELL_BILLS.md
-- before offline queuing of bill actions can be made safe: unlike
-- AddSaleRound/RemoveBillItem (which already reuse sales.idempotency_key
-- via postSaleRound), OpenBill and RecordPayment had no way to detect a
-- retried/replayed queued action and would otherwise create a duplicate
-- bill or double-apply a payment.
--
-- Both are nullable (an online, interactively-opened bill/payment needs
-- no key) with a partial unique index so only rows that *do* carry one
-- are constrained -- the same shape sales.idempotency_key would use if it
-- weren't NOT NULL already (sales never had an online-only path that
-- skips it).
ALTER TABLE bills ADD COLUMN idempotency_key uuid;
CREATE UNIQUE INDEX bills_business_id_idempotency_key_key
    ON bills (business_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE payments ADD COLUMN idempotency_key uuid;
CREATE UNIQUE INDEX payments_business_id_idempotency_key_key
    ON payments (business_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
