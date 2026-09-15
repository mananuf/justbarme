-- The tenant root. Not itself RLS-scoped by a business_id column — application
-- code resolves exactly one business per request via an active membership,
-- rather than filtering an arbitrary user-supplied business_id.
CREATE TABLE businesses (
    id                    UUID PRIMARY KEY,
    name                  TEXT NOT NULL,
    timezone              TEXT NOT NULL DEFAULT 'Africa/Lagos',
    currency              TEXT NOT NULL DEFAULT 'NGN',
    phone                 TEXT,
    address               TEXT,
    receipt_wording       TEXT,
    receipt_footer        TEXT,
    payment_instructions  TEXT,
    logo_object_key       TEXT,
    status                TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
