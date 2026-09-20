-- Expenses -- docs/ARCHITECTURE.md §8.6, Phase 8
-- (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md). A wholly new
-- domain: nothing in this codebase touched expenses before this
-- migration. Same narrow, purpose-built pattern every other tenant table
-- here uses.

CREATE TABLE expense_categories (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    name        TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, name)
);

ALTER TABLE expense_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE expense_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY expense_categories_tenant_isolation ON expense_categories
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);

-- Immutable posted expense. idempotency_key is client-generated (a UUID
-- made on-device the moment the expense form is submitted) so a retried
-- POST after a lost response -- expenses:record is offline-safe, see
-- internal/tenancy/capabilities.go's offlineSafeCapabilities -- can never
-- double-post. reversal_of_expense_id is null on every ordinary expense;
-- a reversal is a second row here, never an edit of the original (same
-- pattern as sales.reversal_of_sale_id).
CREATE TABLE expenses (
    id                   UUID NOT NULL,
    business_id          UUID NOT NULL REFERENCES businesses (id),
    location_id          UUID NOT NULL,
    category_id          UUID NOT NULL,
    description          TEXT NOT NULL,
    amount_kobo          BIGINT NOT NULL CHECK (amount_kobo <> 0),
    payment_method       TEXT NOT NULL CHECK (payment_method IN ('cash', 'transfer', 'card')),
    recorded_by          UUID NOT NULL REFERENCES users (id),
    idempotency_key      UUID NOT NULL,
    occurred_at          TIMESTAMPTZ NOT NULL,
    reversal_of_expense_id UUID,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    UNIQUE (business_id, idempotency_key),
    FOREIGN KEY (business_id, location_id) REFERENCES locations (business_id, id),
    FOREIGN KEY (business_id, category_id) REFERENCES expense_categories (business_id, id),
    FOREIGN KEY (business_id, reversal_of_expense_id) REFERENCES expenses (business_id, id)
);

CREATE INDEX expenses_business_occurred_at_idx ON expenses (business_id, occurred_at DESC);
CREATE INDEX expenses_business_category_idx ON expenses (business_id, category_id);

ALTER TABLE expenses ENABLE ROW LEVEL SECURITY;
ALTER TABLE expenses FORCE ROW LEVEL SECURITY;
CREATE POLICY expenses_tenant_isolation ON expenses
    USING (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid)
    WITH CHECK (business_id = NULLIF(current_setting('app.business_id', true), '')::uuid);
