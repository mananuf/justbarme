-- Not every catalogue item is physical stock -- a snooker/pool game or
-- table time has no "restock" event, so unconditionally deducting
-- inventory_balances on every sale (internal/sales.postSaleRound) would
-- open a fresh negative_inventory review on every single sale, forever.
-- tracks_inventory lets an owner mark a variant as a service: false means
-- postSaleRound skips the inventory movement/balance/review pipeline
-- entirely for that variant, and internal/inventory.ReceiveStock refuses
-- to "restock" it. Defaults true -- every existing physical-drink variant
-- keeps behaving exactly as before.
ALTER TABLE product_variants ADD COLUMN tracks_inventory BOOLEAN NOT NULL DEFAULT true;
