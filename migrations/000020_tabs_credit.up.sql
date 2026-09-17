-- Phase 6 -- flexible tabs, named credit, and payments.
-- docs/ARCHITECTURE.md §8.3, docs/IMPLEMENTATION_PLAN.md Phase 6. Builds on
-- migration 000019's bills/payments; Phase 4's multi-device sync
-- foundation remains deferred (see project memory), so this stays a
-- single-device slice, same reasoning as Phase 5 -- see
-- docs/PHASE_TABS_CREDIT.md.

-- Business-configured labels (T1-T5 style). No kitchen/routing behavior.
CREATE TABLE tables (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    label       TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, label)
);

ALTER TABLE tables ENABLE ROW LEVEL SECURITY;
ALTER TABLE tables FORCE ROW LEVEL SECURITY;
CREATE POLICY tables_tenant_isolation ON tables
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- name is NOT NULL at the row level -- the "optional" part of customer
-- identity (docs/ARCHITECTURE.md §8.3) is whether a *bill* references a
-- customer at all, not whether a customer record can be created nameless.
CREATE TABLE customers (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    name        TEXT NOT NULL CHECK (name <> ''),
    phone       TEXT,
    email       TEXT,
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id)
);

CREATE INDEX customers_business_name_idx ON customers (business_id, name);

ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE ROW LEVEL SECURITY;
CREATE POLICY customers_tenant_isolation ON customers
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Widen bills for tabs: an optional table and/or named customer (a
-- walk-in bill has neither), and an outstanding-balance projection.
-- balance_kobo is a rebuildable projection, same "immutable movements +
-- materialized state" pattern as inventory_balances -- never itself the
-- source of truth. It is always sales.total_kobo summed for this bill,
-- minus payments.amount_kobo summed, minus bill_write_offs.amount_kobo
-- summed; internal/sales.Service recomputes and persists it, in the same
-- transaction as the write that changed it, on every bill mutation.
ALTER TABLE bills DROP CONSTRAINT bills_status_check;
ALTER TABLE bills ADD CONSTRAINT bills_status_check
    CHECK (status IN ('open', 'closed_unpaid', 'settled', 'void'));

ALTER TABLE bills ADD COLUMN table_id UUID;
ALTER TABLE bills ADD CONSTRAINT bills_table_id_fkey
    FOREIGN KEY (business_id, table_id) REFERENCES tables (business_id, id);

ALTER TABLE bills ADD COLUMN customer_id UUID;
ALTER TABLE bills ADD CONSTRAINT bills_customer_id_fkey
    FOREIGN KEY (business_id, customer_id) REFERENCES customers (business_id, id);

-- Every existing row is an already-settled, fully-paid walk-in sale
-- (Phase 5) -- 0 is factually correct for all of them, so no backfill
-- computation is needed.
ALTER TABLE bills ADD COLUMN balance_kobo BIGINT NOT NULL DEFAULT 0;

CREATE INDEX bills_business_outstanding_idx ON bills (business_id) WHERE balance_kobo <> 0;

-- Owner-only debt forgiveness (docs/ARCHITECTURE.md §8.3's explicit "Only
-- owners can write off debt" acceptance criterion). Never a payment -- no
-- money changes hands -- kept in its own table so `payments` stays "actual
-- money movement only."
CREATE TABLE bill_write_offs (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    bill_id     UUID NOT NULL,
    amount_kobo BIGINT NOT NULL CHECK (amount_kobo > 0),
    reason      TEXT NOT NULL CHECK (reason <> ''),
    actor_id    UUID NOT NULL REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, bill_id) REFERENCES bills (business_id, id)
);

CREATE INDEX bill_write_offs_bill_idx ON bill_write_offs (business_id, bill_id);

ALTER TABLE bill_write_offs ENABLE ROW LEVEL SECURITY;
ALTER TABLE bill_write_offs FORCE ROW LEVEL SECURITY;
CREATE POLICY bill_write_offs_tenant_isolation ON bill_write_offs
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
